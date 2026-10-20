package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// Exercise the actual handler -> delayed worker -> RecordCyberPolicyUsageLog ->
// real billing calculator path, not just the service input field in isolation.
func TestCyberHandlerAsyncUsageRetainsRequestOrTurnPricing(t *testing.T) {
	for _, transport := range []string{"http", "websocket turn"} {
		t.Run(transport, func(t *testing.T) {
			cfg := &config.Config{RunMode: config.RunModeSimple}
			cfg.Default.RateMultiplier = 1
			usageRepo := &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 2)}
			billing := service.NewBillingService(cfg, nil)
			gw := service.NewOpenAIGatewayService(nil, usageRepo, nil, nil, nil, nil, nil, cfg, nil, nil,
				billing, nil, nil, nil, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
			pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{WorkerCount: 1, QueueSize: 4})
			release, started := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			t.Cleanup(func() { unblock(); pool.Stop() })
			require.Equal(t, service.UsageRecordSubmitModeEnqueued, pool.Submit(func(context.Context) {
				close(started)
				<-release
			}))
			<-started
			h := &OpenAIGatewayHandler{gatewayService: gw, usageRecordWorkerPool: pool}
			c := newTestGinContext()
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			connectionCtx, connectionAt := gw.WithOpenAIRequestPricingContext(c.Request.Context(), nil)
			c.Request = c.Request.WithContext(connectionCtx)
			c.Writer.Header().Set("X-Request-Id", "cyber-async-"+transport)
			service.MarkOpsCyberPolicy(c, service.CyberPolicyMark{UpstreamInTok: 1000, UpstreamOutTok: 1})
			key := &service.APIKey{ID: 9, User: &service.User{ID: 5}}
			account := &service.Account{ID: 21, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}
			pricingAt := connectionAt
			var turnPricing openAIWSTurnPricing
			if transport == "http" {
				h.recordCyberPolicyIfMarked(c, key, account, nil, "deepseek-v4-pro", true, nil, service.ContentModerationProtocolOpenAIResponses, service.ChannelUsageFields{}, "")
			} else {
				// Deliberately different from the handshake clock and worker time.
				turnPricing.freeze(time.Date(2026, 10, 12, 2, 0, 0, 0, time.UTC))
				pricingAt = turnPricing.currentOr(connectionAt)
				require.False(t, pricingAt.Equal(connectionAt))
				h.recordCyberPolicyIfMarkedAt(c, key, account, nil, "deepseek-v4-pro", true, nil, service.ContentModerationProtocolOpenAIResponses, service.ChannelUsageFields{}, "", pricingAt)
			}
			// Later context/turn changes must not alter the queued money task.
			c.Request = c.Request.WithContext(context.Background())
			turnPricing.freeze(pricingAt.Add(3 * time.Hour))
			unblock()
			var usage *service.UsageLog
			select {
			case usage = <-usageRepo.created:
			case <-time.After(3 * time.Second):
				t.Fatal("cyber usage was not recorded")
			}
			expected, err := billing.CalculateCostUnified(service.CostInput{Ctx: context.Background(), Model: "deepseek-v4-pro",
				Tokens: service.UsageTokens{InputTokens: 1000, OutputTokens: 1}, RateMultiplier: 1, PricingAt: pricingAt})
			require.NoError(t, err)
			require.Equal(t, service.RequestTypeCyberBlocked, usage.RequestType)
			require.InDelta(t, expected.ActualCost, usage.ActualCost, 1e-12)
			if transport == "websocket turn" {
				other, err := billing.CalculateCostUnified(service.CostInput{Ctx: context.Background(), Model: "deepseek-v4-pro",
					Tokens: service.UsageTokens{InputTokens: 1000, OutputTokens: 1}, RateMultiplier: 1, PricingAt: pricingAt.Add(3 * time.Hour)})
				require.NoError(t, err)
				require.NotEqual(t, other.ActualCost, usage.ActualCost, "test must distinguish the pricing windows")
			}
			pool.Stop()
			select {
			case <-usageRepo.created:
				t.Fatal("one cyber event recorded multiple usage rows")
			default:
			}
		})
	}
}
