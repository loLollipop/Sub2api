package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

func TestLeaderboardUsageQueryUsesBoundedRetainedRawLogs(t *testing.T) {
	matcher := sqlmock.QueryMatcherFunc(func(expected, actual string) error {
		// Read a single source with explicit, indexable half-open boundaries.
		for _, fragment := range []string{
			"FROM usage_logs", "created_at >= $1 AND created_at < $2",
			"COALESCE(NULLIF(TRIM(requested_model), ''), model)", "input_tokens::bigint + output_tokens + cache_creation_tokens + cache_read_tokens",
			"date_bin(interval '1 hour'", "date_trunc($3", "GROUP BY 1, 2 ORDER BY 1, 2",
		} {
			require.Contains(t, actual, fragment)
		}
		for _, private := range []string{"upstream_model", "user_id", "api_key_id", "account_id", "total_cost"} {
			require.NotContains(t, actual, private)
		}
		for _, rollup := range []string{"usage_dashboard_", "computed_at", "NOW()", "$5", "UNION"} {
			require.NotContains(t, actual, rollup, "do not mix retained raw logs with stale or ambiguously bounded rollups")
		}
		return nil
	})
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		require.NoError(t, db.Close())
	})
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(1, 0, 0)
	mock.ExpectQuery("leaderboard").WithArgs(start, end, "month", timezone.Location().String()).
		WillReturnRows(sqlmock.NewRows([]string{"date", "model", "tokens", "requests"}).AddRow(start, "requested", int64(150), int64(3)))
	repo := newUsageLogRepositoryWithSQL(nil, db)
	repo.SetDashboardRollupReadEnabled(true)
	rows, err := repo.GetLeaderboardUsage(context.Background(), start, end, "month")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "requested", rows[0].Model)
	require.Equal(t, int64(150), rows[0].TotalTokens)
	require.Equal(t, int64(3), rows[0].Requests)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLeaderboardUsageEmptyInvalidAndQueryFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		require.NoError(t, db.Close())
	})
	repo := newUsageLogRepositoryWithSQL(nil, db)
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	rows, err := repo.GetLeaderboardUsage(context.Background(), start, start, "day")
	require.NoError(t, err)
	require.NotNil(t, rows)
	_, err = repo.GetLeaderboardUsage(context.Background(), start, start.Add(time.Hour), "unsafe")
	require.ErrorContains(t, err, "granularity")
	_, err = repo.GetLeaderboardUsage(context.Background(), start, start.AddDate(2, 0, 0), "month")
	require.ErrorContains(t, err, "one year")
	mock.ExpectQuery("SELECT CASE").WithArgs(start, start.Add(time.Hour), "hour", timezone.Location().String()).WillReturnError(errors.New("query failed"))
	_, err = repo.GetLeaderboardUsage(context.Background(), start, start.Add(time.Hour), "hour")
	require.EqualError(t, err, "query failed")
	require.NoError(t, mock.ExpectationsWereMet())
}
