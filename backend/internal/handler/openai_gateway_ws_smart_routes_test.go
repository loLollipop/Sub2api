//go:build unit

package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIResponsesWebSocket_SmartKeyResolvesFirstFrameFromAccountCatalog(t *testing.T) {
	for _, tc := range []struct {
		name            string
		primaryPlatform string
		model           string
		mappedModel     string
		channelMapped   bool
		singleGroup     bool
		wantCloseReason string
	}{
		{name: "native primary reaches later OpenAI account", primaryPlatform: service.PlatformTypeSafe, model: "gemini-catalog-alias", mappedModel: "gpt-5.1"},
		{name: "native primary reaches channel alias in later group", primaryPlatform: service.PlatformTypeSafe, model: "public-alias", mappedModel: "gpt-5.1", channelMapped: true},
		{name: "unresolved native primary fails closed", primaryPlatform: service.PlatformTypeSafe, model: "gpt-unmapped", mappedModel: "gpt-5.1", wantCloseReason: systemOneOnlyPlatformMessage},
		{name: "unresolved OpenAI primary fails closed", primaryPlatform: service.PlatformOpenAI, model: "gpt-unmapped", mappedModel: "gpt-5.1", wantCloseReason: "Responses WebSocket API requires a resolved OpenAI-compatible model for smart-routing keys"},
		{name: "single native group rejects foreign protocol", primaryPlatform: service.PlatformTypeSafe, model: "gemini-catalog-alias", mappedModel: "gpt-5.1", singleGroup: true, wantCloseReason: systemOneOnlyPlatformMessage},
		{name: "native model rejects foreign protocol", primaryPlatform: service.PlatformTypeSafe, model: "jev-latest", mappedModel: "gpt-5.1", wantCloseReason: systemOneOnlyPlatformMessage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			var upstreamCalls atomic.Int32
			upstreamPayload := make(chan []byte, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamCalls.Add(1)
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
				defer cancel()
				_, payload, err := conn.Read(ctx)
				if err != nil {
					return
				}
				upstreamPayload <- payload
				_ = conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.completed","response":{"id":"resp_smart_key","model":"gpt-5.1","usage":{"input_tokens":2,"output_tokens":1}}}`))
				_, _, _ = conn.Read(ctx)
			}))
			defer upstream.Close()

			primary := &service.Group{ID: 4200, Platform: tc.primaryPlatform, Status: service.StatusActive, Hydrated: true, RateMultiplier: 1}
			backup := &service.Group{ID: 4201, Platform: service.PlatformOpenAI, Status: service.StatusActive, Hydrated: true, RateMultiplier: 1}
			account := service.Account{
				ID: 9901, Name: "smart-key-ws-catalog", Platform: service.PlatformOpenAI,
				Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true,
				GroupIDs: []int64{backup.ID}, Concurrency: 1,
				Credentials: map[string]any{
					"api_key": "sk-local-regression", "base_url": upstream.URL,
					"model_mapping": map[string]any{"gemini-catalog-alias": tc.mappedModel},
				},
				Extra: map[string]any{
					"openai_apikey_responses_websockets_v2_enabled": true,
					"openai_apikey_responses_websockets_v2_mode":    service.OpenAIWSIngressModePassthrough,
				},
			}
			var channelService *service.ChannelService
			if tc.channelMapped {
				account.Credentials["model_mapping"] = map[string]any{tc.mappedModel: tc.mappedModel}
				channelService = service.NewChannelService(&openAIWSUsageHandlerChannelRepoStub{
					channels: []service.Channel{{
						ID: 7701, Name: "smart-key-channel-alias", Status: service.StatusActive,
						GroupIDs:     []int64{backup.ID},
						ModelMapping: map[string]map[string]string{service.PlatformOpenAI: {tc.model: tc.mappedModel}},
					}},
					groupPlatforms: map[int64]string{backup.ID: service.PlatformOpenAI},
				}, nil, nil, nil, nil)
				// Request scheduling deliberately uses nonblocking channel snapshots.
				// This test exercises routing with an existing mapping, not cold-cache
				// refresh timing; publish that snapshot before the WS request begins.
				mapping, restricted := channelService.ResolveChannelMappingAndRestrict(context.Background(), &backup.ID, tc.model)
				require.False(t, restricted)
				require.True(t, mapping.Mapped)
				require.Equal(t, tc.mappedModel, mapping.MappedModel)
			}
			cfg := &config.Config{RunMode: config.RunModeStandard}
			cfg.Default.RateMultiplier = 1
			cfg.Security.URLAllowlist.AllowInsecureHTTP = true
			cfg.Gateway.OpenAIWS.Enabled = true
			cfg.Gateway.OpenAIWS.APIKeyEnabled = true
			cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
			cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
			cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
			cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
			cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
			repo := &openAIWSUsageHandlerAccountRepoStub{account: account}
			accounts := []*service.Account{&account}
			if primary.Platform == service.PlatformTypeSafe {
				accounts = append(accounts, &service.Account{
					ID: 9900, Platform: service.PlatformTypeSafe, Type: service.AccountTypeAPIKey,
					Status: service.StatusActive, Schedulable: true, GroupIDs: []int64{primary.ID},
					Credentials: map[string]any{"api_key": "native-credential-must-not-forward", "base_url": upstream.URL},
				})
			}
			snapshot := service.NewSchedulerSnapshotService(&smartRouteHandlerSnapshot{fakeSchedulerCache{accounts: accounts}}, nil, repo,
				smartRouteHandlerGroups{groups: map[int64]*service.Group{primary.ID: primary, backup.ID: backup}}, cfg)
			for _, id := range []int64{primary.ID, backup.ID} {
				_, err := snapshot.GetGroupByIDLite(context.Background(), id)
				require.NoError(t, err)
			}
			billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, &config.Config{RunMode: config.RunModeSimple}, nil)
			t.Cleanup(billing.Stop)
			usage := &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 1)}
			refusals := &refusalHandlerRepo{durableMediaHandlerRepo: &durableMediaHandlerRepo{jobs: map[string]*service.MediaBillingJob{}}}
			billingRepo := smartRouteHandlerRefusalBilling{MediaBillingJobRepository: refusals, GrokRefusalRepository: refusals}
			gateway := service.NewOpenAIGatewayService(repo, usage, billingRepo, nil, nil, nil, nil, cfg, snapshot, nil,
				service.NewBillingService(cfg, nil), nil, billing, nil, &service.DeferredService{}, nil, nil, nil, channelService, nil, nil, nil)
			cache := &concurrencyCacheMock{
				acquireUserSlotFn:    func(context.Context, int64, int, string) (bool, error) { return true, nil },
				acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
			}
			h := &OpenAIGatewayHandler{gatewayService: gateway, billingCacheService: billing, apiKeyService: &service.APIKeyService{},
				concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second)}
			key := &service.APIKey{ID: 1801, UserID: 1701, GroupID: &primary.ID, Group: primary, RouteGroupIDs: []int64{primary.ID, backup.ID},
				User: &service.User{ID: 1701, Status: service.StatusActive}}
			if tc.singleGroup {
				key.RouteGroupIDs = nil
			}
			router := gin.New()
			router.GET("/openai/v1/responses", func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyAPIKey), key)
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: key.User.ID, Concurrency: 1})
				h.ResponsesWebSocket(c)
			})
			server := httptest.NewServer(router)
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/openai/v1/responses", nil)
			require.NoError(t, err)
			defer conn.CloseNow()
			require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(fmt.Sprintf(`{"type":"response.create","model":%q,"input":"hello"}`, tc.model))))
			_, event, err := conn.Read(ctx)
			if tc.wantCloseReason != "" {
				var closeErr coderws.CloseError
				require.ErrorAs(t, err, &closeErr)
				require.Equal(t, coderws.StatusPolicyViolation, closeErr.Code)
				require.Equal(t, tc.wantCloseReason, closeErr.Reason)
				require.Zero(t, upstreamCalls.Load(), "rejected keys must never forward credentials")
				return
			}
			require.NoError(t, err)
			require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())
			require.NoError(t, conn.Close(coderws.StatusNormalClosure, "done"))
			select {
			case payload := <-upstreamPayload:
				// Catalog ownership selects the OpenAI account, but the existing
				// passthrough transport intentionally preserves the original model.
				expectedModel := tc.model
				if tc.channelMapped {
					expectedModel = tc.mappedModel
				}
				require.Equal(t, expectedModel, gjson.GetBytes(payload, "model").String(), "catalog routing must preserve the existing channel mapping and passthrough semantics")
			case <-ctx.Done():
				t.Fatal("the catalog-resolved first frame did not reach the upstream")
			}
			select {
			case log := <-usage.created:
				require.Equal(t, backup.ID, *log.GroupID)
			case <-ctx.Done():
				t.Fatal("the selected backup group was not recorded")
			}
		})
	}
}
