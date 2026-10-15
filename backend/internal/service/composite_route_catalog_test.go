package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompositeRouteCatalogExactAliases(t *testing.T) {
	route := func(model, kind, endpoint string, enabled bool) CompositeModelRoute {
		return CompositeModelRoute{GroupID: 7, PublicModel: model, MatchType: kind, Endpoint: endpoint, Enabled: enabled}
	}
	repo := compositeRouteRepoStub{routes: []CompositeModelRoute{
		route("public-any", CompositeRouteMatchExact, CompositeRouteEndpointAny, true),
		route("public-responses", CompositeRouteMatchExact, CompositeRouteEndpointResponses, true),
		route("public-chat", CompositeRouteMatchExact, CompositeRouteEndpointChatCompletions, true),
		route("prefix", CompositeRouteMatchPrefix, CompositeRouteEndpointAny, true),
		route("disabled", CompositeRouteMatchExact, CompositeRouteEndpointAny, false),
		route(" ", CompositeRouteMatchExact, CompositeRouteEndpointAny, true),
		route("literal-*", CompositeRouteMatchExact, CompositeRouteEndpointAny, true),
		{GroupID: 8, PublicModel: "other-group", MatchType: CompositeRouteMatchExact, Enabled: true},
	}}
	r := NewCompositeRouteResolver(repo)
	all, err := r.ListExactPublicModels(context.Background(), 7, "")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"public-any", "public-responses", "public-chat"}, all)
	responses, err := r.ListExactPublicModels(context.Background(), 7, CompositeRouteEndpointResponses)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"public-any", "public-responses"}, responses)
}

func TestCompositeRouteCatalogScopeUsesSelectedMappedTarget(t *testing.T) {
	r := NewCompositeRouteResolver(compositeRouteRepoStub{routes: []CompositeModelRoute{
		{ID: 1, GroupID: 7, PublicModel: "gpt-safe", UpstreamModel: "gpt-5.4", MatchType: CompositeRouteMatchExact, Enabled: true},
		{ID: 2, GroupID: 7, PublicModel: "gpt-cross-family", UpstreamModel: "claude-sonnet-4-6", MatchType: CompositeRouteMatchExact, Enabled: true},
		// Lower-priority same-name route cannot make a forbidden winner visible.
		{ID: 3, GroupID: 7, PublicModel: "gpt-cross-family", UpstreamModel: "gpt-5.4", MatchType: CompositeRouteMatchExact, Priority: 100, Enabled: true},
		{ID: 4, GroupID: 7, PublicModel: "deepseek-public", UpstreamModel: "deepseek-v3.2", MatchType: CompositeRouteMatchExact, Enabled: true},
	}})
	ctx := ContextWithModelScope(context.Background(), ModelScopeOpenAI)
	got, err := r.ListExactPublicModels(ctx, 7, CompositeRouteEndpointResponses)
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-safe"}, got)
	got, err = r.ListExactPublicModels(ContextWithModelScope(context.Background(), ModelScopeCN), 7, "")
	require.NoError(t, err)
	require.Equal(t, []string{"deepseek-public"}, got)
}

type compositeCatalogErrorRepo struct{ CompositeModelRouteRepository }

func (compositeCatalogErrorRepo) ListByGroup(context.Context, int64, bool) ([]CompositeModelRoute, error) {
	return nil, errors.New("route catalog unavailable")
}

func TestCompositeRouteCatalogErrorAndCacheInvalidation(t *testing.T) {
	_, err := NewCompositeRouteResolver(compositeCatalogErrorRepo{}).ListExactPublicModels(context.Background(), 7, "")
	require.ErrorContains(t, err, "unavailable")
	repo := &compositeRouteCountingRepo{routes: []CompositeModelRoute{{GroupID: 7, PublicModel: "before", MatchType: CompositeRouteMatchExact, Enabled: true}}}
	r := NewCompositeRouteResolver(repo)
	got, err := r.ListExactPublicModels(context.Background(), 7, "")
	require.NoError(t, err)
	require.Equal(t, []string{"before"}, got)
	repo.routes[0].PublicModel = "after"
	r.InvalidateGroup(7)
	got, err = r.ListExactPublicModels(context.Background(), 7, "")
	require.NoError(t, err)
	require.Equal(t, []string{"after"}, got)
}

func TestCompositeRouteCatalogScopeDoesNotBorrowAnotherPlatformAccount(t *testing.T) {
	for _, endpoint := range []string{"", CompositeRouteEndpointResponses} {
		for _, safeTarget := range []bool{false, true} {
			mapped := "gpt-5.4"
			if safeTarget {
				mapped = "deepseek-v4"
			}
			repo := &modelScopeAccountRepo{accounts: []Account{
				{Platform: PlatformDeepseek, Credentials: map[string]any{"model_mapping": map[string]any{"deepseek-target": mapped}}},
				{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"openai_passthrough": true}},
			}}
			svc := &GatewayService{accountRepo: repo, compositeResolver: NewCompositeRouteResolver(compositeRouteRepoStub{routes: []CompositeModelRoute{{
				GroupID: 7, PublicModel: "deepseek-public", UpstreamModel: "deepseek-target", TargetPlatform: PlatformDeepseek,
				MatchType: CompositeRouteMatchExact, Enabled: true,
			}}})}
			groupID := int64(7)
			got := svc.FilterModelsForKeyScope(context.Background(), &groupID, ModelScopeCN, []string{"deepseek-public"}, endpoint)
			if safeTarget {
				require.Equal(t, []string{"deepseek-public"}, got)
			} else {
				require.Empty(t, got)
			}
		}
	}
}
