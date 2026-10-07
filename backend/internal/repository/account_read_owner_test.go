package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

const accountOwnerReadSQL = "SELECT id, created_by FROM accounts WHERE id = ANY($1)"

func newAccountOwnerReadRepo(t *testing.T) (*accountRepository, *sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	return newAccountRepositoryWithSQL(client, db, nil), db, mock
}

func expectAccountOwnerRead(mock sqlmock.Sqlmock, id int64, owner any) {
	mock.ExpectQuery(`SELECT .* FROM "accounts"`).WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "platform", "type", "status", "concurrency", "priority"}).
			AddRow(id, "parent", service.PlatformOpenAI, service.AccountTypeOAuth, service.StatusActive, 2, 50))
	mock.ExpectQuery(`FROM "account_groups"`).WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "group_id", "priority", "created_at"}))
	mock.ExpectQuery(regexp.QuoteMeta(accountOwnerReadSQL)).WithArgs(pq.Array([]int64{id})).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_by"}).AddRow(id, owner))
}

func TestAccountReadReturnsPersistedOwner(t *testing.T) {
	repo, _, mock := newAccountOwnerReadRepo(t)
	// Permit either enrichment order while retaining an exact one-query budget
	// for each association and for all account owners.
	mock.MatchExpectationsInOrder(false)
	expectAccountOwnerRead(mock, 11, int64(8))
	ctx := service.WithAccountOwnerScope(context.Background(), 7, 7)
	got, err := repo.GetByID(ctx, 11)
	require.NoError(t, err)
	require.NotNil(t, got.CreatedBy, "the real repository must retain the persisted uploader")
	require.Equal(t, int64(8), *got.CreatedBy)
	require.NoError(t, mock.ExpectationsWereMet())
}

func newOwnerShadowAdminService(repo service.AdminAccountRepository) service.AdminService {
	return service.NewAdminService(
		nil, nil, nil, repo, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
}

func TestCreateShadowFromRepositoryInheritsPersistedOwner(t *testing.T) {
	for _, tc := range []struct {
		name    string
		owner   any
		actor   int64
		allowed bool
	}{
		{"pool owner creates for another uploader", int64(8), 7, true},
		{"uploader creates for self", int64(8), 8, true},
		{"pool owner creates for legacy NULL", nil, 7, true},
		{"another administrator cannot create", int64(8), 9, false},
		{"ordinary administrator cannot use legacy NULL", nil, 8, false},
		{"ordinary administrator cannot use legacy zero", int64(0), 8, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, _, mock := newAccountOwnerReadRepo(t)
			mock.MatchExpectationsInOrder(false)
			expectAccountOwnerRead(mock, 11, tc.owner)
			if tc.allowed {
				mock.ExpectQuery(`SELECT .* FROM "accounts"`).WithArgs(int64(11), "spark").
					WillReturnRows(sqlmock.NewRows([]string{"id"}))
				mock.ExpectQuery(`INSERT INTO "accounts"`).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(22)))
				if tc.owner != nil {
					mock.ExpectExec(regexp.QuoteMeta("UPDATE accounts SET created_by = $1 WHERE id = $2 AND created_by IS NULL")).
						WithArgs(tc.owner, int64(22)).WillReturnResult(sqlmock.NewResult(0, 1))
				}
				mock.ExpectExec("INSERT INTO scheduler_outbox").WillReturnResult(sqlmock.NewResult(0, 1))
			}
			ctx := service.WithAccountOwnerScope(context.Background(), tc.actor, 7)
			shadow, err := newOwnerShadowAdminService(repo).CreateShadow(ctx, 11, service.ShadowOptions{})
			if !tc.allowed {
				require.ErrorIs(t, err, service.ErrAccountNotFound)
				require.Nil(t, shadow)
			} else {
				require.NoError(t, err)
				require.Equal(t, int64(22), shadow.ID)
				if tc.owner == nil {
					require.Nil(t, shadow.CreatedBy, "a legacy NULL owner must not become the creating admin")
				} else {
					require.Equal(t, tc.owner, *shadow.CreatedBy)
				}
				// Read back through the real repository as the inherited uploader.
				// The owned case proves root-created shadows remain visible to them.
				expectAccountOwnerRead(mock, shadow.ID, tc.owner)
				readCtx := ctx
				if tc.owner != nil {
					ownerID, ok := tc.owner.(int64)
					require.True(t, ok, "test owner must be an int64")
					readCtx = service.WithAccountOwnerScope(context.Background(), ownerID, 7)
				}
				got, readErr := repo.GetByID(readCtx, shadow.ID)
				require.NoError(t, readErr)
				require.Equal(t, shadow.CreatedBy, got.CreatedBy)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAccountOwnerBatchMapping(t *testing.T) {
	for _, kind := range []string{"GetByIDs", "ListActive", "ListShadowsByParent"} {
		t.Run(kind, func(t *testing.T) {
			repo, _, mock := newAccountOwnerReadRepo(t)
			mock.MatchExpectationsInOrder(false)
			mock.ExpectQuery(`SELECT .* FROM "accounts"`).
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(11)).AddRow(int64(22)).AddRow(int64(33)))
			if kind != "ListShadowsByParent" {
				mock.ExpectQuery(`FROM "account_groups"`).WithArgs(int64(11), int64(22), int64(33)).
					WillReturnRows(sqlmock.NewRows([]string{"account_id", "group_id", "priority", "created_at"}))
			}
			mock.ExpectQuery(regexp.QuoteMeta(accountOwnerReadSQL)).WithArgs("{11,22,33}").
				WillReturnRows(sqlmock.NewRows([]string{"id", "created_by"}).
					AddRow(int64(33), int64(9)).AddRow(int64(11), int64(8)).AddRow(int64(22), nil))
			ctx := service.WithAccountOwnerScope(context.Background(), 7, 7)
			var got []*service.Account
			var err error
			switch kind {
			case "GetByIDs":
				got, err = repo.GetByIDs(ctx, []int64{33, 22, 11, 33, 44, -1})
			case "ListActive":
				var values []service.Account
				values, err = repo.ListActive(ctx)
				for i := range values {
					got = append(got, &values[i])
				}
			case "ListShadowsByParent":
				got, err = repo.ListShadowsByParent(ctx, 55)
			}
			require.NoError(t, err)
			require.Len(t, got, 3)
			if kind == "GetByIDs" {
				require.Equal(t, []int64{33, 22, 11}, []int64{got[0].ID, got[1].ID, got[2].ID})
			}
			for _, account := range got {
				switch account.ID {
				case 11:
					require.Equal(t, int64(8), *account.CreatedBy)
				case 22:
					require.Nil(t, account.CreatedBy)
				case 33:
					require.Equal(t, int64(9), *account.CreatedBy)
				}
			}
			require.NoError(t, mock.ExpectationsWereMet(), "owners must be loaded with one batch query, never per account")
		})
	}
}

func TestAccountOwnerReadUsesEntityTransaction(t *testing.T) {
	repo, db, mock := newAccountOwnerReadRepo(t)
	// With one connection, using the pool for owner enrichment would wait for
	// this transaction forever. The context bounds that failure in the test.
	db.SetMaxOpenConns(1)
	mock.ExpectBegin()
	tx, err := repo.client.Tx(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	transactional := newAccountRepositoryWithSQL(tx.Client(), db, nil)
	expectAccountOwnerRead(mock, 11, int64(8))
	ctx, cancel := context.WithTimeout(service.WithAccountOwnerScope(context.Background(), 8, 7), time.Second)
	defer cancel()
	got, err := transactional.GetByID(ctx, 11)
	require.NoError(t, err)
	require.Equal(t, int64(8), *got.CreatedBy)
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountOwnerReadFailureStopsShadowCreation(t *testing.T) {
	for _, kind := range []string{"query", "scan", "row iteration"} {
		t.Run(kind, func(t *testing.T) {
			repo, _, mock := newAccountOwnerReadRepo(t)
			mock.ExpectQuery(`SELECT .* FROM "accounts"`).WithArgs(int64(11)).
				WillReturnRows(sqlmock.NewRows([]string{"id", "platform", "type"}).AddRow(int64(11), "openai", "oauth"))
			mock.ExpectQuery(`FROM "account_groups"`).WithArgs(int64(11)).
				WillReturnRows(sqlmock.NewRows([]string{"account_id", "group_id", "priority", "created_at"}))
			readErr := errors.New("owner read failed")
			expected := mock.ExpectQuery(regexp.QuoteMeta(accountOwnerReadSQL)).WithArgs("{11}")
			switch kind {
			case "query":
				expected.WillReturnError(readErr)
			case "scan":
				expected.WillReturnRows(sqlmock.NewRows([]string{"id", "created_by"}).AddRow(int64(11), "invalid bigint"))
			case "row iteration":
				expected.WillReturnRows(sqlmock.NewRows([]string{"id", "created_by"}).AddRow(int64(11), int64(8)).RowError(0, readErr))
			}
			shadow, err := newOwnerShadowAdminService(repo).CreateShadow(context.Background(), 11, service.ShadowOptions{})
			require.Error(t, err)
			if kind != "scan" {
				require.ErrorIs(t, err, readErr)
			}
			require.Nil(t, shadow)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
