package repository

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPruneContentModerationGroupIDs(t *testing.T) {
	for _, value := range []string{
		``, `broken`, `null`, `[]`, `7`, `"config"`, `{}`, `{"group_ids":null}`,
		`{"group_ids":{}}`, `{"group_ids":"7"}`, ` { "group_ids": [8, "7", 7.0, 7e0] } `,
	} {
		t.Run(value, func(t *testing.T) {
			updated, changed := pruneContentModerationGroupIDs(value, 7)
			require.False(t, changed)
			require.Equal(t, value, updated, "no-op must preserve the original bytes")
		})
	}
	value := `{"group_ids":[7,9007199254740993,null,"7",7.0,7e0,{},[7],7,9223372036854775807],"unknown":{"large":18446744073709551616,"nul":"\u0000","surrogate":"\ud800","null":null},"all_groups":true}`
	updated, changed := pruneContentModerationGroupIDs(value, 7)
	require.True(t, changed)
	var before, after map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(value), &before))
	require.NoError(t, json.Unmarshal([]byte(updated), &after))
	require.Equal(t, string(before["unknown"]), string(after["unknown"]))
	require.Equal(t, string(before["all_groups"]), string(after["all_groups"]))
	require.Equal(t, `[9007199254740993,null,"7",7.0,7e0,{},[7],9223372036854775807]`, string(after["group_ids"]))
	updated, changed = pruneContentModerationGroupIDs(`{"group_ids":[9223372036854775807,7,9223372036854775807]}`, 9223372036854775807)
	require.True(t, changed)
	require.Equal(t, `{"group_ids":[7]}`, updated)
	updated, changed = pruneContentModerationGroupIDs(`{"group_ids":[7,7]}`, 7)
	require.True(t, changed)
	require.Equal(t, `{"group_ids":[]}`, updated)
}

func TestContentModerationDispatchSettingsBypassWarmCache(t *testing.T) {
	repo, mock := newSettingRepositorySQLMock(t)
	repo.keyCache = map[string]settingCacheEntry{
		service.SettingKeyRiskControlEnabled: {setting: &service.Setting{Key: service.SettingKeyRiskControlEnabled, Value: "true"}, found: true, expiresAt: time.Now().Add(time.Hour).UnixNano()},
	}
	for i := 0; i < 2; i++ {
		mock.ExpectQuery(`SELECT .* FROM "settings" WHERE .* IN`).
			WithArgs(service.SettingKeyRiskControlEnabled, service.SettingKeyContentModerationConfig).
			WillReturnRows(sqlmock.NewRows([]string{"id", "key", "value", "updated_at"}).AddRow(int64(1), service.SettingKeyRiskControlEnabled, "false", time.Now()))
		values, err := repo.GetContentModerationDispatchSettings(context.Background())
		require.NoError(t, err)
		require.Equal(t, "false", values[service.SettingKeyRiskControlEnabled])
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func expectModerationDeleteGroup(mock sqlmock.Sqlmock, id int64) {
	mock.ExpectQuery(`SELECT id, subscription_type FROM groups .* FOR UPDATE`).WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id", "subscription_type"}).AddRow(id, service.SubscriptionTypeStandard))
	for _, query := range []string{
		`DELETE FROM api_key_group_routes`, `DELETE FROM user_allowed_groups`,
		`DELETE FROM account_groups`, `UPDATE composite_model_routes`, `UPDATE "groups"`,
	} {
		mock.ExpectExec(query).WillReturnResult(sqlmock.NewResult(0, 1))
	}
}

func expectModerationSettingLock(mock sqlmock.Sqlmock, value string) {
	mock.ExpectQuery(`SELECT value FROM settings WHERE key = \$1 FOR UPDATE`).
		WithArgs(service.SettingKeyContentModerationConfig).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(value))
}

func TestModerationDeleteCascadePrunesAndInvalidatesAfterCommit(t *testing.T) {
	settings, mock := newSettingRepositorySQLMock(t)
	groupRepo := newGroupRepositoryWithSQL(settings.client, nil)
	before := settings.ContentModerationConfigVersion()
	mock.ExpectBegin()
	expectModerationDeleteGroup(mock, 7)
	expectModerationSettingLock(mock, `{"group_ids":[7,8,7],"unknown":null}`)
	mock.ExpectExec(`UPDATE settings SET value = \$2`).
		WithArgs(service.SettingKeyContentModerationConfig, `{"group_ids":[8],"unknown":null}`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	_, err := groupRepo.DeleteCascade(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, before+1, settings.ContentModerationConfigVersion())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModerationDeleteCascadeNoOpDoesNotBroadcast(t *testing.T) {
	for _, value := range []string{`invalid`, `{"group_ids":null}`, ` {"group_ids":[8]} `} {
		t.Run(value, func(t *testing.T) {
			settings, mock := newSettingRepositorySQLMock(t)
			before := settings.ContentModerationConfigVersion()
			mock.ExpectBegin()
			expectModerationDeleteGroup(mock, 7)
			expectModerationSettingLock(mock, value)
			mock.ExpectCommit()
			_, err := newGroupRepositoryWithSQL(settings.client, nil).DeleteCascade(context.Background(), 7)
			require.NoError(t, err)
			require.Equal(t, before, settings.ContentModerationConfigVersion())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestModerationDeleteCascadeMissingSettingIsNoOp(t *testing.T) {
	settings, mock := newSettingRepositorySQLMock(t)
	before := settings.ContentModerationConfigVersion()
	mock.ExpectBegin()
	expectModerationDeleteGroup(mock, 7)
	mock.ExpectQuery(`SELECT value FROM settings`).WillReturnRows(sqlmock.NewRows([]string{"value"}))
	mock.ExpectCommit()
	_, err := newGroupRepositoryWithSQL(settings.client, nil).DeleteCascade(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, before, settings.ContentModerationConfigVersion())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModerationDeleteCascadeDatabaseFailuresRollback(t *testing.T) {
	for _, failure := range []string{"lock", "update", "commit"} {
		t.Run(failure, func(t *testing.T) {
			settings, mock := newSettingRepositorySQLMock(t)
			before := settings.ContentModerationConfigVersion()
			failureErr := errors.New(failure + " failed")
			mock.ExpectBegin()
			expectModerationDeleteGroup(mock, 7)
			if failure == "lock" {
				mock.ExpectQuery(`SELECT value FROM settings`).WillReturnError(failureErr)
			} else {
				expectModerationSettingLock(mock, `{"group_ids":[7]}`)
				update := mock.ExpectExec(`UPDATE settings SET value`)
				if failure == "update" {
					update.WillReturnError(failureErr)
				} else {
					update.WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectCommit().WillReturnError(failureErr)
				}
			}
			if failure != "commit" {
				mock.ExpectRollback()
			}
			_, err := newGroupRepositoryWithSQL(settings.client, nil).DeleteCascade(context.Background(), 7)
			require.ErrorIs(t, err, failureErr)
			require.Equal(t, before, settings.ContentModerationConfigVersion())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestModerationBorrowedTransactionDefersInvalidation(t *testing.T) {
	for _, injected := range []bool{false, true} {
		for _, commit := range []bool{false, true} {
			t.Run(map[bool]string{false: "context", true: "injected"}[injected]+map[bool]string{false: "Rollback", true: "Commit"}[commit], func(t *testing.T) {
				settings, mock := newSettingRepositorySQLMock(t)
				mock.ExpectBegin()
				tx, err := settings.client.Tx(context.Background())
				require.NoError(t, err)
				ctx := dbent.NewTxContext(context.Background(), tx)
				groups := newGroupRepositoryWithSQL(settings.client, nil)
				if injected {
					ctx = context.Background()
					groups = newGroupRepositoryWithSQL(tx.Client(), tx)
				}
				expectModerationDeleteGroup(mock, 7)
				expectModerationSettingLock(mock, `{"group_ids":[7]}`)
				mock.ExpectExec(`UPDATE settings SET value`).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`INSERT INTO scheduler_outbox`).WillReturnResult(sqlmock.NewResult(0, 1))
				before := settings.ContentModerationConfigVersion()
				_, err = groups.DeleteCascade(ctx, 7)
				require.NoError(t, err)
				require.Equal(t, before, settings.ContentModerationConfigVersion())
				if commit {
					mock.ExpectCommit()
					require.NoError(t, tx.Commit())
					require.Equal(t, before+1, settings.ContentModerationConfigVersion())
				} else {
					mock.ExpectRollback()
					require.NoError(t, tx.Rollback())
					require.Equal(t, before, settings.ContentModerationConfigVersion())
				}
				require.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
}

func TestModerationRequiresTransactionOwnerBeforeWrites(t *testing.T) {
	settings, mock := newSettingRepositorySQLMock(t)
	mock.ExpectBegin()
	tx, err := settings.client.Tx(context.Background())
	require.NoError(t, err)
	bareSettings, ok := NewSettingRepository(tx.Client()).(*settingRepository)
	require.True(t, ok)
	require.ErrorIs(t, bareSettings.SetContentModerationConfig(context.Background(), `{}`, nil), dbent.ErrTxStarted)
	_, err = newGroupRepositoryWithSQL(tx.Client(), nil).DeleteCascade(context.Background(), 7)
	require.ErrorIs(t, err, dbent.ErrTxStarted)
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSetContentModerationConfigLocksEveryRetainedGroup(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "missing"}[missing], func(t *testing.T) {
			settings, mock := newSettingRepositorySQLMock(t)
			before := settings.ContentModerationConfigVersion()
			mock.ExpectBegin()
			rows := sqlmock.NewRows([]string{"id"}).AddRow(7)
			if !missing {
				rows.AddRow(8)
			}
			mock.ExpectQuery(`SELECT id FROM groups .* ORDER BY id FOR SHARE`).WithArgs("{7,8}").WillReturnRows(rows)
			if missing {
				mock.ExpectRollback()
			} else {
				mock.ExpectExec(`INSERT INTO settings .* ON CONFLICT`).
					WithArgs(service.SettingKeyContentModerationConfig, `{"all_groups":true,"group_ids":[7,8]}`).
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit()
			}
			err := settings.SetContentModerationConfig(context.Background(), `{"all_groups":true,"group_ids":[7,8]}`, []int64{7, 8})
			if missing {
				require.ErrorIs(t, err, service.ErrGroupNotFound)
				require.Equal(t, before, settings.ContentModerationConfigVersion())
			} else {
				require.NoError(t, err)
				require.Equal(t, before+1, settings.ContentModerationConfigVersion())
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestSetContentModerationConfigDatabaseFailures(t *testing.T) {
	for _, failure := range []string{"begin", "group-lock", "upsert", "commit"} {
		t.Run(failure, func(t *testing.T) {
			repo, mock := newSettingRepositorySQLMock(t)
			before := repo.ContentModerationConfigVersion()
			failureErr := errors.New(failure + " failed")
			begin := mock.ExpectBegin()
			if failure == "begin" {
				begin.WillReturnError(failureErr)
			} else {
				lock := mock.ExpectQuery(`SELECT id FROM groups .* FOR SHARE`)
				if failure == "group-lock" {
					lock.WillReturnError(failureErr)
				} else {
					lock.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
					upsert := mock.ExpectExec(`INSERT INTO settings`)
					if failure == "upsert" {
						upsert.WillReturnError(failureErr)
					} else {
						upsert.WillReturnResult(sqlmock.NewResult(1, 1))
						mock.ExpectCommit().WillReturnError(failureErr)
					}
				}
				if failure != "commit" {
					mock.ExpectRollback()
				}
			}
			err := repo.SetContentModerationConfig(context.Background(), `{"group_ids":[7]}`, []int64{7})
			require.ErrorIs(t, err, failureErr)
			require.Equal(t, before, repo.ContentModerationConfigVersion())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestSetContentModerationConfigBorrowedTransactionInvalidatesOnlyOnCommit(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "commit"}[commit], func(t *testing.T) {
			repo, mock := newSettingRepositorySQLMock(t)
			before := repo.ContentModerationConfigVersion()
			mock.ExpectBegin()
			tx, err := repo.client.Tx(context.Background())
			require.NoError(t, err)
			mock.ExpectExec(`INSERT INTO settings`).WillReturnResult(sqlmock.NewResult(1, 1))
			require.NoError(t, repo.SetContentModerationConfig(dbent.NewTxContext(context.Background(), tx), `{"group_ids":[]}`, nil))
			require.Equal(t, before, repo.ContentModerationConfigVersion())
			if commit {
				mock.ExpectCommit()
				require.NoError(t, tx.Commit())
				require.Equal(t, before+1, repo.ContentModerationConfigVersion())
			} else {
				mock.ExpectRollback()
				require.NoError(t, tx.Rollback())
				require.Equal(t, before, repo.ContentModerationConfigVersion())
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestModerationGenerationEvictsAllCacheAndRejectsOldStores(t *testing.T) {
	for _, access := range []string{"peek", "version", "key-store", "all-store", "get-all"} {
		t.Run(access, func(t *testing.T) {
			repo, mock := newSettingRepositorySQLMock(t)
			version := repo.cacheVersion()
			entry := settingCacheEntry{found: true, setting: &service.Setting{Value: "old"}, expiresAt: time.Now().Add(time.Hour).UnixNano()}
			repo.storeKeyCacheIfVersion(service.SettingKeyContentModerationConfig, entry, version)
			repo.storeKeyCacheIfVersion("unrelated", entry, version)
			repo.storeAllCacheIfVersion(&settingAllCacheEntry{values: map[string]string{"old": "value"}, expiresAt: entry.expiresAt}, version)
			contentModerationConfigGeneration.Add(1)
			switch access {
			case "peek":
				_, hit := repo.peekKeyCache(service.SettingKeyContentModerationConfig)
				require.False(t, hit)
			case "version":
				require.Greater(t, repo.cacheVersion(), version)
			case "key-store":
				repo.storeKeyCacheIfVersion(service.SettingKeyContentModerationConfig, entry, version)
			case "all-store":
				repo.storeAllCacheIfVersion(&settingAllCacheEntry{values: map[string]string{"old": "value"}, expiresAt: entry.expiresAt}, version)
			case "get-all":
				mock.ExpectQuery(`SELECT .* FROM "settings"`).WillReturnRows(settingRows(1, service.SettingKeyContentModerationConfig, "new"))
				values, err := repo.GetAll(context.Background())
				require.NoError(t, err)
				require.Equal(t, "new", values[service.SettingKeyContentModerationConfig])
			}
			_, hit := repo.peekKeyCache(service.SettingKeyContentModerationConfig)
			require.False(t, hit)
			_, hit = repo.peekKeyCache("unrelated")
			require.True(t, hit)
			if access != "get-all" {
				require.Nil(t, repo.allCache)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestModerationGenerationStartsNewFlightAndOldFlightCannotRefill(t *testing.T) {
	repo, mock := newSettingRepositorySQLMock(t)
	mock.ExpectQuery(`SELECT .* FROM "settings"`).WithArgs(service.SettingKeyContentModerationConfig).
		WillDelayFor(100 * time.Millisecond).WillReturnRows(settingRows(1, service.SettingKeyContentModerationConfig, "old"))
	oldDone := make(chan error, 1)
	go func() {
		_, err := repo.GetValue(context.Background(), service.SettingKeyContentModerationConfig)
		oldDone <- err
	}()
	// sqlmock marks an expectation as fulfilled on entry, before its delay.
	require.Eventually(t, func() bool { return mock.ExpectationsWereMet() == nil }, time.Second, time.Millisecond)
	contentModerationConfigGeneration.Add(1)
	mock.ExpectQuery(`SELECT .* FROM "settings"`).WithArgs(service.SettingKeyContentModerationConfig).
		WillReturnRows(settingRows(1, service.SettingKeyContentModerationConfig, "new"))
	value, err := repo.GetValue(context.Background(), service.SettingKeyContentModerationConfig)
	require.NoError(t, err)
	require.Equal(t, "new", value)
	require.NoError(t, <-oldDone)
	value, err = repo.GetValue(context.Background(), service.SettingKeyContentModerationConfig)
	require.NoError(t, err)
	require.Equal(t, "new", value)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUnrelatedSettingWritesDoNotBroadcastModerationGeneration(t *testing.T) {
	repo, mock := newSettingRepositorySQLMock(t)
	before := repo.ContentModerationConfigVersion()
	mock.ExpectQuery(`INSERT INTO "settings"`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	require.NoError(t, repo.Set(context.Background(), "notification_email_record_delivery", "sent"))
	require.Equal(t, before, repo.ContentModerationConfigVersion())
	require.NoError(t, mock.ExpectationsWereMet())
}
