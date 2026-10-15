package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type gatewayCompositeCatalogRepo struct {
	service.CompositeModelRouteRepository
	routes []service.CompositeModelRoute
	err    error
}

func (r gatewayCompositeCatalogRepo) ListByGroup(_ context.Context, groupID int64, _ bool) ([]service.CompositeModelRoute, error) {
	var routes []service.CompositeModelRoute
	for _, route := range r.routes {
		if route.GroupID == groupID {
			routes = append(routes, route)
		}
	}
	return routes, r.err
}

func newCompositeCatalogHandler(t *testing.T, routes service.CompositeModelRouteRepository) *GatewayHandler {
	t.Helper()
	return &GatewayHandler{gatewayService: service.NewGatewayService(
		&gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{
			7: {{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
				Credentials: map[string]any{"model_mapping": map[string]any{"gpt-existing": "gpt-existing"}}}},
		}},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		service.NewCompositeRouteResolver(routes), nil, nil,
	)}
}

func TestGatewayModelsCompositeExactRouteCatalog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	route := func(id, endpoint, target string) service.CompositeModelRoute {
		return service.CompositeModelRoute{GroupID: 7, PublicModel: id, UpstreamModel: target,
			MatchType: service.CompositeRouteMatchExact, Endpoint: endpoint, Enabled: true}
	}
	routes := []service.CompositeModelRoute{
		route("gpt-route", service.CompositeRouteEndpointAny, "gpt-existing"),
		route("gpt-responses", service.CompositeRouteEndpointResponses, "gpt-existing"),
		route("gpt-chat", service.CompositeRouteEndpointChatCompletions, "gpt-existing"),
		route("gpt-hidden-target", service.CompositeRouteEndpointAny, "claude-sonnet-4-6"),
		route("deepseek-v3.2", service.CompositeRouteEndpointAny, "deepseek-v3.2"),
		{GroupID: 8, PublicModel: "gpt-other-group", MatchType: service.CompositeRouteMatchExact, Enabled: true},
		{GroupID: 7, PublicModel: "gpt-disabled", MatchType: service.CompositeRouteMatchExact},
		{GroupID: 7, PublicModel: "gpt-prefix", MatchType: service.CompositeRouteMatchPrefix, Enabled: true},
	}
	for _, tc := range []struct {
		name      string
		codex     bool
		scope     string
		allowlist []string
		want      []string
	}{
		{name: "ordinary", want: []string{"gpt-existing", "gpt-route", "gpt-responses", "gpt-chat", "gpt-hidden-target", "deepseek-v3.2"}},
		{name: "codex endpoint", codex: true, want: []string{"gpt-existing", "gpt-route", "gpt-responses", "gpt-hidden-target", "deepseek-v3.2"}},
		{name: "openai scope", scope: service.ModelScopeOpenAI, want: []string{"gpt-existing", "gpt-route", "gpt-responses", "gpt-chat"}},
		{name: "codex openai scope", codex: true, scope: service.ModelScopeOpenAI, want: []string{"gpt-existing", "gpt-route", "gpt-responses"}},
		{name: "allowlist", allowlist: []string{"gpt-route"}, want: []string{"gpt-route"}},
		{name: "codex allowlist", codex: true, allowlist: []string{"gpt-route", "gpt-chat"}, want: []string{"gpt-route"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newCompositeCatalogHandler(t, gatewayCompositeCatalogRepo{routes: routes})
			group := &service.Group{ID: 7, Platform: service.PlatformComposite}
			if tc.allowlist != nil {
				group.ModelAllowlist = service.GroupModelAllowlist{Enabled: true, Models: tc.allowlist}
			}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{Group: group, ModelScope: tc.scope})
			var ids []string
			if tc.codex {
				h.CodexModels(c)
				var got codexModelsResponseForTest
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
				ids = codexModelSlugsForTest(got.Models)
			} else {
				h.Models(c)
				var got gatewayModelsResponseForTest
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
				ids = modelIDsForTest(got.Data)
			}
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.ElementsMatch(t, tc.want, ids)
		})
	}
}

func TestGatewayModelsCompositeRouteRepositoryFailureKeepsAccounts(t *testing.T) {
	h := newCompositeCatalogHandler(t, gatewayCompositeCatalogRepo{err: errors.New("unavailable")})
	ids := h.availableModelsForAPIKey(context.Background(), &service.APIKey{
		Group: &service.Group{ID: 7, Platform: service.PlatformComposite},
	}, "")
	require.Equal(t, []string{"gpt-existing"}, ids)
}
