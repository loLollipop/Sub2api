package repository

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type capturedSQL struct {
	mu      sync.Mutex
	queries []string
}

func (c *capturedSQL) last() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.queries) == 0 {
		return ""
	}
	return c.queries[len(c.queries)-1]
}

// newCapturingSQLMock matches every expectation by order only and keeps the executed SQL for the test to inspect.
func newCapturingSQLMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *capturedSQL) {
	t.Helper()
	captured := &capturedSQL{}
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(_, actual string) error {
		captured.mu.Lock()
		captured.queries = append(captured.queries, actual)
		captured.mu.Unlock()
		return nil
	})))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, mock, captured
}

func usagePageWindow() (time.Time, time.Time) {
	start := time.Date(2026, 11, 23, 0, 0, 0, 0, time.FixedZone("CST", 8*3600))
	return start, start.Add(48 * time.Hour)
}

func TestUsageStatsScopes(t *testing.T) {
	start, end := usagePageWindow()
	base := usagestats.UsageLogFilters{UserID: 1, StartTime: &start, EndTime: &end, OmitAccountCost: true}

	t.Run("summary: one aggregate row, no endpoint text and no account columns", func(t *testing.T) {
		db, mock, captured := newCapturingSQLMock(t)
		repo := newUsageLogRepositoryWithSQL(nil, db)
		mock.ExpectQuery("x").WillReturnRows(sqlmock.NewRows([]string{"c", "i", "o", "cc", "cr", "cost", "actual", "account", "avg"}).
			AddRow(int64(10), int64(100), int64(50), int64(20), int64(30), 2.5, 2.0, 0.0, 120.5))
		filters := base
		filters.StatsScope = usagestats.StatsScopeSummary

		stats, err := repo.getStatsWithFiltersRaw(context.Background(), filters)

		require.NoError(t, err)
		query := captured.last()
		require.NotContains(t, query, "GROUPING SETS")
		require.NotContains(t, query, "inbound_endpoint")
		require.NotContains(t, query, "upstream_endpoint")
		require.NotContains(t, query, "account_stats_cost")
		require.Contains(t, query, "user_id = $1")
		require.EqualValues(t, 10, stats.TotalRequests)
		require.EqualValues(t, 100, stats.TotalInputTokens)
		require.EqualValues(t, 50, stats.TotalOutputTokens)
		require.EqualValues(t, 50, stats.TotalCacheTokens)
		require.EqualValues(t, 20, stats.TotalCacheCreationTokens)
		require.EqualValues(t, 30, stats.TotalCacheReadTokens)
		require.EqualValues(t, 200, stats.TotalTokens, "input + output + cache")
		require.InDelta(t, 2.5, stats.TotalCost, 1e-9)
		require.InDelta(t, 2.0, stats.TotalActualCost, 1e-9)
		require.InDelta(t, 120.5, stats.AverageDurationMs, 1e-9)
		require.Empty(t, stats.Endpoints)
	})

	t.Run("summary for an admin keeps the account cost expression", func(t *testing.T) {
		db, mock, captured := newCapturingSQLMock(t)
		repo := newUsageLogRepositoryWithSQL(nil, db)
		mock.ExpectQuery("x").WillReturnRows(sqlmock.NewRows([]string{"c", "i", "o", "cc", "cr", "cost", "actual", "account", "avg"}).
			AddRow(int64(1), int64(1), int64(1), int64(0), int64(0), 1.0, 1.0, 0.7, 5.0))
		filters := base
		filters.OmitAccountCost = false
		filters.StatsScope = usagestats.StatsScopeSummary

		stats, err := repo.getStatsWithFiltersRaw(context.Background(), filters)

		require.NoError(t, err)
		require.Contains(t, captured.last(), "account_stats_cost")
		require.InDelta(t, 0.7, *stats.TotalAccountCost, 1e-9)
	})

	t.Run("endpoints: grouped by inbound endpoint only, busiest first, no totals", func(t *testing.T) {
		db, mock, captured := newCapturingSQLMock(t)
		repo := newUsageLogRepositoryWithSQL(nil, db)
		mock.ExpectQuery("x").WillReturnRows(sqlmock.NewRows([]string{"endpoint", "requests", "tokens", "cost", "actual"}).
			AddRow("/v1/chat/completions", int64(3), int64(30), 0.3, 0.2).
			AddRow("/v1/messages", int64(9), int64(90), 0.9, 0.8).
			AddRow("/v1/embeddings", int64(3), int64(5), 0.1, 0.1))
		filters := base
		filters.StatsScope = usagestats.StatsScopeEndpoints

		stats, err := repo.getStatsWithFiltersRaw(context.Background(), filters)

		require.NoError(t, err)
		query := captured.last()
		require.Contains(t, query, "inbound_endpoint")
		require.NotContains(t, query, "upstream_endpoint")
		require.NotContains(t, query, "GROUPING SETS")
		require.NotContains(t, query, "duration_ms")
		require.Contains(t, query, "GROUP BY 1")
		require.Len(t, stats.Endpoints, 3)
		require.Equal(t, "/v1/messages", stats.Endpoints[0].Endpoint, "most requests first")
		require.Equal(t, "/v1/chat/completions", stats.Endpoints[1].Endpoint, "ties broken by name")
		require.Equal(t, "/v1/embeddings", stats.Endpoints[2].Endpoint)
		require.EqualValues(t, 0, stats.TotalRequests)
		require.Empty(t, stats.UpstreamEndpoints)
		require.Empty(t, stats.EndpointPaths)
	})

	t.Run("no scope: the original single-pass grouping sets query is untouched", func(t *testing.T) {
		db, mock, captured := newCapturingSQLMock(t)
		repo := newUsageLogRepositoryWithSQL(nil, db)
		mock.ExpectQuery("x").WillReturnError(errors.New("stop here"))

		_, err := repo.getStatsWithFiltersRaw(context.Background(), usagestats.UsageLogFilters{UserID: 1, StartTime: &start, EndTime: &end})

		require.Error(t, err)
		require.Contains(t, captured.last(), "GROUPING SETS")
		require.Contains(t, captured.last(), "account_stats_cost")
	})
}

func TestStatsFromRollupsHonoursTheScope(t *testing.T) {
	now := time.Now().UTC()
	start := now.Truncate(time.Hour).Add(-time.Hour)
	end := now.Truncate(time.Hour)
	metricCols := []string{
		"bucket_start", "dimension_key", "user_id", "group_id", "endpoint_type",
		"total_requests", "input_tokens", "output_tokens", "cache_creation_tokens", "cache_read_tokens",
		"total_cost", "actual_cost", "account_cost", "duration_count", "total_duration_ms",
	}
	expectCoverage := func(mock sqlmock.Sqlmock) {
		mock.ExpectQuery("SELECT last_aggregated_at FROM usage_dashboard_aggregation_watermark").
			WillReturnRows(sqlmock.NewRows([]string{"last_aggregated_at"}).AddRow(now))
		mock.ExpectQuery(`SELECT COALESCE\(MIN\(bucket_start\)`).
			WillReturnRows(sqlmock.NewRows([]string{"coverage_start"}).AddRow(start))
		mock.ExpectQuery(`SELECT \(`).WillReturnRows(sqlmock.NewRows([]string{"complete"}).AddRow(true))
	}
	filters := func(scope string) usagestats.UsageLogFilters {
		return usagestats.UsageLogFilters{UserID: 7, StartTime: &start, EndTime: &end, OmitAccountCost: true, StatsScope: scope}
	}

	t.Run("summary: reads the user's totals and never asks for endpoint rows", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		expectCoverage(mock)
		mock.ExpectQuery("SELECT bucket_start, dimension_key, user_id, group_id, endpoint_type").
			WillReturnRows(sqlmock.NewRows(metricCols).AddRow(start, "7", int64(7), int64(0), "", int64(4), int64(10), int64(20), int64(2), int64(1), 0.5, 0.4, 0.0, int64(4), int64(400)))
		repo := newUsageLogRepositoryWithSQL(nil, db)
		repo.SetDashboardRollupReadEnabled(true)

		stats, covered, err := repo.GetStatsWithRollups(context.Background(), filters(usagestats.StatsScopeSummary))

		require.NoError(t, err)
		require.True(t, covered)
		require.EqualValues(t, 4, stats.TotalRequests)
		require.InDelta(t, 100.0, stats.AverageDurationMs, 1e-9)
		require.Empty(t, stats.Endpoints)
		require.NoError(t, mock.ExpectationsWereMet(), "exactly one rollup read: the totals")
	})

	t.Run("endpoints: reads the user's endpoint rows and never asks for the totals", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		expectCoverage(mock)
		mock.ExpectQuery("SELECT bucket_start, dimension_key, user_id, group_id, endpoint_type").
			WillReturnRows(sqlmock.NewRows(metricCols).
				AddRow(start, "/v1/messages", int64(7), int64(0), "inbound", int64(9), int64(10), int64(20), int64(2), int64(1), 0.5, 0.4, 0.0, int64(9), int64(900)).
				AddRow(start, "/v1/embeddings", int64(7), int64(0), "inbound", int64(2), int64(1), int64(1), int64(0), int64(0), 0.1, 0.1, 0.0, int64(2), int64(20)))
		repo := newUsageLogRepositoryWithSQL(nil, db)
		repo.SetDashboardRollupReadEnabled(true)

		stats, covered, err := repo.GetStatsWithRollups(context.Background(), filters(usagestats.StatsScopeEndpoints))

		require.NoError(t, err)
		require.True(t, covered)
		require.Len(t, stats.Endpoints, 2)
		require.Equal(t, "/v1/messages", stats.Endpoints[0].Endpoint)
		require.EqualValues(t, 0, stats.TotalRequests)
		require.NoError(t, mock.ExpectationsWereMet(), "exactly one rollup read: the endpoints")
	})

	t.Run("endpoints with no rollup rows is 'not covered', so the raw path answers", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		expectCoverage(mock)
		mock.ExpectQuery("SELECT bucket_start, dimension_key, user_id, group_id, endpoint_type").
			WillReturnRows(sqlmock.NewRows(metricCols))
		repo := newUsageLogRepositoryWithSQL(nil, db)
		repo.SetDashboardRollupReadEnabled(true)

		_, covered, err := repo.GetStatsWithRollups(context.Background(), filters(usagestats.StatsScopeEndpoints))

		require.NoError(t, err)
		require.False(t, covered)
	})
}
