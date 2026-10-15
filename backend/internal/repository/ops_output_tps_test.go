package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsOutputTPSQueryUsesGenerationWindowAndOriginalFilters(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	at := time.Date(2026, 10, 15, 1, 0, 0, 0, time.UTC)
	groupID := int64(7)
	filter := &service.OpsDashboardFilter{StartTime: at, EndTime: at.Add(time.Hour), Platform: "OPENAI", GroupID: &groupID}
	mock.ExpectQuery(`(?s)percentile_cont\(0\.05\).*percentile_cont\(0\.10\).*percentile_cont\(0\.50\).*ul\.duration_ms::bigint - ul\.first_token_ms::bigint.*ul\.group_id = \$3.*COALESCE\(NULLIF\(g\.platform,''\), a\.platform\) = \$4.*ul\.output_tokens >= 20.*ul\.first_token_ms >= 0.*>= 1000.*NOT IN \('image', 'video'\)`).
		WithArgs(at, at.Add(time.Hour), groupID, "openai").
		WillReturnRows(sqlmock.NewRows([]string{"p5", "p10", "p50", "avg", "samples"}).AddRow(nil, nil, nil, nil, 0))
	stats, err := repo.queryOutputTPS(context.Background(), filter)
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.Zero(t, stats.SampleCount)
	require.Nil(t, stats.P50, "missing samples are not a zero measured speed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsOutputTPSPostgresDistributionAndEligibility(t *testing.T) {
	db, ctx := isolatedPostgres(t)
	_, err := db.ExecContext(ctx, `CREATE TABLE groups (id bigint PRIMARY KEY, platform text);
CREATE TABLE accounts (id bigint PRIMARY KEY, platform text);
CREATE TABLE usage_logs (created_at timestamptz, output_tokens bigint, duration_ms integer, first_token_ms integer,
 image_count integer DEFAULT 0, image_output_tokens integer DEFAULT 0, billing_mode text DEFAULT 'token',
 request_type integer, group_id bigint DEFAULT 7, account_id bigint DEFAULT 5);
INSERT INTO groups VALUES (7, 'openai'), (8, 'anthropic'); INSERT INTO accounts VALUES (5, 'openai');`)
	require.NoError(t, err)
	at := time.Date(2026, 10, 15, 1, 0, 0, 0, time.UTC)
	for _, row := range []struct{ tokens, total, first int }{{100, 21000, 1000}, {100, 12000, 2000}, {120, 4000, 1000}, {300, 4000, 1000}} {
		_, err = db.ExecContext(ctx, `INSERT INTO usage_logs (created_at, output_tokens, duration_ms, first_token_ms, request_type) VALUES ($1,$2,$3,$4,$5)`, at, row.tokens, row.total, row.first, service.RequestTypeStream)
		require.NoError(t, err)
	}
	// Deliberately invalid timing, tiny responses, media and failed request kinds
	// must not skew the distribution or its sample denominator.
	_, err = db.ExecContext(ctx, `INSERT INTO usage_logs (created_at, output_tokens, duration_ms, first_token_ms, image_count, image_output_tokens, billing_mode, request_type) VALUES
 ($1, 19, 10000, 1000, 0, 0, 'token', $2),
 ($1, 100, 1999, 1000, 0, 0, 'token', $2),
 ($1, 100, 10000, NULL, 0, 0, 'token', $2),
 ($1, 100, 10000, -1, 0, 0, 'token', $2),
 ($1, 100, 1000, 1000, 0, 0, 'token', $2),
 ($1, 100, 10000, 1000, 1, 0, 'image', $2),
 ($1, 100, 10000, 1000, 0, 1, 'token', $2),
 ($1, 100, 10000, 1000, 0, 0, 'video', $2),
 ($1, 100, 10000, 1000, 0, 0, 'token', $3)`, at, service.RequestTypeStream, service.RequestTypeCyberBlocked)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO usage_logs (created_at, output_tokens, duration_ms, first_token_ms, request_type, group_id) VALUES ($1, 50000, 3000, 1000, $2, 8), ($3, 50000, 3000, 1000, $2, 7)`, at, service.RequestTypeStream, at.Add(time.Hour))
	require.NoError(t, err)
	groupID := int64(7)
	repo := &opsRepository{db: db}
	filter := &service.OpsDashboardFilter{StartTime: at, EndTime: at.Add(time.Hour), GroupID: &groupID, Platform: "openai"}
	stats, err := repo.queryOutputTPS(ctx, filter)
	require.NoError(t, err)
	require.EqualValues(t, 4, stats.SampleCount)
	require.InDelta(t, 5.75, *stats.P5, 1e-10)
	require.InDelta(t, 6.5, *stats.P10, 1e-10)
	require.InDelta(t, 25, *stats.P50, 1e-10)
	require.InDelta(t, 38.75, *stats.Avg, 1e-10)
	filter.Platform = "anthropic"
	stats, err = repo.queryOutputTPS(ctx, filter)
	require.NoError(t, err)
	require.Zero(t, stats.SampleCount)
	require.Nil(t, stats.P50)
}
