//go:build unit

package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// The monetary boundary uses the real calculator and durable guard lifecycle;
// only the external wallet and database operations are local deterministic fakes.
type initialHoldBoundaryWallet struct {
	service.BillingCache
	available float64
	reserved  float64
}

func (w *initialHoldBoundaryWallet) AuthorizeExistingLiveBalance(_ context.Context, _ int64, _ string, hold float64) (service.LiveBalanceResult, error) {
	if hold > w.available {
		return service.LiveBalanceResult{Outcome: service.LiveBalanceOutcomeInsufficient}, nil
	}
	w.available -= hold
	w.reserved = hold
	return service.LiveBalanceResult{Outcome: service.LiveBalanceOutcomeApplied, State: service.LiveBalanceAttemptAuthorized, ReservedAmount: hold}, nil
}

func (w *initialHoldBoundaryWallet) AuthorizeLiveBalanceAtWatermark(ctx context.Context, userID int64, attemptID string, _ float64, _ int64, hold float64) (service.LiveBalanceResult, error) {
	return w.AuthorizeExistingLiveBalance(ctx, userID, attemptID, hold)
}

func (w *initialHoldBoundaryWallet) AuthorizeLiveBalanceAtWatermarkIfSafe(ctx context.Context, userID int64, attemptID string, balance float64, watermark int64, hold float64, _ bool) (service.LiveBalanceResult, error) {
	return w.AuthorizeLiveBalanceAtWatermark(ctx, userID, attemptID, balance, watermark, hold)
}

func (w *initialHoldBoundaryWallet) RefundLiveBalance(context.Context, int64, string) (service.LiveBalanceResult, error) {
	w.available += w.reserved
	w.reserved = 0
	return service.LiveBalanceResult{Outcome: service.LiveBalanceOutcomeApplied, State: service.LiveBalanceAttemptRefunded}, nil
}

type initialHoldBoundaryRepo struct{ service.UsageBillingRepository }

func (*initialHoldBoundaryRepo) LoadLiveBalanceInitializationSnapshot(context.Context, int64) (service.LiveBalanceInitializationSnapshot, error) {
	return service.LiveBalanceInitializationSnapshot{}, fmt.Errorf("warm wallet must not need initialization")
}

func (*initialHoldBoundaryRepo) PrepareBalancePreauthorization(_ context.Context, cmd *service.BalancePreauthorizationCommand) (*service.BalancePreauthorizationRecord, error) {
	return &service.BalancePreauthorizationRecord{RequestID: cmd.RequestID, APIKeyID: cmd.APIKeyID, UserID: cmd.UserID, HoldAmount: cmd.HoldAmount, Status: service.BalanceSettlementPrepared}, nil
}

func (*initialHoldBoundaryRepo) MarkBalancePreauthorizationAuthorized(context.Context, string, int64) error {
	return nil
}

func (*initialHoldBoundaryRepo) BeginBalancePreauthorizationRefund(context.Context, string, int64) error {
	return nil
}

func (*initialHoldBoundaryRepo) CompleteBalancePreauthorizationRefund(context.Context, string, int64) error {
	return nil
}

func TestPreauthorizeTextFreeFastActualBalanceBoundary(t *testing.T) {
	for _, platform := range []string{service.PlatformOpenAI, service.PlatformComposite} {
		for _, freeFast := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/free=%t", platform, freeFast), func(t *testing.T) {
				cfg := &config.Config{RunMode: config.RunModeStandard, Default: config.DefaultConfig{RateMultiplier: 1}, Billing: config.BillingConfig{BalancePreauthorizationEnabled: true}}
				billing := service.NewBillingService(nil, nil)
				gateway := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, billing, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
				key := perRequestTestAPIKey()
				key.Group = &service.Group{ID: *key.GroupID, Platform: platform, FreeOpenAIFast: freeFast, RateMultiplier: 1}
				body := []byte(`{"model":"gpt-6.1-sol","input":"hello","max_output_tokens":32,"service_tier":"priority"}`)
				at := time.Unix(123, 0)
				estimate := service.EstimateBalancePreauthorizationTokens(body)
				input := gateway.BalancePreauthorizationCostInput(context.Background(), key, "gpt-6.1-sol", at, "")
				input.Tokens = service.UsageTokens{InputTokens: estimate.InputTokens, OutputTokens: estimate.OutputTokens}
				standard, err := billing.CalculateCostUnified(input)
				require.NoError(t, err)
				input.ServiceTier = "priority"
				priority, err := billing.CalculateCostUnified(input)
				require.NoError(t, err)
				require.Greater(t, priority.ActualCost, standard.ActualCost)
				balance := (standard.ActualCost + priority.ActualCost) / 2
				wallet := &initialHoldBoundaryWallet{available: balance}
				cache := service.NewBillingCacheService(wallet, nil, nil, nil, nil, nil, cfg, nil)
				t.Cleanup(cache.Stop)
				preauthorizer := service.NewBalancePreauthorizationService(cfg, billing, cache, &initialHoldBoundaryRepo{})
				guard, err := preauthorizeTextGatewayRequest(context.Background(), preauthorizer, gateway, key, nil, body, "gpt-6.1-sol", at, "priority")
				if !freeFast {
					require.ErrorIs(t, err, service.ErrBalanceWithholdingFailed)
					require.Nil(t, guard)
					require.Zero(t, wallet.reserved)
				} else {
					require.NoError(t, err)
					require.NotNil(t, guard)
					require.InDelta(t, standard.ActualCost, guard.HoldAmount(), 1e-8)
					require.Less(t, guard.HoldAmount(), priority.ActualCost)
					require.NoError(t, guard.Refund(context.Background()))
				}
				require.InDelta(t, balance, wallet.available, 1e-12)
			})
		}
	}
}

func TestAnthropicFinalPreauthorizationErrorEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, streamed := range []bool{false, true} {
		t.Run(fmt.Sprint(streamed), func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			if streamed {
				_, err := c.Writer.WriteString(": ping\n\n")
				require.NoError(t, err)
				c.Writer.Flush()
			}
			h := &OpenAIGatewayHandler{}
			require.False(t, h.handleAnthropicPreauthorizationError(c, nil, streamed))
			require.True(t, h.handleAnthropicPreauthorizationError(c, fmt.Errorf("final hold: %w", service.ErrBalanceWithholdingFailed), streamed))
			payload := rec.Body.String()
			if streamed {
				require.Equal(t, http.StatusOK, rec.Code)
				require.Contains(t, payload, "event: error\n")
				payload = strings.SplitN(strings.SplitN(payload, "data: ", 2)[1], "\n", 2)[0]
			} else {
				require.Equal(t, http.StatusTooManyRequests, rec.Code)
			}
			require.Equal(t, "error", gjson.Get(payload, "type").String())
			require.Equal(t, "insufficient_quota", gjson.Get(payload, "error.type").String())
			require.NotContains(t, payload, "upstream_error")
		})
	}
}
