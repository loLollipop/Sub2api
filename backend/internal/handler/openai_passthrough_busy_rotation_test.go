//go:build unit

package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type passthroughBusySlotCall struct {
	accountID int64
	limit     int
	requestID string
}

type passthroughBusyRotationCache struct {
	busyAccountConcurrencyCache
	mu           sync.Mutex
	acquires     []passthroughBusySlotCall
	releases     []passthroughBusySlotCall
	loadRequests int
}

func (c *passthroughBusyRotationCache) AcquireAccountSlot(ctx context.Context, accountID int64, limit int, requestID string) (bool, error) {
	c.mu.Lock()
	c.acquires = append(c.acquires, passthroughBusySlotCall{accountID: accountID, limit: limit, requestID: requestID})
	attempts := len(c.acquires)
	c.mu.Unlock()
	// Keep a broken rotation loop finite even if it ignores the request deadline.
	if attempts > 32 {
		return false, fmt.Errorf("unexpected account-slot retry loop: %d attempts", attempts)
	}
	return c.busyAccountConcurrencyCache.AcquireAccountSlot(ctx, accountID, limit, requestID)
}

func (c *passthroughBusyRotationCache) ReleaseAccountSlot(_ context.Context, accountID int64, requestID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.releases = append(c.releases, passthroughBusySlotCall{accountID: accountID, requestID: requestID})
	return nil
}

func (c *passthroughBusyRotationCache) GetAccountsLoadBatch(_ context.Context, accounts []service.AccountWithConcurrency) (map[int64]*service.AccountLoadInfo, error) {
	c.mu.Lock()
	c.loadRequests++
	c.mu.Unlock()
	loads := make(map[int64]*service.AccountLoadInfo, len(accounts))
	for _, account := range accounts {
		// The load snapshot predates A becoming busy. Admission must detect the
		// occupied slot and retain the passthrough fallback in the candidate pool.
		loads[account.ID] = &service.AccountLoadInfo{AccountID: account.ID}
	}
	return loads, nil
}

func (c *passthroughBusyRotationCache) calls() ([]passthroughBusySlotCall, []passthroughBusySlotCall, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]passthroughBusySlotCall(nil), c.acquires...), append([]passthroughBusySlotCall(nil), c.releases...), c.loadRequests
}

type passthroughBusyRotationUpstream struct {
	service.HTTPUpstream
	mu         sync.Mutex
	accountIDs []int64
	models     []string
}

func (u *passthroughBusyRotationUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	u.mu.Lock()
	u.accountIDs = append(u.accountIDs, accountID)
	u.models = append(u.models, gjson.GetBytes(body, "model").String())
	u.mu.Unlock()
	response := `{"id":"resp_passthrough_busy_rotation","object":"response","model":"gpt-6.1-sol","status":"completed","output":[{"id":"msg_passthrough","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
	contentType := "application/json"
	// OAuth passthrough forces streaming upstream; return its required terminal
	// event. API-key accounts exercise the native non-streaming JSON response.
	if gjson.GetBytes(body, "stream").Bool() {
		contentType = "text/event-stream"
		response = "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":" + response + "}\n\n"
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(response)),
	}, nil
}

func (u *passthroughBusyRotationUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func (u *passthroughBusyRotationUpstream) calls() ([]int64, []string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]int64(nil), u.accountIDs...), append([]string(nil), u.models...)
}

type passthroughBusyRotationSettingRepo struct {
	service.SettingRepository
	values map[string]string
}

func (s *passthroughBusyRotationSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	if value, ok := s.values[key]; ok {
		return value, nil
	}
	return "", service.ErrSettingNotFound
}

func (s *passthroughBusyRotationSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			values[key] = value
		}
	}
	return values, nil
}

func (s *passthroughBusyRotationSettingRepo) SetMultiple(_ context.Context, values map[string]string) error {
	for key, value := range values {
		s.values[key] = value
	}
	return nil
}

func (s *passthroughBusyRotationSettingRepo) GetAll(ctx context.Context) (map[string]string, error) {
	keys := make([]string, 0, len(s.values))
	for key := range s.values {
		keys = append(keys, key)
	}
	return s.GetMultiple(ctx, keys)
}

// Exercise real Responses selection, admission, forwarding and release. A has
// an explicit model mapping; B delegates model support to its passthrough upstream.
func TestOpenAIResponses_PassthroughBusyRotation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, scheduler := range []struct {
		name      string
		advanced  bool
		loadBatch bool
	}{
		{name: "advanced_top_k_1", advanced: true, loadBatch: true},
		{name: "legacy_without_load_batch"},
		{name: "legacy_with_load_batch", loadBatch: true},
	} {
		for _, accountType := range []string{service.AccountTypeAPIKey, service.AccountTypeOAuth} {
			for _, mappedPassthrough := range []bool{false, true} {
				for _, allBusy := range []bool{false, true} {
					name := fmt.Sprintf("%s/%s/mapped_passthrough=%t/all_busy=%t", scheduler.name, accountType, mappedPassthrough, allBusy)
					t.Run(name, func(t *testing.T) {
						const mappedID, fallbackID, groupID = int64(1257), int64(1258), int64(7)
						accounts := []service.Account{
							{ID: mappedID, Name: "mapped-busy", Platform: service.PlatformOpenAI, Type: accountType, Status: service.StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID}, Credentials: map[string]any{"api_key": "sk-test-mapped", "access_token": "test-mapped-token", "base_url": "https://api.example.test", "model_mapping": map[string]any{"gpt-6.1-sol": "gpt-6.1-sol"}}, Extra: map[string]any{"openai_passthrough": mappedPassthrough}},
							{ID: fallbackID, Name: "unmapped-passthrough", Platform: service.PlatformOpenAI, Type: accountType, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, GroupIDs: []int64{groupID}, Credentials: map[string]any{"api_key": "sk-test-fallback", "access_token": "test-fallback-token", "base_url": "https://api.example.test", "model_mapping": map[string]any{}}, Extra: map[string]any{"openai_passthrough": true}},
						}
						require.Empty(t, accounts[1].GetModelMapping())
						cache := &passthroughBusyRotationCache{busyAccountConcurrencyCache: busyAccountConcurrencyCache{busy: map[int64]bool{mappedID: true, fallbackID: allBusy}}}
						concurrency := service.NewConcurrencyService(cache)
						upstream := &passthroughBusyRotationUpstream{}
						cfg := &config.Config{RunMode: config.RunModeSimple}
						cfg.Default.RateMultiplier = 1
						cfg.Gateway.MaxAccountSwitches = 1
						cfg.Gateway.Scheduling.LoadBatchEnabled = scheduler.loadBatch
						cfg.Gateway.OpenAIWS.LBTopK = 1
						cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
						settings := service.NewSettingService(&passthroughBusyRotationSettingRepo{values: map[string]string{}}, cfg)
						// The scheduler cache is process-wide. Use its public settings write
						// path to publish each mode immediately instead of waiting for its TTL.
						initialSettings, err := settings.GetAllSettings(context.Background())
						require.NoError(t, err)
						t.Cleanup(func() { require.NoError(t, settings.UpdateSettings(context.Background(), initialSettings)) })
						require.NoError(t, settings.UpdateSettings(context.Background(), &service.SystemSettings{OpenAIAdvancedSchedulerEnabled: scheduler.advanced, OpenAIAdvancedSchedulerLBTopK: "1"}))
						repo := &openAIWSFailoverHandlerAccountRepoStub{accounts: accounts}
						rateLimit := service.NewRateLimitService(repo, nil, cfg, nil, nil)
						rateLimit.SetSettingService(settings)
						billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
						t.Cleanup(billing.Stop)
						gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, cfg, nil, concurrency, service.NewBillingService(cfg, nil), rateLimit, billing, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
						h := NewOpenAIGatewayHandler(gateway, concurrency, billing, service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg)
						ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
						defer cancel()
						rec := httptest.NewRecorder()
						c, _ := gin.CreateTestContext(rec)
						c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(`{"model":"gpt-6.1-sol","input":"hello","stream":false}`)).WithContext(ctx)
						c.Request.Header.Set("Content-Type", "application/json")
						requestGroupID := groupID
						c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 99, GroupID: &requestGroupID, User: &service.User{ID: 100, Status: service.StatusActive}, Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive, Hydrated: true}})
						c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 100, Concurrency: 1})

						h.Responses(c)

						require.NoError(t, ctx.Err(), "account exhaustion must terminate before the client deadline")
						acquires, releases, loadRequests := cache.calls()
						require.NotEmpty(t, acquires)
						require.LessOrEqual(t, len(acquires), 32)
						require.Equal(t, mappedID, acquires[0].accountID, "the higher-priority mapped account must be tried first")
						var attemptedIDs []int64
						for _, call := range acquires {
							attemptedIDs = append(attemptedIDs, call.accountID)
							require.Equal(t, 1, call.limit)
							require.NotEmpty(t, call.requestID)
						}
						require.Contains(t, attemptedIDs, fallbackID, "the empty-mapping passthrough account must survive model filtering")
						if scheduler.loadBatch {
							require.Positive(t, loadRequests)
						} else {
							require.Zero(t, loadRequests)
						}
						upstreamIDs, models := upstream.calls()
						if allBusy {
							require.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
							require.Equal(t, gatewayConcurrencyLimitCode, gjson.GetBytes(rec.Body.Bytes(), "error.code").String())
							require.Empty(t, upstreamIDs)
							require.Empty(t, releases)
							return
						}
						require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
						if accountType == service.AccountTypeOAuth {
							require.Contains(t, rec.Body.String(), `"type":"response.completed"`)
							require.Contains(t, rec.Body.String(), `"id":"resp_passthrough_busy_rotation"`)
						} else {
							require.Equal(t, "resp_passthrough_busy_rotation", gjson.GetBytes(rec.Body.Bytes(), "id").String())
						}
						require.Equal(t, []int64{fallbackID}, upstreamIDs, "the busy mapped account must never reach upstream")
						require.Equal(t, []string{"gpt-6.1-sol"}, models)
						require.Len(t, releases, 1)
						require.Equal(t, fallbackID, releases[0].accountID)
						require.Contains(t, acquires, passthroughBusySlotCall{accountID: fallbackID, limit: 1, requestID: releases[0].requestID}, "release must reclaim the acquired fallback slot")
					})
				}
			}
		}
	}
}
