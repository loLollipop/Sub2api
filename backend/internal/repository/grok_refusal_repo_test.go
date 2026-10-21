//go:build unit

package repository

import (
	"context"
	"database/sql"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func refusalTestJob(preauth bool) *service.MediaBillingJob {
	intent := testMediaIntent(0.1)
	intent.Command.APIKeyQuotaCost = 0
	intent.Command.BalancePreauthorized = preauth
	intent.Command.Normalize()
	return &service.MediaBillingJob{ID: "refusal:req", Kind: service.MediaBillingKindGrokRefusal, UserID: 42, APIKeyID: 7, AccountID: 9, GroupID: 11, Status: service.MediaBillingReady, Intent: intent}
}

func TestGrokRefusalCommitAtomicEffectsBlockAndJob(t *testing.T) {
	for _, mode := range []string{"balance", "preauth", "job_failure", "block_failure", "commit_failure", "quota", "quota_failure"} {
		t.Run(mode, func(t *testing.T) {
			repo, mock := newMediaBillingSQLMock(t)
			job := refusalTestJob(mode == "preauth")
			key := strings.Repeat("a", 64)
			if strings.HasPrefix(mode, "quota") {
				job.Intent.QuotaPlatform = service.PlatformGrok
			}
			mock.ExpectBegin()
			mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WithArgs(int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery(`SELECT job_id FROM grok_refusal_blocks`).WithArgs(int64(42), key).WillReturnError(sql.ErrNoRows)
			mock.ExpectQuery(`(?s)SELECT.*FROM media_billing_jobs WHERE id`).WithArgs(job.ID).WillReturnError(sql.ErrNoRows)
			mock.ExpectQuery(`(?s)INSERT INTO usage_billing_dedup`).WithArgs(job.Intent.Command.RequestID, int64(7), job.Intent.Command.RequestFingerprint).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
			mock.ExpectQuery(`(?s)SELECT request_fingerprint.*usage_billing_dedup_archive`).WillReturnError(sql.ErrNoRows)
			if mode == "preauth" {
				mock.ExpectExec(`(?s)UPDATE billing_balance_settlements.*amount_usd.*request_fingerprint`).WillReturnResult(sqlmock.NewResult(0, 1))
			} else {
				mock.ExpectQuery(conditionalBalanceDeductSQL).WithArgs(0.1, int64(42)).WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(4.9))
			}

			if strings.HasPrefix(mode, "quota") {
				mock.ExpectQuery(`(?s)SELECT grok_refusal_usage.*FOR UPDATE`).WithArgs(int64(42), service.PlatformGrok).WillReturnRows(sqlmock.NewRows([]string{"grok_refusal_usage", "monthly_window_start"}).AddRow([]byte(`{}`), nil))
				update := mock.ExpectExec(`UPDATE user_platform_quotas SET grok_refusal_usage`).WithArgs(int64(42), service.PlatformGrok, sqlmock.AnyArg())
				if mode == "quota_failure" {
					update.WillReturnError(errors.New("quota write failed"))
					mock.ExpectRollback()
					_, created, err := repo.CommitGrokRefusal(context.Background(), job, []string{key}, []string{key})
					require.Error(t, err)
					require.False(t, created)
					require.NoError(t, mock.ExpectationsWereMet())
					return
				}
				update.WillReturnResult(sqlmock.NewResult(0, 1))
			}
			insert := mock.ExpectExec(`(?s)INSERT INTO media_billing_jobs`)
			if mode == "job_failure" {
				insert.WillReturnError(errors.New("job insert failed"))
				mock.ExpectRollback()
			} else {
				insert.WillReturnResult(sqlmock.NewResult(0, 1))
				block := mock.ExpectExec(`INSERT INTO grok_refusal_blocks`).WithArgs(int64(42), key, job.ID)
				if mode == "block_failure" {
					block.WillReturnError(errors.New("block insert failed"))
					mock.ExpectRollback()
				} else {
					block.WillReturnResult(sqlmock.NewResult(0, 1))
					commit := mock.ExpectCommit()
					if mode == "commit_failure" {
						commit.WillReturnError(errors.New("commit failed"))
					}
				}
			}
			persisted, created, err := repo.CommitGrokRefusal(context.Background(), job, []string{key}, []string{key})
			if strings.HasSuffix(mode, "failure") {
				require.Error(t, err)
				require.False(t, created)
				require.Nil(t, persisted)
			} else {
				require.NoError(t, err)
				require.True(t, created)
				require.Equal(t, service.MediaBillingBilled, persisted.Status)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestGrokRefusalConversationDuplicateDoesNotClaimRequestOrGuard(t *testing.T) {
	repo, mock := newMediaBillingSQLMock(t)
	job := refusalTestJob(true)
	existing := *job
	existing.ID = "first"
	existing.APIKeyID = 99
	existing.Intent = testMediaIntent(.1)
	existing.Intent.Command.APIKeyID = 99
	existing.Status = service.MediaBillingBilled
	key := strings.Repeat("b", 64)
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT job_id FROM grok_refusal_blocks`).WillReturnRows(sqlmock.NewRows([]string{"job_id"}).AddRow(existing.ID))
	mock.ExpectQuery(`(?s)SELECT.*FROM media_billing_jobs WHERE id`).WithArgs(existing.ID).WillReturnRows(mediaBillingTestRow(existing))
	mock.ExpectExec(`INSERT INTO grok_refusal_blocks`).WithArgs(int64(42), key, existing.ID).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	actual, created, err := repo.CommitGrokRefusal(context.Background(), job, []string{key}, []string{key})
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, int64(99), actual.APIKeyID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrokRefusalLookupSearchesEveryBoundedBatch(t *testing.T) {
	repo, mock := newMediaBillingSQLMock(t)
	keys := make([]string, 601)
	for i := range keys {
		keys[i] = strings.Repeat("c", 64)
	}
	for i := 0; i < 4; i++ {
		mock.ExpectQuery(`SELECT job_id FROM grok_refusal_blocks`).WillReturnError(sql.ErrNoRows)
	}
	mock.ExpectQuery(`SELECT job_id FROM grok_refusal_blocks`).WillReturnRows(sqlmock.NewRows([]string{"job_id"}).AddRow("persisted"))
	found, err := repo.FindGrokRefusal(context.Background(), 42, keys)
	require.NoError(t, err)
	require.True(t, found)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrokRefusalSimpleClaimUsesSQLKindAndZeroPriceFilter(t *testing.T) {
	repo, mock := newMediaBillingSQLMock(t)
	mock.ExpectQuery(`(?s)WHERE status IN.*kind = 'grok_refusal'.*status = 'billed'.*BalanceCost.*FOR UPDATE SKIP LOCKED`).WithArgs(1, int64(time.Minute.Milliseconds())).WillReturnRows(sqlmock.NewRows(mediaBillingTestColumns))
	jobs, err := repo.ListGrokRefusalBillingJobs(context.Background(), 1)
	require.NoError(t, err)
	require.Empty(t, jobs)
	require.NoError(t, mock.ExpectationsWereMet())
}
