package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGeminiCachedContentRepositoryOwnerAndActualGroups(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &geminiCachedContentRepository{sql: db}
	ctx := context.Background()
	// An empty authorization set is not an unfiltered owner query.
	rows, err := repo.ListForOwner(ctx, 9, nil, time.Now(), 0, 50)
	require.NoError(t, err)
	require.Empty(t, rows)
	now := time.Date(2026, 10, 15, 8, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`(?s)WHERE api_key_id = \$1 AND group_id = ANY\(\$2::bigint\[\]\).*expire_time > \$3.*id < \$4.*LIMIT \$5`).
		WithArgs(int64(9), "{17,23}", now, int64(88), service.GeminiCachedContentListMaxPageSize).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	rows, err = repo.ListForOwner(ctx, 9, []int64{17, 23}, now, 88, 5000)
	require.NoError(t, err)
	require.Empty(t, rows)
	mock.ExpectQuery(`(?s)WHERE public_id = \$1 AND api_key_id = \$2 AND deleted_at IS NULL`).
		WithArgs("private", int64(10)).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	_, err = repo.GetForOwner(ctx, 10, "private")
	require.ErrorIs(t, err, service.ErrGeminiCachedContentNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGeminiCachedContentRepositoryMutationUsesLeaseAndBaseline(t *testing.T) {
	for _, affected := range []int64{0, 1} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })
		repo := &geminiCachedContentRepository{sql: db}
		now := time.Now()
		mock.ExpectExec(`(?s)mutation_token IS NULL AND expire_time = \$4`).
			WithArgs(int64(7), "lease", float64(120), now).WillReturnResult(sqlmock.NewResult(0, affected))
		if affected == 0 {
			mock.ExpectQuery(`SELECT EXISTS`).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		}
		ok, err := repo.BeginMutation(context.Background(), 7, now, "lease", 2*time.Minute)
		require.NoError(t, err)
		require.Equal(t, affected > 0, ok)
		mock.ExpectExec(`(?s)SET expire_time = \$3.*mutation_token = \$2 AND mutation_lease_until > NOW\(\)`).
			WithArgs(int64(7), "lease", now).WillReturnResult(sqlmock.NewResult(0, affected))
		err = repo.CommitMutation(context.Background(), 7, "lease", now)
		if affected == 0 {
			require.ErrorIs(t, err, service.ErrGeminiCachedContentMutationConflict)
		} else {
			require.NoError(t, err)
		}
		mock.ExpectExec(`(?s)SET deleted_at = NOW\(\).*mutation_token = \$2 AND mutation_lease_until > NOW\(\)`).
			WithArgs(int64(7), "lease").WillReturnResult(sqlmock.NewResult(0, affected))
		err = repo.SoftDelete(context.Background(), 7, "lease")
		if affected == 0 {
			require.ErrorIs(t, err, service.ErrGeminiCachedContentMutationConflict)
		} else {
			require.NoError(t, err)
		}
		require.NoError(t, mock.ExpectationsWereMet())
	}
}

func TestGeminiCachedContentRepositoryRejectsInvalidMutation(t *testing.T) {
	repo := &geminiCachedContentRepository{} // Invalid input must never touch SQL.
	for _, lease := range []time.Duration{0, -time.Second, time.Hour} {
		_, err := repo.BeginMutation(context.Background(), 7, time.Now(), "lease", lease)
		require.Error(t, err)
	}
	_, err := repo.TakeOverStaleMutation(context.Background(), 7, "", time.Minute)
	require.Error(t, err)
	require.Error(t, repo.SoftDelete(context.Background(), 7, ""))
	require.Error(t, repo.Create(context.Background(), nil))
	for _, identity := range []string{"", "v1:short", "v2:" + strings.Repeat("a", 64), "v1:" + strings.Repeat("A", 64)} {
		require.Error(t, repo.Create(context.Background(), &service.GeminiCachedContent{
			PublicID: "resource", UserID: 3, APIKeyID: 9, GroupID: 17, AccountID: 21,
			UpstreamName: "cachedContents/cache", Model: "gemini-a", ChannelMappedModel: "gemini-a", UpstreamModel: "models/gemini-a",
			PricingAt: time.Now(), ExpireTime: time.Now().Add(time.Hour), UpstreamEndpointIdentity: identity,
		}))
	}
}

func TestGeminiCachedContentRepositoryRetainsFrozenBindingAndLease(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &geminiCachedContentRepository{sql: db}
	at := time.Date(2026, 10, 15, 8, 0, 0, 0, time.UTC)
	record := &service.GeminiCachedContent{
		PublicID: "resource", UserID: 3, APIKeyID: 9, GroupID: 17, AccountID: 21,
		UpstreamName: "cachedContents/upstream", Model: "gemini-public", ChannelMappedModel: "gemini-channel", UpstreamModel: "models/gemini-live",
		UpstreamEndpointIdentity: "v1:" + strings.Repeat("a", 64),
		TotalTokenCount:          100, ExpireTime: at.Add(time.Hour), PricingAt: at,
		ChannelUsage: service.ChannelUsageFields{ChannelID: 77, OriginalModel: "gemini-public", ChannelMappedModel: "gemini-channel", BillingModelSource: "requested"},
	}
	mock.ExpectQuery(`(?s)INSERT INTO gemini_cached_contents.*channel_mapped_model.*pricing_at.*RETURNING id, created_at, updated_at`).
		WithArgs("resource", int64(3), int64(9), int64(17), int64(21), "cachedContents/upstream", "gemini-public", "gemini-channel", "models/gemini-live", "", int64(100), at.Add(time.Hour), at, sqlmock.AnyArg(), record.UpstreamEndpointIdentity).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(55, at.Add(time.Second), at.Add(time.Second)))
	require.NoError(t, repo.Create(context.Background(), record))
	require.Equal(t, int64(55), record.ID)
	leaseUntil := at.Add(2 * time.Minute)
	mock.ExpectQuery(`(?s)UPDATE gemini_cached_contents.*mutation_token IS NOT NULL AND mutation_lease_until < NOW\(\).*RETURNING`).
		WithArgs(int64(55), "new-lease", float64(120)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "public_id", "user_id", "api_key_id", "group_id", "account_id", "upstream_name", "model", "channel_mapped_model", "upstream_model",
			"display_name", "total_token_count", "expire_time", "pricing_at", "channel_usage", "mutation_token", "mutation_lease_until", "created_at", "updated_at", "upstream_endpoint_identity",
		}).AddRow(55, "resource", 3, 9, 17, 21, "cachedContents/upstream", "gemini-public", "gemini-channel", "models/gemini-live", "", 100, at.Add(time.Hour), at,
			`{"ChannelID":77,"OriginalModel":"gemini-public","ChannelMappedModel":"gemini-channel","BillingModelSource":"requested"}`, "new-lease", leaseUntil, at.Add(time.Second), at.Add(time.Second), record.UpstreamEndpointIdentity))
	taken, err := repo.TakeOverStaleMutation(context.Background(), 55, "new-lease", 2*time.Minute)
	require.NoError(t, err)
	require.Equal(t, record.GroupID, taken.GroupID, "actual secondary group must survive recovery")
	require.Equal(t, record.PricingAt, taken.PricingAt)
	require.Equal(t, record.UpstreamEndpointIdentity, taken.UpstreamEndpointIdentity)
	require.Equal(t, record.ChannelUsage, taken.ChannelUsage)
	require.Equal(t, "new-lease", taken.MutationToken)
	require.Equal(t, leaseUntil, *taken.MutationLeaseUntil)
	require.NoError(t, mock.ExpectationsWereMet())
}
