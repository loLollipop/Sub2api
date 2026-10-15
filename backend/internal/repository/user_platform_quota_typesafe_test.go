package repository

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestTypeSafeQuotaSnapshotUsesCommonFlusher(t *testing.T) {
	db, mock := newSQLMock(t)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	repo := &userPlatformQuotaRepository{client: client}
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	mock.ExpectExec("INSERT INTO user_platform_quotas").
		WithArgs(now, int64(7), domain.PlatformTypeSafe, 1.25, 2.5, 3.75, now, now, now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.BatchSnapshotUsage(context.Background(), []UserPlatformQuotaSnapshot{{
		UserID: 7, Platform: domain.PlatformTypeSafe,
		DailyUsageUSD: 1.25, WeeklyUsageUSD: 2.5, MonthlyUsageUSD: 3.75,
		DailyWindowStart: now, WeeklyWindowStart: now, MonthlyWindowStart: now,
	}}, now))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTypeSafeQuotaSnapshotPostgresPersistsAfterRepositoryReload(t *testing.T) {
	db, ctx := isolatedPostgres(t)
	_, err := db.ExecContext(ctx, `CREATE TABLE user_platform_quotas (
user_id bigint, platform text, daily_usage_usd double precision,
weekly_usage_usd double precision, monthly_usage_usd double precision,
daily_window_start timestamptz, weekly_window_start timestamptz, monthly_window_start timestamptz,
created_at timestamptz, updated_at timestamptz, deleted_at timestamptz);
CREATE UNIQUE INDEX quota_active_uq ON user_platform_quotas(user_id, platform) WHERE deleted_at IS NULL;`)
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	repo := &userPlatformQuotaRepository{client: client}
	now := time.Now().UTC().Truncate(time.Microsecond)
	snapshot := UserPlatformQuotaSnapshot{UserID: 7, Platform: domain.PlatformTypeSafe,
		DailyUsageUSD: 1.25, WeeklyUsageUSD: 2.5, MonthlyUsageUSD: 3.75,
		DailyWindowStart: now, WeeklyWindowStart: now, MonthlyWindowStart: now}
	for range 2 {
		require.NoError(t, repo.BatchSnapshotUsage(ctx, []UserPlatformQuotaSnapshot{snapshot}, now))
	}
	// Read independently from storage: Redis eviction/repository recreation cannot
	// restore an old usage value, and repeated snapshots are not cumulative.
	var daily, weekly, monthly float64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT daily_usage_usd, weekly_usage_usd, monthly_usage_usd
FROM user_platform_quotas WHERE user_id = 7 AND platform = 'typesafe'`).Scan(&daily, &weekly, &monthly))
	require.Equal(t, 1.25, daily)
	require.Equal(t, 2.5, weekly)
	require.Equal(t, 3.75, monthly)
}
