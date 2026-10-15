package repository

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// Count every SQL match, including unexpected reads that the best-effort
// snapshot synchronizer would otherwise log and swallow.
type providerCooldownQueryMatcher struct{ count int }

func (m *providerCooldownQueryMatcher) Match(_, _ string) error {
	m.count++
	return nil
}

type providerCooldownSchedulerCache struct{ service.SchedulerCache }

func TestProviderCooldownRecoveryDoesNotPublishBeforeOuterCommit(t *testing.T) {
	for _, operation := range []string{"replace", "clear model", "clear account"} {
		t.Run(operation, func(t *testing.T) {
			matcher := &providerCooldownQueryMatcher{}
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			t.Cleanup(func() { _ = client.Close() })
			repo := newAccountRepositoryWithSQL(client, db, &providerCooldownSchedulerCache{})
			mock.ExpectBegin()
			tx, err := client.Tx(context.Background())
			require.NoError(t, err)
			ctx := dbent.NewTxContext(context.Background(), tx)
			mock.ExpectExec("atomic update with outbox").WillReturnResult(sqlmock.NewResult(0, 1))
			observed := []byte(`{"reason":"cline_credits_exhausted"}`)
			var changed bool
			switch operation {
			case "replace":
				changed, err = repo.SetObservedModelRateLimit(ctx, 5, "cline:credits", observed, []byte(`{"reason":"cline_credits_low"}`))
			case "clear model":
				changed, err = repo.ClearObservedModelRateLimit(ctx, 5, "cline:credits", observed)
			default:
				changed, err = repo.ClearObservedTempUnschedulable(ctx, 5, time.Now(), "cn_balance_low")
			}
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, 1, matcher.count, "no pre-commit account read/cache publication")
			mock.ExpectRollback()
			require.NoError(t, tx.Rollback())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestProviderCooldownRecoveryUsesAtomicObservedState(t *testing.T) {
	for _, changed := range []int64{0, 1} {
		t.Run(time.Duration(changed).String(), func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			repo := newAccountRepositoryWithSQL(nil, db, nil)
			observed := []byte(`{"reason":"cline_credits_exhausted","rate_limit_reset_at":"2026-10-15T15:00:00Z","rate_limited_at":"2026-10-15T14:00:00Z"}`)
			// Equality covers the whole observed JSON record, not just the
			// reason prefix. A concurrent write makes this update affect zero.
			mock.ExpectExec(`(?s)WITH changed AS.*extra = extra #- ARRAY.*extra->'model_rate_limits'->\$2 = \$3::jsonb.*INSERT INTO scheduler_outbox`).
				WithArgs(int64(5), "cline:credits", observed, service.SchedulerOutboxEventAccountChanged).
				WillReturnResult(sqlmock.NewResult(0, changed))
			ok, err := repo.ClearObservedModelRateLimit(context.Background(), 5, "cline:credits", observed)
			require.NoError(t, err)
			require.Equal(t, changed > 0, ok)
			replacement := []byte(`{"reason":"cline_credits_low","rate_limit_reset_at":"2026-10-15T16:00:00Z"}`)
			for _, expected := range [][]byte{observed, []byte(`null`)} {
				mock.ExpectExec(`(?s)WITH changed AS.*jsonb_set.*COALESCE\(extra->'model_rate_limits'->\$2, 'null'::jsonb\) = \$3::jsonb.*INSERT INTO scheduler_outbox`).
					WithArgs(int64(5), "cline:credits", expected, replacement, service.SchedulerOutboxEventAccountChanged).
					WillReturnResult(sqlmock.NewResult(0, changed))
				ok, err = repo.SetObservedModelRateLimit(context.Background(), 5, "cline:credits", expected, replacement)
				require.NoError(t, err)
				require.Equal(t, changed > 0, ok)
			}
			until := time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC)
			mock.ExpectExec(`(?s)WITH changed AS.*temp_unschedulable_until = \$2 AND temp_unschedulable_reason = \$3.*INSERT INTO scheduler_outbox`).
				WithArgs(int64(5), until, "command_code_window_limit", service.SchedulerOutboxEventAccountChanged).
				WillReturnResult(sqlmock.NewResult(0, changed))
			ok, err = repo.ClearObservedTempUnschedulable(context.Background(), 5, until, "command_code_window_limit")
			require.NoError(t, err)
			require.Equal(t, changed > 0, ok)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
