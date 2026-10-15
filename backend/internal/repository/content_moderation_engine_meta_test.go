package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestContentModerationRepositoryNullableEngineMeta(t *testing.T) {
	for _, meta := range []*service.ContentModerationEngineMeta{nil, {Engine: "typesafe", Model: "jev-actual", RulesVersion: service.TypeSafeModerationRulesVersion, SkippedImages: 2}} {
		t.Run(map[bool]string{true: "historical_record", false: "typesafe_record"}[meta == nil], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			repo := NewContentModerationRepository(db)
			var expectedMeta driver.Value
			if meta != nil {
				raw, err := json.Marshal(meta)
				require.NoError(t, err)
				expectedMeta = string(raw)
			}
			args := make([]driver.Value, 26)
			for i := range args {
				args[i] = sqlmock.AnyArg()
			}
			args[25] = expectedMeta
			now := time.Now()
			mock.ExpectQuery(`INSERT INTO content_moderation_logs`).WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(1, now))
			log := &service.ContentModerationLog{EngineMeta: meta, Action: "allow"}
			require.NoError(t, repo.CreateLog(context.Background(), log))
			require.Equal(t, int64(1), log.ID)
			mock.ExpectQuery(`SELECT COUNT`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			columns := []string{"id", "request_id", "user_id", "user_email", "api_key_id", "api_key_name", "group_id", "group_name", "endpoint", "provider", "model", "mode", "action", "flagged", "highest_category", "highest_score", "category_scores", "threshold_snapshot", "input_excerpt", "upstream_latency_ms", "error", "violation_count", "auto_banned", "email_sent", "status", "queue_delay_ms", "matched_keyword", "created_at", "engine_meta"}
			row := []driver.Value{int64(1), "", nil, "", nil, "", nil, "", "", "", "", "observe", "allow", false, "", 0.0, []byte(`{}`), []byte(`{}`), "", nil, "", 0, false, false, "", nil, "", now, expectedMeta}
			mock.ExpectQuery(`SELECT[\s\S]*l.engine_meta`).WillReturnRows(sqlmock.NewRows(columns).AddRow(row...))
			items, _, err := repo.ListLogs(context.Background(), service.ContentModerationLogFilter{})
			require.NoError(t, err)
			require.Len(t, items, 1)
			require.Equal(t, meta, items[0].EngineMeta)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
