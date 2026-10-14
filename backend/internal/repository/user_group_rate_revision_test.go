package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserGroupRateRepositoryRevision_SuccessfulWrites(t *testing.T) {
	zero := 0.0
	for _, tc := range []struct {
		name        string
		write       func(service.UserGroupRateRepository) error
		queries     []string
		transaction bool
	}{
		{"user upsert zero", func(r service.UserGroupRateRepository) error {
			return r.SyncUserGroupRates(context.Background(), 42, map[int64]*float64{10: &zero})
		}, []string{"INSERT INTO user_group_rate_multipliers"}, true},
		{"user clear specific", func(r service.UserGroupRateRepository) error {
			return r.SyncUserGroupRates(context.Background(), 42, map[int64]*float64{10: nil})
		}, []string{"UPDATE user_group_rate_multipliers", "DELETE FROM user_group_rate_multipliers"}, true},
		{"user clear all", func(r service.UserGroupRateRepository) error {
			return r.SyncUserGroupRates(context.Background(), 42, nil)
		}, []string{"UPDATE user_group_rate_multipliers", "DELETE FROM user_group_rate_multipliers"}, true},
		{"group upsert zero", func(r service.UserGroupRateRepository) error {
			return r.SyncGroupRateMultipliers(context.Background(), 10, []service.GroupRateMultiplierInput{{UserID: 42, RateMultiplier: 0}})
		}, []string{"UPDATE user_group_rate_multipliers", "DELETE FROM user_group_rate_multipliers", "INSERT INTO user_group_rate_multipliers"}, true},
		{"group clear all", func(r service.UserGroupRateRepository) error {
			return r.SyncGroupRateMultipliers(context.Background(), 10, nil)
		}, []string{"UPDATE user_group_rate_multipliers", "DELETE FROM user_group_rate_multipliers"}, true},
		{"delete group", func(r service.UserGroupRateRepository) error { return r.DeleteByGroupID(context.Background(), 10) }, []string{"DELETE FROM user_group_rate_multipliers"}, false},
		{"delete user", func(r service.UserGroupRateRepository) error { return r.DeleteByUserID(context.Background(), 42) }, []string{"DELETE FROM user_group_rate_multipliers"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var revision service.UserGroupRateRevisionRepository
			matcher := sqlmock.QueryMatcherFunc(func(expected, actual string) error {
				require.Zero(t, revision.UserGroupRateRevision(), "revision must remain unchanged during durable writes")
				return sqlmock.QueryMatcherRegexp.Match(expected, actual)
			})
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			repo := NewUserGroupRateRepository(db)
			revision = repo.(service.UserGroupRateRevisionRepository)
			if tc.transaction {
				mock.ExpectBegin()
			}
			for _, query := range tc.queries {
				mock.ExpectExec(query).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			if tc.transaction {
				mock.ExpectCommit()
			}
			require.NoError(t, tc.write(repo))
			require.Equal(t, uint64(1), revision.UserGroupRateRevision())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestUserGroupRateRepositoryRevision_FailedWritesDoNotAdvance(t *testing.T) {
	writeErr := errors.New("write failed")
	for _, method := range []string{"user sync", "group sync"} {
		for _, stage := range []string{"begin", "first write", "later write", "commit"} {
			t.Run(method+"/"+stage, func(t *testing.T) {
				db, mock, err := sqlmock.New()
				require.NoError(t, err)
				t.Cleanup(func() { _ = db.Close() })
				repo := NewUserGroupRateRepository(db)
				begin := mock.ExpectBegin()
				if stage == "begin" {
					begin.WillReturnError(writeErr)
				} else {
					first := mock.ExpectExec("UPDATE user_group_rate_multipliers")
					if stage == "first write" {
						first.WillReturnError(writeErr)
						mock.ExpectRollback()
					} else {
						first.WillReturnResult(sqlmock.NewResult(0, 1))
						later := mock.ExpectExec("DELETE FROM user_group_rate_multipliers")
						if stage == "later write" {
							later.WillReturnError(writeErr)
							mock.ExpectRollback()
						} else {
							later.WillReturnResult(sqlmock.NewResult(0, 1))
							mock.ExpectCommit().WillReturnError(writeErr)
						}
					}
				}
				if method == "user sync" {
					err = repo.SyncUserGroupRates(context.Background(), 42, nil)
				} else {
					err = repo.SyncGroupRateMultipliers(context.Background(), 10, nil)
				}
				require.ErrorIs(t, err, writeErr)
				require.Zero(t, repo.(service.UserGroupRateRevisionRepository).UserGroupRateRevision())
				require.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
	for _, group := range []bool{true, false} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		repo := NewUserGroupRateRepository(db)
		mock.ExpectExec("DELETE FROM user_group_rate_multipliers").WillReturnError(writeErr)
		if group {
			err = repo.DeleteByGroupID(context.Background(), 10)
		} else {
			err = repo.DeleteByUserID(context.Background(), 42)
		}
		require.ErrorIs(t, err, writeErr)
		require.Zero(t, repo.(service.UserGroupRateRevisionRepository).UserGroupRateRevision())
		require.NoError(t, mock.ExpectationsWereMet())
		_ = db.Close()
	}
}
