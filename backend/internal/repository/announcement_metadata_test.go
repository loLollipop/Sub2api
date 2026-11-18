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
	"github.com/stretchr/testify/require"
)

func TestAnnouncementMetadataDoesNotSelectBodies(t *testing.T) {
	var query string
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(_, actual string) error { query = actual; return nil })))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	repo := &announcementRepository{client: client}
	mock.ExpectQuery("active metadata").WillReturnRows(sqlmock.NewRows([]string{"id", "title", "status", "notify_mode", "targeting"}).AddRow(1, "Title", "active", "popup", []byte(`{}`)))
	items, err := repo.ListActiveMetadata(context.Background(), time.Now())
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Empty(t, items[0].Content)
	require.NotContains(t, strings.Split(query, " FROM ")[0], `"content"`)
	require.Contains(t, query, `"targeting"`)
	require.Contains(t, query, "LIMIT 200")
	require.NoError(t, mock.ExpectationsWereMet())
}
