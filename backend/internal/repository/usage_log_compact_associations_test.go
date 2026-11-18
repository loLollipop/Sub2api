package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogCompactAssociationsProjectsDisplayFields(t *testing.T) {
	var queries []string
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(expected, actual string) error {
		require.Contains(t, actual, `FROM "`+expected+`"`)
		queries = append(queries, actual)
		return nil
	})))
	require.NoError(t, err)
	defer db.Close()
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	repo := &usageLogRepository{client: client, sql: db}
	deleted := time.Now().UTC()
	mock.ExpectQuery("users").WillReturnRows(sqlmock.NewRows([]string{"id", "email", "username", "deleted_at"}).AddRow(1, "deleted@example.test", "deleted", deleted))
	mock.ExpectQuery("api_keys").WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(2, "key display"))
	mock.ExpectQuery("accounts").WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(3, "account display"))
	mock.ExpectQuery("groups").WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(4, "group display"))
	groupID, subID := int64(4), int64(5)
	logs := []service.UsageLog{{UserID: 1, APIKeyID: 2, AccountID: 3, GroupID: &groupID, SubscriptionID: &subID}}
	require.NoError(t, repo.hydrateUsageLogAssociations(context.Background(), logs, true))
	require.True(t, logs[0].AssociationsCompact)
	require.Equal(t, "deleted@example.test", logs[0].User.Email)
	require.NotNil(t, logs[0].User.DeletedAt)
	require.Equal(t, "key display", logs[0].APIKey.Name)
	require.Empty(t, logs[0].APIKey.Key)
	require.Equal(t, "account display", logs[0].Account.Name)
	require.Empty(t, logs[0].Account.Credentials)
	require.Equal(t, "group display", logs[0].Group.Name)
	require.Nil(t, logs[0].Subscription)
	require.Len(t, queries, 4)
	for _, query := range queries {
		projection := strings.Split(query, " FROM ")[0]
		for _, field := range []string{"password_hash", "credentials", "model_pricing", "rate_multiplier", `"key"`} {
			require.NotContains(t, projection, field)
		}
	}
	require.NotContains(t, queries[0], `"users"."deleted_at" IS NULL`, "historical deleted identities still resolve")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogLegacyAssociationsKeepFullAPIKey(t *testing.T) {
	var query string
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(_, actual string) error { query = actual; return nil })))
	require.NoError(t, err)
	defer db.Close()
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	repo := &usageLogRepository{client: client, sql: db}
	mock.ExpectQuery("api keys").WillReturnRows(sqlmock.NewRows([]string{"id", "name", "key"}).AddRow(2, "legacy", "synthetic-test-key"))
	keys, err := repo.loadAPIKeys(context.Background(), []int64{2}, false)
	require.NoError(t, err)
	require.Equal(t, "synthetic-test-key", keys[2].Key)
	require.Contains(t, query, `"api_keys"."key"`)
	require.NoError(t, mock.ExpectationsWereMet())
}
