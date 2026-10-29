package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestAccountWindowStatsByStartParameterizedQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	start1 := time.Date(2026, 10, 28, 1, 2, 3, 0, time.UTC)
	start2 := start1.Add(3 * time.Hour)
	// Bind each distinct start to its own account. Cost formulas match the
	// single-account reader, including the standard cost used by the admin list.
	mock.ExpectQuery(`(?s)SELECT.*u.account_id.*COUNT\(\*\).*SUM\(u.input_tokens \+ u.output_tokens \+ u.cache_creation_tokens \+ u.cache_read_tokens\).*SUM\(COALESCE\(u.account_stats_cost, u.total_cost\) \* COALESCE\(u.account_rate_multiplier, 1\)\).*SUM\(u.total_cost\).*SUM\(u.actual_cost\).*FROM usage_logs u.*JOIN \(VALUES \(\$1::bigint, \$2::timestamptz\),\(\$3::bigint, \$4::timestamptz\)\) AS w\(account_id, start_at\) ON u.account_id = w.account_id.*WHERE u.created_at >= w.start_at.*GROUP BY u.account_id`).
		WithArgs(int64(11), start1, int64(22), start2).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "requests", "tokens", "cost", "standard_cost", "user_cost"}).AddRow(11, 3, 60, 9.0, 4.5, 3.0))
	stats, err := (&usageLogRepository{sql: db}).GetAccountWindowStatsByStart(context.Background(), map[int64]time.Time{22: start2, 11: start1})
	require.NoError(t, err)
	require.Len(t, stats, 2)
	require.EqualValues(t, 3, stats[11].Requests)
	require.EqualValues(t, 60, stats[11].Tokens)
	require.Equal(t, 9.0, stats[11].Cost)
	require.Equal(t, 4.5, stats[11].StandardCost)
	require.Equal(t, 3.0, stats[11].UserCost)
	require.Zero(t, *stats[22])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountWindowStatsByStartEmptyAndDatabaseErrors(t *testing.T) {
	empty, err := (&usageLogRepository{}).GetAccountWindowStatsByStart(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, empty)
	for _, rowError := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		query := mock.ExpectQuery(`SELECT`)
		failure := errors.New("database failure")
		if rowError {
			query.WillReturnRows(sqlmock.NewRows([]string{"account_id", "requests", "tokens", "cost", "standard_cost", "user_cost"}).AddRow(1, 1, 1, 1, 1, 1).RowError(0, failure))
		} else {
			query.WillReturnError(failure)
		}
		stats, err := (&usageLogRepository{sql: db}).GetAccountWindowStatsByStart(context.Background(), map[int64]time.Time{1: time.Now()})
		require.ErrorIs(t, err, failure)
		require.Nil(t, stats)
		require.NoError(t, mock.ExpectationsWereMet())
		_ = db.Close()
	}
}
