//go:build unit

package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDeepSeekToolsFlowUsesRealModelSelectedGroupAndReleasesSlots(t *testing.T) {
	for _, scenario := range []string{"success", "group rejects model", "capability excluded", "upstream failure", "duplicate query", "escaped query", "case query", "duplicate url", "escaped url", "case url"} {
		t.Run(scenario, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			cfg := &config.Config{}
			cfg.Default.RateMultiplier = 1
			price := 9.0
			primary := &service.Group{ID: 9, Platform: service.PlatformDeepseek, Status: service.StatusActive, Hydrated: true, RateMultiplier: 1}
			backup := &service.Group{ID: 17, Platform: service.PlatformDeepseek, Status: service.StatusActive, Hydrated: true, RateMultiplier: 2, SearchPricePer1k: &price}
			for _, g := range []*service.Group{primary, backup} {
				g.ModelAllowlist = service.GroupModelsListConfig{Enabled: true, Models: []string{deepSeekToolsDefaultModel}}
				if scenario == "group rejects model" {
					g.ModelAllowlist.Models = []string{"deepseek-v4-pro"}
				}
			}
			account := service.Account{ID: 32, Platform: service.PlatformDeepseek, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 2, GroupIDs: []int64{17},
				Credentials: map[string]any{"api_key": "fixture-only", "base_url": "https://api.deepseek.com/v1", "model_mapping": map[string]any{deepSeekToolsDefaultModel: deepSeekToolsDefaultModel}}}
			if scenario == "capability excluded" {
				account.Credentials["openai_capabilities"] = []string{"chat_completions"}
			}
			accountRepo := openAIImagesFailoverAccountRepo{accounts: []service.Account{account}}
			snapshot := service.NewSchedulerSnapshotService(&smartRouteHandlerSnapshot{fakeSchedulerCache{accounts: []*service.Account{&account}}}, nil, accountRepo, smartRouteHandlerGroups{groups: map[int64]*service.Group{9: primary, 17: backup}}, cfg)
			for _, id := range []int64{9, 17} {
				_, err := snapshot.GetGroupByIDLite(context.Background(), id)
				require.NoError(t, err)
			}
			cache := &grokMediaSlotsCache{accounts: map[string]int64{}, users: map[string]int64{}}
			concurrency := service.NewConcurrencyService(cache)
			billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, &config.Config{RunMode: config.RunModeSimple}, nil)
			t.Cleanup(billing.Stop)
			usage := smartRouteHandlerUsage{logs: make(chan *service.UsageLog, 2)}
			body := ` {"query":"actual search text","extra":{"keep":true}} `
			invalidBodies := map[string]string{
				"duplicate query": `{"query":"safe","query":"other"}`,
				"escaped query":   `{"query":"safe","qu\u0065ry":"other"}`,
				"case query":      `{"query":"safe","Query":"other"}`,
				"duplicate url":   `{"url":"https://example.com","url":"https://other.example"}`,
				"escaped url":     `{"url":"https://example.com","u\u0072l":"https://other.example"}`,
				"case url":        `{"url":"https://example.com","URL":"https://other.example"}`,
			}
			invalidBody, invalid := invalidBodies[scenario]
			path := service.DeepSeekToolsPathSearch
			if invalid {
				body = invalidBody
				if strings.HasSuffix(scenario, "url") {
					path = service.DeepSeekToolsPathFetch
				}
			}
			calls := 0
			upstream := &grokMediaSlotUpstream{call: func(req *http.Request, id int64) (*http.Response, error) {
				calls++
				require.Equal(t, account.ID, id)
				group, ok := req.Context().Value(ctxkey.Group).(*service.Group)
				require.True(t, ok)
				require.Equal(t, backup.ID, group.ID)
				forwarded, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.Equal(t, body, string(forwarded))
				require.Equal(t, service.DeepSeekToolsPathSearch, req.URL.Path)
				status, payload := 200, `{"results":["ok"]}`
				if scenario == "upstream failure" {
					status = 400
					payload = `{"error":{"message":"fixture-only"}}`
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(payload))}, nil
			}}
			gateway := service.NewOpenAIGatewayService(accountRepo, usage, smartRouteHandlerBilling{}, nil, nil, nil, nil, cfg, snapshot, concurrency, service.NewBillingService(cfg, nil), nil, billing, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
			h := NewOpenAIGatewayHandler(gateway, concurrency, billing, &service.APIKeyService{}, nil, nil, nil, nil, cfg)
			key := &service.APIKey{ID: 7, UserID: 42, User: &service.User{ID: 42, Status: service.StatusActive}, GroupID: &primary.ID, Group: primary, RouteGroupIDs: []int64{9, 17}}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)).WithContext(context.WithValue(context.Background(), ctxkey.Group, primary))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set(string(middleware.ContextKeyAPIKey), key)
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42, Concurrency: 2})
			if path == service.DeepSeekToolsPathFetch {
				h.DeepSeekToolsFetch(c)
			} else {
				h.DeepSeekToolsSearch(c)
			}
			if invalid {
				require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
			}
			if scenario == "success" {
				require.Equal(t, 200, recorder.Code, recorder.Body.String())
				require.Equal(t, 1, calls)
				select {
				case log := <-usage.logs:
					require.Equal(t, backup.ID, *log.GroupID)
					require.Equal(t, 2.0, log.RateMultiplier)
					require.InDelta(t, 0.018, log.ActualCost, 1e-12)
					require.Equal(t, service.DeepSeekToolsLabelSearch, log.Model)
					require.Equal(t, service.DeepSeekToolsPathSearch, *log.InboundEndpoint)
					require.Equal(t, service.DeepSeekToolsPathSearch, *log.UpstreamEndpoint)
				case <-time.After(3 * time.Second):
					t.Fatal("successful search usage missing")
				}
			} else {
				require.GreaterOrEqual(t, recorder.Code, 400, recorder.Body.String())
				if scenario == "upstream failure" {
					require.Equal(t, 1, calls)
				} else {
					require.Zero(t, calls)
				}
				select {
				case <-usage.logs:
					t.Fatal("failed tool call must not be billed")
				default:
				}
			}
			cache.assertReleased(t)
		})
	}
}
