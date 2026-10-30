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

func TestListErrorLogsUpstreamRequestIDQueryAndScan(t *testing.T) {
	for _, upstreamID := range []string{"", "upstream-xai-id"} {
		t.Run(upstreamID, func(t *testing.T) {
			db, mock := newSQLMock(t)
			mock.MatchExpectationsInOrder(false)
			repo := &opsRepository{db: db}
			mock.ExpectQuery(`SELECT COUNT\(\*\) FROM ops_error_logs e`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			// Exact ordering also guards response-before-request and all three header keys.
			priority := `(?s)NULLIF\(e.upstream_response_headers->>'x-request-id', ''\),\s+NULLIF\(e.upstream_response_headers->>'xai-request-id', ''\),\s+NULLIF\(e.upstream_response_headers->>'x-oneapi-request-id', ''\),\s+NULLIF\(e.upstream_request_headers->>'x-request-id', ''\),\s+NULLIF\(e.upstream_request_headers->>'xai-request-id', ''\),\s+NULLIF\(e.upstream_request_headers->>'x-oneapi-request-id', ''\),\s+''`
			columns := strings.Fields("id created_at phase type owner source severity status platform model resolved resolved_at resolved_by resolved_name client_request_id request_id upstream_request_id message user_id email api_key_id account_id account_name group_id group_name client_ip path stream inbound upstream requested_model upstream_model user_agent request_type api_key_name api_key_deleted")
			rows := sqlmock.NewRows(columns).AddRow(1, time.Now(), "upstream", "upstream_error", "provider", "upstream_http", "error", 502, "openai", "gpt-5", false, nil, nil, "", "client", "local", upstreamID, "failed", nil, "", nil, nil, "", nil, "", nil, "/v1/responses", false, "", "", "", "", "ua", nil, "", nil)
			mock.ExpectQuery(priority).WithArgs(20, 0).WillReturnRows(rows)
			result, err := repo.ListErrorLogs(context.Background(), &service.OpsErrorLogFilter{})
			require.NoError(t, err)
			require.Len(t, result.Errors, 1)
			require.Equal(t, "local", result.Errors[0].RequestID)
			require.Equal(t, "client", result.Errors[0].ClientRequestID)
			require.Equal(t, upstreamID, result.Errors[0].UpstreamRequestID)
			require.Equal(t, "failed", result.Errors[0].Message)
			require.Equal(t, "ua", result.Errors[0].UserAgent)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
