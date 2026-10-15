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
