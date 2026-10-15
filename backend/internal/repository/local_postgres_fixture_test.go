package repository

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Explicitly opt in to a LOCAL disposable database. This suite never discovers
// deployment credentials or connects to a configured production database.
func isolatedPostgres(t *testing.T) (*sql.DB, context.Context) {
	t.Helper()
	raw := os.Getenv("SUB2API_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("SUB2API_TEST_DATABASE_URL is not set")
	}
	u, err := localTestDatabaseURL(raw)
	require.NoError(t, err)
	root, err := sql.Open("postgres", raw)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	require.NoError(t, root.PingContext(ctx))
	schema := "local_integration_test_" + uuid.NewString()[:8]
	_, err = root.ExecContext(ctx, "CREATE SCHEMA "+pq.QuoteIdentifier(schema))
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		_, err := root.ExecContext(cleanup, "DROP SCHEMA "+pq.QuoteIdentifier(schema)+" CASCADE")
		require.NoError(t, err)
	})
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	db, err := sql.Open("postgres", u.String())
	require.NoError(t, err)
	db.SetMaxOpenConns(10)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db, ctx
}

func localTestDatabaseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Fragment != "" {
		return nil, fmt.Errorf("invalid cache journal local test database URL")
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
	default:
		return nil, fmt.Errorf("cache journal tests require a literal loopback database host")
	}
	// lib/pq processes query settings AFTER the URL authority. In particular a
	// second host can override the checked destination. Keep only explicitly
	// harmless settings; never include the original URL/credentials in errors.
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return nil, fmt.Errorf("invalid cache journal local test database parameters")
	}
	for key := range query {
		switch key {
		case "sslmode", "sslcert", "sslkey", "sslrootcert", "connect_timeout", "application_name":
		default:
			return nil, fmt.Errorf("unsupported cache journal test database parameter")
		}
	}
	return u, nil
}

func TestLocalPostgresURLRejectsDestinationOverrides(t *testing.T) {
	for _, raw := range []string{
		"postgres://localhost/test?host=remote.example", "postgres://localhost/test?h%6fst=remote.example",
		"postgres://localhost/test?hostaddr=203.0.113.1", "postgres://localhost/test?service=external",
		"postgres://localhost/test?options=external", "postgres://localhost/test?host=localhost&host=remote.example",
		"postgres://remote.example/test", "postgres://localhost.evil.invalid/test", "postgres://127.0.0.2/test",
		"postgres://localhost/test?sslmode=disable&bad%ZZ=1", "postgres://localhost/test#ignore", "file:///test",
	} {
		_, err := localTestDatabaseURL(raw)
		require.Error(t, err, raw)
	}
	for _, raw := range []string{
		"postgres://localhost/test?sslmode=disable", "postgresql://127.0.0.1:5432/test?connect_timeout=1",
		"postgres://[::1]/test?application_name=cache_test",
	} {
		_, err := localTestDatabaseURL(raw)
		require.NoError(t, err, raw)
	}
}
