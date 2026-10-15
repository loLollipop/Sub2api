package repository

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestRequestIDSearchSQLModesAcrossUsageAndOps(t *testing.T) {
	for _, tc := range []struct {
		input string
		args  []any
		exact bool
	}{
		{"abc", []any{"%abc%"}, false},
		{"client:abc", []any{"%abc%"}, false},
		{"=local:abc", []any{"abc", "client:abc", "local:abc"}, true},
		{`abc%_\`, []any{`%abc\%\_\\%`}, false},
		{`^client:abc%_\`, []any{`abc\%\_\\%`, `client:abc\%\_\\%`, `local:abc\%\_\\%`}, false},
	} {
		t.Run(tc.input, func(t *testing.T) {
			for _, columns := range [][]string{{"request_id", "upstream_request_id"}, {"e.request_id", "e.client_request_id"}} {
				conditions, args := appendRequestIDSearchCondition(nil, []any{int64(42)}, tc.input, columns...)
				want := append([]any{int64(42)}, tc.args...)
				require.Equal(t, want, args)
				require.Len(t, conditions, 1)
				where := conditions[0]
				for _, column := range columns {
					if tc.exact {
						require.Contains(t, where, column+" IN ($2, $3, $4)")
						require.NotContains(t, where, "ILIKE")
					} else {
						count := strings.Count(where, "("+column+" ILIKE $") + strings.Count(where, " OR "+column+" ILIKE $")
						require.Equal(t, len(tc.args), count)
					}
				}
				require.NotContains(t, where, "COALESCE")
			}
			opsWhere, opsArgs := buildOpsErrorLogsWhere(&service.OpsErrorLogFilter{RequestID: tc.input})
			require.True(t, reflect.DeepEqual(tc.args, opsArgs), "Ops arguments: %s %v", opsWhere, opsArgs)
			require.Contains(t, opsWhere, "e.request_id")
			require.Contains(t, opsWhere, "e.client_request_id")
		})
	}
}

func TestUsageRequestIDContainsUsesEscapedBoundParameters(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}
	mock.ExpectQuery(`SELECT .* FROM usage_logs WHERE \(request_id ILIKE \$1 OR upstream_request_id ILIKE \$1\) ORDER BY id DESC LIMIT \$2 OFFSET \$3`).
		WithArgs(`%abc\%\_\\%`, 21, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	_, _, err := repo.ListWithFilters(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20}, UsageLogFilters{RequestID: `abc%_\`})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRequestIDListRepositoriesRejectUnsafePatternsBeforeSQL(t *testing.T) {
	start := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	day := start.Add(24 * time.Hour)
	long := day.Add(2 * time.Second)
	for _, tc := range []struct {
		input      string
		start, end *time.Time
	}{
		{"ab", nil, nil}, {"^ab", nil, nil}, {"%_", nil, nil},
		{"client:a", nil, nil}, {"=", &start, &day}, {"^", &start, &day},
		{"ab", &start, &long}, {"ab", &day, &start}, {"ab", &start, &start},
	} {
		t.Run(tc.input, func(t *testing.T) {
			db, mock := newSQLMock(t)
			_, _, err := (&usageLogRepository{sql: db}).ListWithFilters(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20}, usagestats.UsageLogFilters{RequestID: tc.input, StartTime: tc.start, EndTime: tc.end})
			require.Error(t, err)
			_, err = (&opsRepository{db: db}).ListErrorLogs(context.Background(), &service.OpsErrorLogFilter{RequestID: tc.input, StartTime: tc.start, EndTime: tc.end})
			require.Error(t, err)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
