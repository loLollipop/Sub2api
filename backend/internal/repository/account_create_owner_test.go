package repository

import (
	"context"
	"errors"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountCreateOwnerDefaultsPreserveLegacyShadow(t *testing.T) {
	for _, shadow := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal upload", true: "legacy shadow"}[shadow], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			repo := newAccountRepositoryWithSQL(client, db, nil)
			account := &service.Account{Name: "test", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
				Status: service.StatusActive, Concurrency: 2, Priority: 50}
			if shadow {
				parent := int64(11)
				account.ParentAccountID = &parent
				account.QuotaDimension = service.QuotaDimensionSpark
			}
			writeErr := errors.New("stop before persistence")
			mock.ExpectQuery(`INSERT INTO "accounts"`).WillReturnError(writeErr)
			err = repo.Create(service.WithAccountOwnerScope(context.Background(), 7, 7), account)
			require.ErrorIs(t, err, writeErr)
			if shadow {
				require.Nil(t, account.CreatedBy)
			} else {
				require.Equal(t, int64(7), *account.CreatedBy)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
