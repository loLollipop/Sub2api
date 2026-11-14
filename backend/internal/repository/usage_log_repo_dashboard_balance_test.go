package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestDashboardEntityStatsIncludesPositiveUserBalances(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count int64
		total float64
	}{
		{name: "funded users", count: 2, total: 14.75000001},
		{name: "no funded users"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := newSQLMock(t)
			repo := newUsageLogRepositoryWithSQL(nil, db)
			now := time.Date(2026, time.November, 14, 12, 0, 0, 0, time.UTC)
			today := now.Truncate(24 * time.Hour)

			// Count and sum must use the same positive-balance population,
			// alongside the existing exclusion of soft-deleted users.
			mock.ExpectQuery(`COUNT\(CASE WHEN balance > 0 THEN 1 END\) as users_with_balance,\s+COALESCE\(SUM\(CASE WHEN balance > 0 THEN balance ELSE 0 END\), 0\) as users_with_balance_total\s+FROM users\s+WHERE deleted_at IS NULL`).
				WithArgs(today).
				WillReturnRows(sqlmock.NewRows([]string{"total_users", "today_new_users", "users_with_balance", "users_with_balance_total"}).
					AddRow(7, 1, tc.count, tc.total))
			mock.ExpectQuery(`FROM api_keys\s+WHERE deleted_at IS NULL`).
				WithArgs(service.StatusActive).
				WillReturnRows(sqlmock.NewRows([]string{"total_api_keys", "active_api_keys"}).AddRow(5, 4))
			mock.ExpectQuery(`FROM accounts\s+WHERE deleted_at IS NULL`).
				WithArgs(service.StatusActive, service.StatusError, now, now).
				WillReturnRows(sqlmock.NewRows([]string{"total_accounts", "normal_accounts", "error_accounts", "ratelimit_accounts", "overload_accounts"}).
					AddRow(3, 2, 1, 0, 0))

			stats := &DashboardStats{}
			require.NoError(t, repo.fillDashboardEntityStats(context.Background(), stats, today, now))
			require.Equal(t, tc.count, stats.UsersWithBalance)
			require.Equal(t, tc.total, stats.UsersWithBalanceTotal)
			require.Equal(t, int64(7), stats.TotalUsers)
			require.Equal(t, int64(4), stats.ActiveAPIKeys)
			require.Equal(t, int64(2), stats.NormalAccounts)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
