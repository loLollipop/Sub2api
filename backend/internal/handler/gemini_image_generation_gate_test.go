//go:build unit

package handler

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// groupScopedSchedulerCache 按桶的分组只返回该分组的成员账号，并记录被查询过的分组。
type groupScopedSchedulerCache struct {
	*fakeSchedulerCache
	mu       sync.Mutex
	groupIDs []int64
}

func (c *groupScopedSchedulerCache) GetSnapshot(_ context.Context, bucket service.SchedulerBucket) ([]*service.Account, bool, error) {
	c.mu.Lock()
	c.groupIDs = append(c.groupIDs, bucket.GroupID)
	c.mu.Unlock()
	var members []*service.Account
	for _, account := range c.accounts {
		for _, ag := range account.AccountGroups {
			if ag.GroupID == bucket.GroupID {
				members = append(members, account)
				break
			}
		}
	}
	return members, true, nil
}

func (c *groupScopedSchedulerCache) queriedGroupIDs() []int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int64(nil), c.groupIDs...)
}

type groupMapRepo struct {
	*fakeGroupRepo
	groups map[int64]*service.Group
}

func (r *groupMapRepo) GetByID(_ context.Context, id int64) (*service.Group, error) {
	if group, ok := r.groups[id]; ok {
		return group, nil
	}
	return nil, service.ErrGroupNotFound
}

func (r *groupMapRepo) GetByIDLite(ctx context.Context, id int64) (*service.Group, error) {
	return r.GetByID(ctx, id)
}

type geminiImageGateEndpoint struct {
	name          string
	path          string
	body          string
	params        gin.Params
	forcePlatform string
	call          func(*GatewayHandler, *gin.Context)
}

func geminiImageGateEndpoints(model string) []geminiImageGateEndpoint {
	return []geminiImageGateEndpoint{
		{
			name: "messages", path: "/v1/messages",
			body: `{"model":"` + model + `","max_tokens":16,"messages":[{"role":"user","content":"draw a cat"}]}`,
			call: (*GatewayHandler).Messages,
		},
		{
			name: "chat completions", path: "/v1/chat/completions",
			body: `{"model":"` + model + `","messages":[{"role":"user","content":"draw a cat"}]}`,
			call: (*GatewayHandler).ChatCompletions,
		},
		{
			name: "responses", path: "/v1/responses",
			body: `{"model":"` + model + `","input":"draw a cat"}`,
			call: (*GatewayHandler).Responses,
		},
		{
			name: "gemini native", path: "/v1beta/models/" + model + ":generateContent",
			body:   `{"contents":[{"role":"user","parts":[{"text":"draw a cat"}]}]}`,
			params: gin.Params{{Key: "modelAction", Value: "/" + model + ":generateContent"}},
			call:   (*GatewayHandler).GeminiV1BetaModels,
		},
	}
}

func runGeminiImageGateEndpoint(t *testing.T, ep geminiImageGateEndpoint, group *service.Group, accountPlatform string) (*httptest.ResponseRecorder, *gin.Context, *groupScopedSchedulerCache) {
	t.Helper()
	const accountID = int64(9411)
	schedulerCache := &groupScopedSchedulerCache{fakeSchedulerCache: &fakeSchedulerCache{accounts: []*service.Account{{
		ID: accountID, Platform: accountPlatform, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true, Concurrency: 1,
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: group.ID}},
	}}}}
	gatewayService := service.NewGatewayService(
		nil, &groupMapRepo{fakeGroupRepo: &fakeGroupRepo{}, groups: map[int64]*service.Group{group.ID: group}},
		nil, nil, nil, nil, nil, nil, nil,
		service.NewSchedulerSnapshotService(schedulerCache, nil, nil, nil, nil),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	cfg := &config.Config{RunMode: config.RunModeSimple}
	billingCacheService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCacheService.Stop)
	h := &GatewayHandler{
		gatewayService:      gatewayService,
		billingCacheService: billingCacheService,
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(&fakeConcurrencyCache{}), SSEPingFormatClaude, 0),
		maxAccountSwitches:  1,
		cfg:                 cfg,
	}

	groupID := group.ID
	apiKey := &service.APIKey{
		ID: 9420, UserID: 9430, GroupID: &groupID, Group: group, Status: service.StatusActive,
		User: &service.User{ID: 9430, Concurrency: 10, Balance: 100},
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx := context.WithValue(context.Background(), ctxkey.Group, group)
	req := httptest.NewRequest(http.MethodPost, ep.path, bytes.NewBufferString(ep.body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Params = ep.params
	if ep.forcePlatform != "" {
		c.Set(string(middleware.ContextKeyForcePlatform), ep.forcePlatform)
	}
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: 10})

	ep.call(h, c)
	return recorder, c, schedulerCache
}

func TestGatewayHandlers_GeminiImageGenerationDisabledGroupRejectsBeforeSelection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, platform := range []string{service.PlatformGemini, service.PlatformAntigravity} {
		for _, ep := range geminiImageGateEndpoints("gemini-3-pro-image") {
			if platform == service.PlatformAntigravity && ep.params != nil {
				ep.path = "/antigravity" + ep.path
				ep.forcePlatform = service.PlatformAntigravity
			}
			t.Run(platform+"/"+ep.name, func(t *testing.T) {
				group := &service.Group{
					ID: 9400, Hydrated: true, Platform: platform, Status: service.StatusActive,
					AllowImageGeneration: false,
				}
				recorder, c, schedulerCache := runGeminiImageGateEndpoint(t, ep, group, platform)

				require.Equal(t, http.StatusBadRequest, recorder.Code)
				require.Contains(t, recorder.Body.String(), service.ImageGenerationPermissionMessage())
				_, reachedSelection := c.Get(opsAccountIDKey)
				require.False(t, reachedSelection)
				require.Empty(t, schedulerCache.queriedGroupIDs())
			})
		}
	}
}

func TestGatewayHandlers_GeminiImageGenerationGateSkipsGroupsWithoutTheSwitch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Anthropic 分组在后台不展示生图开关，混合调度 Antigravity 账号时不受门禁约束。
	for _, ep := range geminiImageGateEndpoints("gemini-3-pro-image")[:3] {
		t.Run(ep.name, func(t *testing.T) {
			group := &service.Group{
				ID: 9401, Hydrated: true, Platform: service.PlatformAnthropic, Status: service.StatusActive,
				AllowImageGeneration: false,
			}
			recorder, _, schedulerCache := runGeminiImageGateEndpoint(t, ep, group, service.PlatformAnthropic)

			require.NotContains(t, recorder.Body.String(), service.ImageGenerationPermissionMessage())
			require.NotEmpty(t, schedulerCache.queriedGroupIDs())
		})
	}
}

func TestGatewayHandlers_GeminiImageGenerationGateAllowsTextModelsAndEnabledGroups(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name  string
		model string
		allow bool
	}{
		{name: "text model in disabled group", model: "gemini-3.7-flash", allow: false},
		{name: "image model in enabled group", model: "gemini-3-pro-image", allow: true},
	}
	for _, tc := range cases {
		for _, ep := range geminiImageGateEndpoints(tc.model)[:3] {
			t.Run(tc.name+"/"+ep.name, func(t *testing.T) {
				group := &service.Group{
					ID: 9402, Hydrated: true, Platform: service.PlatformGemini, Status: service.StatusActive,
					AllowImageGeneration: tc.allow,
				}
				recorder, _, schedulerCache := runGeminiImageGateEndpoint(t, ep, group, service.PlatformGemini)

				require.NotContains(t, recorder.Body.String(), service.ImageGenerationPermissionMessage())
				require.NotEmpty(t, schedulerCache.queriedGroupIDs())
			})
		}
	}
}

// The handler must stop locally after account mapping, before entering the OpenAI
// gateway. Two eligible accounts make an accidental failover observable.
func TestGatewayHandlers_GeminiOpenAIProtocolAliasImageGenerationGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, allow := range []bool{false, true} {
		for _, ep := range geminiImageGateEndpoints("my-draw") {
			t.Run(ep.name+"/allow="+fmt.Sprint(allow), func(t *testing.T) {
				f := newSingleAccountProbeFixture(t)
				f.group.AllowImageGeneration = allow
				f.group.CrossProtocolConversionEnabled = true
				account := f.schedulerCache.accounts[0]
				account.Credentials["api_protocol"] = service.APIProtocolChatCompletions
				account.Credentials["base_url"] = "https://gemini-relay.example"
				account.Credentials["model_mapping"] = map[string]any{"my-draw": "gemini-3-pro-image"}
				second := *account
				second.ID++
				f.schedulerCache.accounts = append(f.schedulerCache.accounts, &second)
				cfg := &config.Config{RunMode: config.RunModeSimple}
				f.handler.cfg = cfg
				f.handler.openAIGatewayService = service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, f.upstream, nil, nil, nil, nil, nil, nil, nil, nil)
				var requestContext *gin.Context
				route := ep.path
				if ep.params != nil {
					route = "/v1beta/models/*modelAction"
				}
				rec, _ := f.serve(t, route, ep.path, ep.body, func(c *gin.Context) {
					requestContext = c
					ep.call(f.handler, c)
				})
				require.Equal(t, http.StatusBadRequest, rec.Code)
				geminiBucketReads := 0
				for _, bucket := range f.schedulerCache.readPlatforms() {
					if strings.HasPrefix(bucket, service.PlatformGemini+":") {
						geminiBucketReads++
					}
				}
				require.Equal(t, 1, geminiBucketReads, "must not select a second account")
				require.True(t, account.Schedulable)
				require.Equal(t, service.StatusActive, account.Status)
				require.Nil(t, account.TempUnschedulableUntil)
				if allow {
					require.NotContains(t, rec.Body.String(), service.ImageGenerationPermissionMessage())
					require.EqualValues(t, 1, f.upstream.calls.Load())
					require.False(t, service.HasOpsClientBusinessLimited(requestContext))
				} else {
					require.Contains(t, rec.Body.String(), service.ImageGenerationPermissionMessage())
					require.Zero(t, f.upstream.calls.Load())
					require.Equal(t, service.OpsClientBusinessLimitedReasonLocalFeatureGate, service.OpsClientBusinessLimitedReason(requestContext))
				}
			})
		}
	}
}

func TestGatewayHandlers_CompositePublicImageModelRemainsGatedAfterAliasRewrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, allow := range []bool{false, true} {
		for _, ep := range geminiImageGateEndpoints("nano-banana-2")[:3] {
			t.Run(ep.name+"/allow="+fmt.Sprint(allow), func(t *testing.T) {
				f := newSingleAccountProbeFixture(t)
				f.group.Platform = service.PlatformComposite
				f.group.CrossProtocolConversionEnabled = true
				account := f.schedulerCache.accounts[0]
				account.Credentials["api_protocol"] = service.APIProtocolChatCompletions
				account.Credentials["base_url"] = "https://gemini-relay.example"
				cfg := &config.Config{RunMode: config.RunModeSimple}
				f.handler.cfg = cfg
				f.handler.openAIGatewayService = service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, f.upstream, nil, nil, nil, nil, nil, nil, nil, nil)
				f.group.AllowImageGeneration = allow
				// Match the middleware state after rewriting the body to the relay alias.
				// No IMAGE output modality is present in any of these JSON requests.
				f.ctx = service.WithCompositeRouteDecision(f.ctx, service.CompositeRouteDecision{
					Matched: true, PublicModel: "gemini-3-pro-image", UpstreamModel: "nano-banana-2",
					TargetPlatform: service.PlatformGemini,
				})
				var requestContext *gin.Context
				rec, _ := f.serve(t, ep.path, ep.path, ep.body, func(c *gin.Context) {
					requestContext = c
					ep.call(f.handler, c)
				})
				if allow {
					require.EqualValues(t, 1, f.upstream.calls.Load())
					require.NotContains(t, rec.Body.String(), service.ImageGenerationPermissionMessage())
					require.NotEmpty(t, f.schedulerCache.readPlatforms())
				} else {
					require.Equal(t, http.StatusBadRequest, rec.Code)
					require.Contains(t, rec.Body.String(), service.ImageGenerationPermissionMessage())
					require.Zero(t, f.upstream.calls.Load())
					require.Empty(t, f.schedulerCache.readPlatforms(), "reject before selection")
					require.Equal(t, service.OpsClientBusinessLimitedReasonLocalFeatureGate, service.OpsClientBusinessLimitedReason(requestContext))
				}
			})
		}
	}
}

type imageGateAffinityCache struct {
	service.GatewayCache
	binding     int64
	invalidated int
}

func (cache *imageGateAffinityCache) GetSessionAccountID(context.Context, int64, string) (int64, error) {
	return cache.binding, nil
}

func (cache *imageGateAffinityCache) SetSessionAccountID(_ context.Context, _ int64, _ string, accountID int64, _ time.Duration) error {
	cache.binding = accountID
	return nil
}

func (*imageGateAffinityCache) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}

func (cache *imageGateAffinityCache) InvalidateFailedSession(context.Context, int64, int64, string, int64) error {
	cache.invalidated++
	cache.binding = 0
	return nil
}

func (*imageGateAffinityCache) FailedUserAccounts(context.Context, int64, int64) ([]int64, error) {
	return nil, nil
}

func TestGatewayHandlers_MessagesNormalizedGeminiImageMappingGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, protocol := range []string{service.APIProtocolChatCompletions, service.APIProtocolResponses} {
		for _, allow := range []bool{false, true} {
			t.Run(protocol+"/allow="+fmt.Sprint(allow), func(t *testing.T) {
				f := newSingleAccountProbeFixture(t)
				affinity := &imageGateAffinityCache{}
				f.handler.gatewayService = service.NewGatewayService(
					nil, &fakeGroupRepo{group: f.group}, nil, nil, nil, nil, nil, affinity, nil,
					service.NewSchedulerSnapshotService(f.schedulerCache, nil, nil, nil, nil),
					nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
				)
				f.group.AllowImageGeneration = allow
				f.group.CrossProtocolConversionEnabled = true
				account := f.schedulerCache.accounts[0]
				account.Credentials["api_protocol"] = protocol
				account.Credentials["base_url"] = "https://gemini-relay.example"
				// Selection accepts the exact client spelling. The Messages
				// converter normalizes its effort suffix before the final mapping.
				account.Credentials["model_mapping"] = map[string]any{
					"gpt-5.4-xhigh": "gpt-5.4-xhigh",
					"gpt-5.4":       "gemini-3-pro-image",
				}
				cfg := &config.Config{RunMode: config.RunModeSimple}
				f.handler.cfg = cfg
				f.handler.openAIGatewayService = service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, f.upstream, nil, nil, nil, nil, nil, nil, nil, nil)
				ep := geminiImageGateEndpoints("gpt-5.4-xhigh")[0]
				var requestContext *gin.Context
				rec, _ := f.serve(t, ep.path, ep.path, ep.body, func(c *gin.Context) {
					requestContext = c
					ep.call(f.handler, c)
				})
				if allow {
					require.EqualValues(t, 1, f.upstream.calls.Load())
					require.NotContains(t, rec.Body.String(), service.ImageGenerationPermissionMessage())
					require.Positive(t, affinity.invalidated, "a real upstream failure still clears failed affinity")
				} else {
					require.Equal(t, http.StatusBadRequest, rec.Code)
					require.Contains(t, rec.Body.String(), service.ImageGenerationPermissionMessage())
					require.Zero(t, f.upstream.calls.Load())
					require.Equal(t, service.OpsClientBusinessLimitedReasonLocalFeatureGate, service.OpsClientBusinessLimitedReason(requestContext))
					require.Zero(t, affinity.invalidated, "local image permission must not penalize session/user affinity")
				}
				require.True(t, account.Schedulable)
				require.Equal(t, service.StatusActive, account.Status)
				require.Nil(t, account.TempUnschedulableUntil)
			})
		}
	}
}
