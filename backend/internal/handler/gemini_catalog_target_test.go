//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type geminiCatalogChannelRepo struct {
	service.ChannelRepository
	channel  service.Channel
	platform string
}

func (r geminiCatalogChannelRepo) ListAll(context.Context) ([]service.Channel, error) {
	return []service.Channel{r.channel}, nil
}
func (r geminiCatalogChannelRepo) GetGroupPlatforms(context.Context, []int64) (map[int64]string, error) {
	return map[int64]string{45: r.platform}, nil
}

type geminiCatalogRecordingUpstream struct {
	service.HTTPUpstream
	paths []string
}

func (u *geminiCatalogRecordingUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.paths = append(u.paths, req.URL.Path)
	body := `{"name":"models/gemini-live","inputTokenLimit":123,"custom":{"retained":true}}`
	if req.URL.Path == "/v1beta/models" {
		body = `{"models":[` + body + `],"nextPageToken":"next","customEnvelope":42}`
	}
	return (&geminiMixedModelsUpstream{status: 200, body: body}).Do(req, "", 0, 0)
}

func newGeminiCatalogProjectionHandler(platform string, routes []service.CompositeModelRoute, mapping map[string]map[string]string) (*GatewayHandler, *geminiCatalogRecordingUpstream) {
	repo := &geminiAllowlistAccountRepoStub{gatewayModelsAccountRepoStub: gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{45: {
		{ID: 1, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test", "model_mapping": map[string]any{"gemini-live": "gemini-live"}}},
	}}}}
	var channel *service.ChannelService
	if mapping != nil {
		channel = service.NewChannelService(geminiCatalogChannelRepo{platform: platform, channel: service.Channel{
			ID: 1, Status: service.StatusActive, GroupIDs: []int64{45}, ModelMapping: mapping,
		}}, nil, nil, nil, nil)
	}
	var resolver *service.CompositeRouteResolver
	if routes != nil {
		resolver = service.NewCompositeRouteResolver(gatewayCompositeCatalogRepo{routes: routes})
	}
	upstream := &geminiCatalogRecordingUpstream{}
	return &GatewayHandler{
		geminiCompatService: service.NewGeminiMessagesCompatService(repo, nil, nil, nil, nil, nil, upstream, nil, &config.Config{}),
		gatewayService: service.NewGatewayService(repo, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, channel, nil, resolver, nil, nil),
	}, upstream
}

func TestGeminiNativeCatalogChannelAndCompositeAliases(t *testing.T) {
	for _, composite := range []bool{false, true} {
		platform := service.PlatformGemini
		mapping := map[string]map[string]string{platform: {"gemini-public": "gemini-live"}}
		var routes []service.CompositeModelRoute
		if composite {
			platform = service.PlatformComposite
			mapping = map[string]map[string]string{service.PlatformGemini: {"gemini-channel": "gemini-live"}, service.PlatformAntigravity: {"gemini-channel": "gemini-wrong"}}
			routes = []service.CompositeModelRoute{{GroupID: 45, PublicModel: "gemini-public", UpstreamModel: "gemini-channel", TargetPlatform: service.PlatformGemini, MatchType: service.CompositeRouteMatchExact, Endpoint: service.CompositeRouteEndpointGemini, Enabled: true}}
		}
		for _, detail := range []bool{false, true} {
			h, upstream := newGeminiCatalogProjectionHandler(platform, routes, mapping)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
			// The actual routing middleware defaults list requests to Gemini
			// before a per-model Composite target is known.
			c.Request = c.Request.WithContext(service.WithResolvedTargetPlatform(c.Request.Context(), service.PlatformGemini))
			id := int64(45)
			c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{GroupID: &id, ModelScope: service.ModelScopeOther, Group: &service.Group{ID: id, Platform: platform,
				ModelAllowlist: service.GroupModelAllowlist{Enabled: true, Models: []string{"gemini-public"}},
			}})
			if detail {
				c.Params = gin.Params{{Key: "model", Value: "gemini-public"}}
				h.GeminiV1BetaGetModel(c)
				require.Equal(t, []string{"/v1beta/models/gemini-live"}, upstream.paths)
			} else {
				h.GeminiV1BetaListModels(c)
				require.Contains(t, rec.Body.String(), `"nextPageToken":"next"`)
				require.Contains(t, rec.Body.String(), `"customEnvelope":42`)
			}
			require.Equal(t, 200, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), `"name":"models/gemini-public"`)
			require.Contains(t, rec.Body.String(), `"inputTokenLimit":123`)
			require.Contains(t, rec.Body.String(), `"custom":{"retained":true}`)
			require.NotContains(t, rec.Body.String(), `"name":"models/gemini-live"`)
		}
	}
}

func TestGeminiNativeCatalogCannotBorrowAnotherPlatformMetadata(t *testing.T) {
	routes := []service.CompositeModelRoute{{GroupID: 45, PublicModel: "gemini-public", UpstreamModel: "gemini-live", TargetPlatform: service.PlatformAntigravity, MatchType: service.CompositeRouteMatchExact, Enabled: true}}
	for _, detail := range []bool{false, true} {
		h, upstream := newGeminiCatalogProjectionHandler(service.PlatformComposite, routes, nil)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
		id := int64(45)
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{GroupID: &id, Group: &service.Group{ID: id, Platform: service.PlatformComposite,
			ModelAllowlist: service.GroupModelAllowlist{Enabled: true, Models: []string{"gemini-public"}},
		}})
		if detail {
			c.Params = gin.Params{{Key: "model", Value: "gemini-public"}}
			h.GeminiV1BetaGetModel(c)
			require.Equal(t, 404, rec.Code)
			require.Empty(t, upstream.paths)
		} else {
			h.GeminiV1BetaListModels(c)
			require.Equal(t, 200, rec.Code)
			require.NotContains(t, rec.Body.String(), "gemini-public")
		}
	}
}

func TestGeminiNativeCatalogColdBoundGroupPolicyFailsClosed(t *testing.T) {
	repo := &geminiAllowlistAccountRepoStub{gatewayModelsAccountRepoStub: gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{46: {
		{ID: 1, Platform: service.PlatformAntigravity, Extra: map[string]any{"mixed_scheduling": true}, Credentials: map[string]any{"model_mapping": map[string]any{"gemini-hidden": "gemini-live"}}},
	}}}}
	h := &GatewayHandler{geminiCompatService: service.NewGeminiMessagesCompatService(repo, nil, nil, nil, nil, nil, nil, nil, &config.Config{})}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
	id := int64(45)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{GroupID: &id, RouteGroupIDs: []int64{45, 46}, Group: &service.Group{ID: id, Platform: service.PlatformGemini}})
	h.GeminiV1BetaListModels(c)
	require.Equal(t, 200, rec.Code)
	require.NotContains(t, rec.Body.String(), "gemini-hidden")
}

func TestGeminiNativeCatalogMiddlewareDefaultCannotOverrideCompositeRoute(t *testing.T) {
	routes := []service.CompositeModelRoute{{GroupID: 45, PublicModel: "gemini-live", UpstreamModel: "gemini-ag", TargetPlatform: service.PlatformAntigravity, MatchType: service.CompositeRouteMatchExact, Enabled: true}}
	h, _ := newGeminiCatalogProjectionHandler(service.PlatformComposite, routes, nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
	c.Request = c.Request.WithContext(service.WithResolvedTargetPlatform(c.Request.Context(), service.PlatformGemini))
	id := int64(45)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{GroupID: &id, Group: &service.Group{ID: id, Platform: service.PlatformComposite}})
	h.GeminiV1BetaListModels(c)
	require.Equal(t, 200, rec.Code)
	require.NotContains(t, rec.Body.String(), "gemini-live")
}

func TestGeminiNativeCatalogPrefixChecksIntermediateModelScope(t *testing.T) {
	routes := []service.CompositeModelRoute{{GroupID: 45, PublicModel: "gemini-", UpstreamModel: "gpt-5.4", TargetPlatform: service.PlatformGemini, MatchType: service.CompositeRouteMatchPrefix, Enabled: true}}
	mapping := map[string]map[string]string{service.PlatformGemini: {"gemini-public": "gemini-live", "gpt-5.4": "gemini-live"}}
	for _, detail := range []bool{false, true} {
		h, upstream := newGeminiCatalogProjectionHandler(service.PlatformComposite, routes, mapping)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
		id := int64(45)
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{GroupID: &id, ModelScope: service.ModelScopeOther, Group: &service.Group{ID: id, Platform: service.PlatformComposite,
			ModelAllowlist: service.GroupModelAllowlist{Enabled: true, Models: []string{"gemini-public"}},
		}})
		if detail {
			c.Params = gin.Params{{Key: "model", Value: "gemini-public"}}
			h.GeminiV1BetaGetModel(c)
			require.Equal(t, 404, rec.Code)
			require.Empty(t, upstream.paths)
		} else {
			h.GeminiV1BetaListModels(c)
			require.Equal(t, 200, rec.Code)
			require.NotContains(t, rec.Body.String(), "gemini-public")
		}
	}
}

func TestGeminiNativeModelDetailDoesNotInventSuccess(t *testing.T) {
	for _, forced := range []bool{false, true} {
		groupID := int64(45)
		repo := &geminiAllowlistAccountRepoStub{}
		h := &GatewayHandler{geminiCompatService: service.NewGeminiMessagesCompatService(repo, nil, nil, nil, nil, nil, nil, nil, &config.Config{})}
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models/gemini-invented", nil)
		c.Params = gin.Params{{Key: "model", Value: "gemini-invented"}}
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: service.PlatformGemini}})
		if forced {
			c.Set(string(middleware.ContextKeyForcePlatform), service.PlatformAntigravity)
		}
		h.GeminiV1BetaGetModel(c)
		require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	}
}

func TestGeminiNativeModelDetailPreservesRealUpstreamFailure(t *testing.T) {
	id := int64(45)
	repo := &geminiAllowlistAccountRepoStub{gatewayModelsAccountRepoStub: gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{id: {
		{ID: 1, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test"}},
	}}}}
	upstream := &geminiMixedModelsUpstream{status: 403, body: `{"error":{"code":403,"message":"scope denied"}}`}
	h := &GatewayHandler{geminiCompatService: service.NewGeminiMessagesCompatService(repo, nil, nil, nil, nil, nil, upstream, nil, &config.Config{})}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models/gemini-2.5-pro", nil)
	c.Params = gin.Params{{Key: "model", Value: "gemini-2.5-pro"}}
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{GroupID: &id, Group: &service.Group{ID: id, Platform: service.PlatformGemini}})
	h.GeminiV1BetaGetModel(c)
	require.Equal(t, 403, rec.Code, rec.Body.String())
	require.JSONEq(t, upstream.body, rec.Body.String())
}

func TestGeminiNativeCatalogUsesOpenAICompatibleExplicitMappingsOnly(t *testing.T) {
	id := int64(45)
	repo := &geminiAllowlistAccountRepoStub{gatewayModelsAccountRepoStub: gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{id: {
		{ID: 1, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test", "api_protocol": service.APIProtocolChatCompletions, "model_mapping": map[string]any{
			"gemini-live": "gemini-3.8-flash", "gpt-not-native": "gpt-5.4", "gemini-unsafe": "gemini-test/../x",
		}}},
	}}}}
	h := &GatewayHandler{geminiCompatService: service.NewGeminiMessagesCompatService(repo, nil, nil, nil, nil, nil, nil, nil, &config.Config{})}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{GroupID: &id, Group: &service.Group{ID: id, Platform: service.PlatformGemini}})
	h.GeminiV1BetaListModels(c)
	require.Equal(t, 200, rec.Code, rec.Body.String())
	var body struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Models, 1)
	require.Equal(t, "models/gemini-live", body.Models[0].Name)
}
