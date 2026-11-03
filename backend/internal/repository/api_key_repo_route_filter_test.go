package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyRepositoryGroupFilterIncludesSecondaryRoutes(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	_, err := repo.sql.ExecContext(ctx, `CREATE TABLE api_key_group_routes (api_key_id INTEGER NOT NULL, group_id INTEGER NOT NULL, position INTEGER NOT NULL)`)
	require.NoError(t, err)
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "route-filter@test.com")
	other := mustCreateAPIKeyRepoUser(t, ctx, client, "other-route-filter@test.com")
	primary, err := client.Group.Create().SetName("route-primary").Save(ctx)
	require.NoError(t, err)
	secondary, err := client.Group.Create().SetName("route-secondary").Save(ctx)
	require.NoError(t, err)
	makeKey := func(name string, owner int64, groupID *int64) *service.APIKey {
		key := &service.APIKey{UserID: owner, Key: "sk-" + name, Name: name, Status: service.StatusActive, GroupID: groupID}
		require.NoError(t, repo.Create(ctx, key))
		return key
	}
	smart := makeKey("smart", user.ID, &primary.ID)
	plain := makeKey("plain", user.ID, &secondary.ID)
	_ = makeKey("unrelated", user.ID, &primary.ID)
	_ = makeKey("unbound", user.ID, nil)
	foreign := makeKey("foreign", other.ID, &primary.ID)
	deleted := makeKey("deleted", user.ID, &primary.ID)
	for _, key := range []*service.APIKey{smart, foreign, deleted} {
		require.NoError(t, repo.ReplaceGroupRoutes(ctx, key.ID, []int64{primary.ID, secondary.ID}))
	}
	require.NoError(t, repo.Delete(ctx, deleted.ID))
	// Exercise the real count/paging predicate with SQLite. The independent
	// post-read route hydration uses PostgreSQL ANY and is outside this test.
	repo.sql = nil
	filters := service.APIKeyListFilters{GroupID: &secondary.ID}
	for page := 1; page <= 2; page++ {
		keys, result, err := repo.ListByUserID(ctx, user.ID, pagination.PaginationParams{Page: page, PageSize: 1, SortBy: "id", SortOrder: "asc"}, filters)
		require.NoError(t, err)
		require.Equal(t, int64(2), result.Total, "matching primary and route must not duplicate keys or cross owner/deleted boundaries")
		require.Len(t, keys, 1)
		require.Equal(t, []int64{smart.ID, plain.ID}[page-1], keys[0].ID)
	}
	all, err := repo.ListAllByUserID(ctx, user.ID, filters)
	require.NoError(t, err)
	require.Len(t, all, 2, "concurrency-sorted listings use the same predicate")
	filters.Search = "smart"
	all, err = repo.ListAllByUserID(ctx, user.ID, filters)
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.Equal(t, smart.ID, all[0].ID)
	filters = service.APIKeyListFilters{GroupID: new(int64)}
	all, err = repo.ListAllByUserID(ctx, user.ID, filters)
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.Equal(t, "unbound", all[0].Name)
}
