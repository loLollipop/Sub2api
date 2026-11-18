package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

func TestUserUsageTrendRollupReaderMergesHourlyRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	start := now.Truncate(time.Hour).Add(-time.Hour)
	end := now.Truncate(time.Hour)

	mock.ExpectQuery("SELECT last_aggregated_at FROM usage_dashboard_aggregation_watermark").
		WillReturnRows(sqlmock.NewRows([]string{"last_aggregated_at"}).AddRow(now))
	mock.ExpectQuery(`SELECT COALESCE\(MIN\(bucket_start\)`).
		WillReturnRows(sqlmock.NewRows([]string{"coverage_start"}).AddRow(start))
	mock.ExpectQuery(`SELECT \(`).
		WillReturnRows(sqlmock.NewRows([]string{"complete"}).AddRow(true))
	mock.ExpectQuery("SELECT bucket_start, dimension_key, user_id, group_id, endpoint_type").
		WillReturnRows(sqlmock.NewRows([]string{
			"bucket_start", "dimension_key", "user_id", "group_id", "endpoint_type",
			"total_requests", "input_tokens", "output_tokens", "cache_creation_tokens", "cache_read_tokens",
			"total_cost", "actual_cost", "account_cost", "duration_count", "total_duration_ms",
		}).AddRow(start, "7", int64(7), int64(0), "", int64(3), int64(10), int64(20), int64(2), int64(1), 0.5, 0.4, 0.6, int64(2), int64(100)))
	mock.ExpectQuery(`SELECT id, COALESCE\(email,''\), COALESCE\(username,''\)`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "username"}).AddRow(int64(7), "u@example.com", "user"))

	repo := newUsageLogRepositoryWithSQL(nil, db)
	repo.SetDashboardRollupReadEnabled(true)
	result, covered, err := repo.GetUserUsageTrendWithRollups(context.Background(), start, end, "hour", 12)
	require.NoError(t, err)
	require.True(t, covered)
	require.Len(t, result, 1)
	require.Equal(t, int64(7), result[0].UserID)
	require.Equal(t, int64(3), result[0].Requests)
	require.Equal(t, int64(33), result[0].Tokens)
	require.Equal(t, "u@example.com", result[0].Email)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardRollupReadDisabledFallsBackToRawPath(t *testing.T) {
	repo := newUsageLogRepositoryWithSQL(nil, nil)
	repo.SetDashboardRollupReadEnabled(false)
	_, _, ok := repo.rollupBounds(context.Background(), time.Now().Add(-time.Hour), time.Now())
	require.False(t, ok)
}

func TestMergeUsageStatsLeavesAverageDurationForExplicitSummary(t *testing.T) {
	dst := &UsageStats{
		TotalRequests:     10,
		AverageDurationMs: 100,
	}
	src := &UsageStats{
		TotalRequests:     10,
		AverageDurationMs: 200,
	}

	mergeUsageStats(dst, src)

	// Requests with NULL duration are included in TotalRequests but excluded
	// from the duration denominator. The caller must use duration sum/count,
	// rather than reverse-engineering an average from request totals.
	require.Equal(t, int64(20), dst.TotalRequests)
	require.Equal(t, 100.0, dst.AverageDurationMs)
}

func TestUsageStatsRollupEndpointsKeepAccountCostAcrossHoursAndRawTail(t *testing.T) {
	for _, withTail := range []bool{false, true} {
		t.Run(map[bool]string{false: "whole_hours", true: "with_raw_tail"}[withTail], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			now := time.Now().UTC()
			cutoff := now.Truncate(time.Hour)
			start, end := cutoff.Add(-2*time.Hour), cutoff
			if withTail {
				end = now
			}
			mock.ExpectQuery("SELECT last_aggregated_at FROM usage_dashboard_aggregation_watermark").
				WillReturnRows(sqlmock.NewRows([]string{"watermark"}).AddRow(now))
			mock.ExpectQuery(`SELECT COALESCE\(MIN\(bucket_start\)`).
				WillReturnRows(sqlmock.NewRows([]string{"coverage_start"}).AddRow(start))
			mock.ExpectQuery(`SELECT \(`).
				WillReturnRows(sqlmock.NewRows([]string{"complete"}).AddRow(true))
			mock.ExpectQuery("SELECT bucket_start, total_requests, input_tokens").
				WillReturnRows(sqlmock.NewRows([]string{"bucket_start", "requests", "input", "output", "cache_write", "cache_read", "cost", "actual", "account", "duration_count", "duration_ms"}).
					AddRow(start, 1, 2, 3, 0, 0, 1, .15, .4, 1, 40).
					AddRow(start.Add(time.Hour), 1, 2, 3, 0, 0, 1, .15, .4, 1, 40))
			endpointRows := sqlmock.NewRows([]string{"bucket_start", "dimension_key", "user_id", "group_id", "endpoint_type", "requests", "input", "output", "cache_write", "cache_read", "cost", "actual", "account", "duration_count", "duration_ms"})
			for _, at := range []time.Time{start, start.Add(time.Hour)} {
				for _, endpoint := range []struct{ kind, key string }{{"inbound", "/v1/messages"}, {"upstream", "/v1/responses"}, {"path", "/v1/messages -> /v1/responses"}} {
					endpointRows.AddRow(at, endpoint.key, 0, 0, endpoint.kind, 1, 2, 3, 0, 0, 1, .15, .4, 1, 40)
				}
			}
			mock.ExpectQuery("SELECT bucket_start, dimension_key, user_id, group_id, endpoint_type").WillReturnRows(endpointRows)
			if withTail {
				mock.ExpectQuery("(?s)FROM usage_logs.*GROUP BY GROUPING SETS").
					WithArgs(cutoff, end).
					WillReturnRows(sqlmock.NewRows([]string{"inbound_grouped", "upstream_grouped", "inbound_endpoint", "upstream_endpoint", "requests", "input", "output", "cache_write", "cache_read", "cost", "actual", "account", "avg_duration"}).
						AddRow(1, 1, nil, nil, 1, 2, 3, 0, 0, 1, .15, .4, 40).
						AddRow(0, 1, "/v1/messages", nil, 1, 2, 3, 0, 0, 1, .15, .4, 40).
						AddRow(1, 0, nil, "/v1/responses", 1, 2, 3, 0, 0, 1, .15, .4, 40).
						AddRow(0, 0, "/v1/messages", "/v1/responses", 1, 2, 3, 0, 0, 1, .15, .4, 40))
				mock.ExpectQuery(`SELECT COALESCE\(SUM\(duration_ms\), 0\), COUNT\(duration_ms\)`).
					WithArgs(cutoff, end).WillReturnRows(sqlmock.NewRows([]string{"duration", "count"}).AddRow(40, 1))
			}
			repo := newUsageLogRepositoryWithSQL(nil, db)
			repo.SetDashboardRollupReadEnabled(true)
			stats, covered, err := repo.GetStatsWithRollups(context.Background(), UsageLogFilters{StartTime: &start, EndTime: &end})
			require.NoError(t, err)
			require.True(t, covered)
			count := 2
			if withTail {
				count++
			}
			require.InDelta(t, .15*float64(count), stats.TotalActualCost, 1e-12)
			require.InDelta(t, .4*float64(count), *stats.TotalAccountCost, 1e-12)
			for _, endpoints := range [][]usagestats.EndpointStat{stats.Endpoints, stats.UpstreamEndpoints, stats.EndpointPaths} {
				require.Len(t, endpoints, 1, "hours and tail must consolidate to one row per endpoint")
				require.Equal(t, int64(count), endpoints[0].Requests)
				require.InDelta(t, .15*float64(count), endpoints[0].ActualCost, 1e-12)
				require.InDelta(t, .4*float64(count), endpoints[0].AccountCost, 1e-12)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
