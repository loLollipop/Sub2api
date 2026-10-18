package handler

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseDeepSeekToolsRequest(t *testing.T) {
	for _, tc := range []struct {
		name, body, query, path, model, content string
		invalid                                 bool
	}{
		{name: "default", body: `{"query":"latest news","future":true}`, path: service.DeepSeekToolsPathSearch, model: deepSeekToolsDefaultModel, content: "latest news"},
		{name: "query model", body: `{"query":"q"}`, query: "deepseek-v4-pro", path: service.DeepSeekToolsPathSearch, model: "deepseek-v4-pro", content: "q"},
		{name: "body wins", body: `{"query":"q","model":"deepseek-v4-flash"}`, query: "deepseek-v4-pro", path: service.DeepSeekToolsPathSearch, model: "deepseek-v4-flash", content: "q"},
		{name: "fetch", body: `{"url":"https://example.com/page"}`, path: service.DeepSeekToolsPathFetch, model: deepSeekToolsDefaultModel, content: "https://example.com/page"},
		{name: "array", body: `[{"query":"q"}]`, invalid: true},
		{name: "scalar", body: `"q"`, invalid: true},
		{name: "number", body: `{"query":5}`, invalid: true},
		{name: "object", body: `{"query":{"q":"text"}}`, invalid: true},
		{name: "empty", body: `{"query":"  "}`, invalid: true},
		{name: "null", body: `{"query":null}`, invalid: true},
		{name: "bad model", body: `{"query":"q","model":42}`, invalid: true},
		{name: "conflicting models", body: `{"query":"q","model":"a","model":"b"}`, invalid: true},
		{name: "duplicate query", body: `{"query":"safe","query":"other"}`, invalid: true},
		{name: "escaped duplicate query", body: `{"query":"safe","qu\u0065ry":"other"}`, invalid: true},
		{name: "case query alias", body: `{"query":"safe","QUERY":"other"}`, invalid: true},
		{name: "mixed case query", body: `{"Query":"other"}`, invalid: true},
		{name: "duplicate fetch url", body: `{"url":"https://example.com","url":"https://other.example"}`, path: service.DeepSeekToolsPathFetch, invalid: true},
		{name: "escaped duplicate fetch url", body: `{"url":"https://example.com","u\u0072l":"https://other.example"}`, path: service.DeepSeekToolsPathFetch, invalid: true},
		{name: "case fetch url alias", body: `{"url":"https://example.com","URL":"https://other.example"}`, path: service.DeepSeekToolsPathFetch, invalid: true},
		{name: "model alias", body: `{"query":"q","Model":"a"}`, invalid: true},
		{name: "escaped query", body: `{"qu\u0065ry":"q"}`, model: deepSeekToolsDefaultModel, content: "q"},
		{name: "fetch requires URL", body: `{"query":"q"}`, path: service.DeepSeekToolsPathFetch, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model, audit, err := parseDeepSeekToolsRequest([]byte(tc.body), tc.query, tc.path)
			if tc.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.model, model)
			require.Equal(t, tc.content, gjson.GetBytes(audit, "messages.0.content").String())
			require.Equal(t, "user", gjson.GetBytes(audit, "messages.0.role").String())
		})
	}
}

func TestDeepSeekToolsCanonicalEndpoints(t *testing.T) {
	for _, path := range []string{service.DeepSeekToolsPathSearch, service.DeepSeekToolsPathFetch} {
		require.Equal(t, path, NormalizeInboundEndpoint(path))
		require.Equal(t, path, NormalizeInboundEndpoint("/openai"+path))
		require.Equal(t, path, DeriveUpstreamEndpoint(path, path, service.PlatformDeepseek))
	}
}

func TestDeepSeekToolsMandatoryRecordSurvivesFullWorkerQueue(t *testing.T) {
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1
	logs, repo := &geminiBridgeUsageRepo{}, &geminiBridgeBillingRepo{}
	gateway := service.NewOpenAIGatewayService(nil, logs, repo, nil, nil, nil, nil, cfg, nil, nil, service.NewBillingService(cfg, nil), nil, &service.BillingCacheService{}, nil, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{WorkerCount: 1, QueueSize: 1, TaskTimeout: time.Second, OverflowPolicy: "drop"})
	started, release := make(chan struct{}), make(chan struct{})
	t.Cleanup(pool.Stop)
	t.Cleanup(func() { close(release) })
	pool.Submit(func(context.Context) { close(started); <-release })
	<-started
	pool.Submit(func(context.Context) {})
	h := &OpenAIGatewayHandler{gatewayService: gateway, usageRecordWorkerPool: pool}
	gid := int64(7)
	key := &service.APIKey{ID: 2, UserID: 1, User: &service.User{ID: 1}, GroupID: &gid, Group: &service.Group{ID: gid, Platform: service.PlatformDeepseek, RateMultiplier: 1}}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, service.DeepSeekToolsPathFetch, nil)
	result := &service.OpenAIForwardResult{RequestID: service.DeepSeekToolsLabelFetch + ":mandatory", Model: service.DeepSeekToolsLabelFetch, SearchOnly: true, SearchCount: 1, UpstreamEndpoint: service.DeepSeekToolsPathFetch}
	h.recordDeepSeekToolsUsage(c, zap.NewNop(), key, &service.Account{ID: 3, Platform: service.PlatformDeepseek, Type: service.AccountTypeAPIKey}, nil, []byte(`{"url":"https://example.com"}`), result, 1, time.Now())
	require.NotNil(t, logs.last, "overflow must synchronously preserve the mandatory charge and usage")
	require.NotNil(t, repo.last)
	require.Equal(t, result.RequestID, repo.last.RequestID)
	require.InDelta(t, 0.005, repo.last.BalanceCost, 1e-12)
	require.Equal(t, service.DeepSeekToolsLabelFetch, logs.last.Model)
	require.Equal(t, service.DeepSeekToolsPathFetch, *logs.last.InboundEndpoint)
	require.Equal(t, service.DeepSeekToolsPathFetch, *logs.last.UpstreamEndpoint)
}

func TestDeepSeekToolsReservationLivesUntilMandatorySettlementAndFailureReleases(t *testing.T) {
	for _, success := range []bool{false, true} {
		cfg := &config.Config{}
		cfg.Default.RateMultiplier = 1
		cfg.Billing.InflightReservation = config.InflightReservationConfig{Enabled: true, TTLSeconds: 60}
		cache := newHandlerInflightCache(1)
		billing := service.NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
		t.Cleanup(billing.Stop)
		gateway := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, service.NewBillingService(cfg, nil), nil, billing, nil, nil, nil, nil, nil, nil, nil, nil, nil)
		gid := int64(7)
		price := 13.0
		key := &service.APIKey{ID: 2, UserID: 1, User: &service.User{ID: 1}, GroupID: &gid, Group: &service.Group{ID: gid, Platform: service.PlatformDeepseek, RateMultiplier: 2, SearchPricePer1k: &price}}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, service.DeepSeekToolsPathSearch, nil)
		done, err := reserveInflightBalance(c, billing, service.DeepSeekToolsReservationEstimator{Gateway: gateway}, key, nil, service.InflightEstimateRequest{Model: service.DeepSeekToolsLabelSearch, SearchCalls: 1})
		require.NoError(t, err)
		require.Equal(t, 1, cache.count())
		cache.mu.Lock()
		for _, amount := range cache.res {
			require.InDelta(t, 0.026, amount, 1e-12)
		}
		cache.mu.Unlock()
		if !success {
			done()
			require.Eventually(t, func() bool { return cache.count() == 0 }, time.Second, 10*time.Millisecond)
			continue
		}
		pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{WorkerCount: 1, QueueSize: 2})
		t.Cleanup(pool.Stop)
		started, release, settled := make(chan struct{}), make(chan struct{}), make(chan struct{})
		pool.Submit(func(context.Context) { close(started); <-release })
		<-started
		h := &OpenAIGatewayHandler{usageRecordWorkerPool: pool}
		h.submitMandatoryUsageRecordTask(c.Request.Context(), func(context.Context) { close(settled) })
		done()
		require.Equal(t, 1, cache.count(), "the usage task must retain the reservation after the handler returns")
		close(release)
		select {
		case <-settled:
		case <-time.After(time.Second):
			t.Fatal("settlement did not run")
		}
		require.Eventually(t, func() bool { return cache.count() == 0 }, time.Second, 10*time.Millisecond)
	}
}
