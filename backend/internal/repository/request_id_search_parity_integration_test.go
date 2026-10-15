//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type requestIDParityInflightStore struct {
	rows []service.UsageInflightSnapshot
}

func (*requestIDParityInflightStore) Save(context.Context, service.UsageInflightSnapshot) error {
	return nil
}
func (*requestIDParityInflightStore) Delete(context.Context, int64, string) error { return nil }
func (s *requestIDParityInflightStore) ListUser(context.Context, int64) ([]service.UsageInflightSnapshot, error) {
	return s.rows, nil
}
func (s *requestIDParityInflightStore) ListAll(context.Context) ([]service.UsageInflightSnapshot, error) {
	return s.rows, nil
}

// Exercise the actual PostgreSQL predicates against the public in-flight path,
// including namespaced inputs, case sensitivity and literal LIKE metacharacters.
func TestRequestIDSearchPersistedInflightParity(t *testing.T) {
	tx := testEntTx(t)
	repo := newUsageLogRepositoryWithSQL(tx.Client(), tx)
	start := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	ids := []string{"abc", "client:abc", "local:abc", "client:ABC", "xabc", "client:abcdef", `client:abc%_\tail`, "client:abcZZZtail", "local:ab"}
	store := &requestIDParityInflightStore{}
	values := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for i, id := range ids {
		store.rows = append(store.rows, service.UsageInflightSnapshot{RequestID: id, UserID: 42, StartedAt: start})
		args = append(args, id)
		values = append(values, fmt.Sprintf("($%d::text, NULL::text)", i+1))
	}
	service.SetUsageInflightStore(store)
	t.Cleanup(func() { service.SetUsageInflightStore(nil) })
	for _, input := range []string{"abc", "client:abc", "=abc", "=local:abc", "=ABC", "^abc", "^client:abc", `abc%_\`, `^abc%_\`, "%_", "^ab"} {
		for _, columns := range [][]string{
			{"request_id", "upstream_request_id"}, {"upstream_request_id", "request_id"},
			{"request_id", "client_request_id"}, {"client_request_id", "request_id"},
		} {
			t.Run(input+"/"+columns[0]+"/"+columns[1], func(t *testing.T) {
				conditions, queryArgs := appendRequestIDSearchCondition(nil, append([]any{}, args...), input, columns...)
				query := "SELECT " + columns[0] + " FROM (VALUES " + strings.Join(values, ", ") + ") AS logs(" + strings.Join(columns, ", ") + ") WHERE " + strings.Join(conditions, " AND ")
				rows, err := repo.sql.QueryContext(context.Background(), query, queryArgs...)
				require.NoError(t, err)
				persisted := []string{}
				for rows.Next() {
					var id string
					require.NoError(t, rows.Scan(&id))
					persisted = append(persisted, id)
				}
				require.NoError(t, rows.Err())
				require.NoError(t, rows.Close())
				live := service.VisibleUsageInflight(context.Background(), 42, usagestats.UsageLogFilters{RequestID: input, StartTime: &start, EndTime: &end}, nil)
				liveIDs := []string{}
				for _, row := range live {
					liveIDs = append(liveIDs, row.RequestID)
				}
				require.ElementsMatch(t, persisted, liveIDs)
			})
		}
	}
}
