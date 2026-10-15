package repository

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Reuses the explicitly opted-in local disposable schema, never the deployment
// database. pg_trgm must already be installed: this test does not install a
// database-wide extension or mutate objects outside its generated schema.
func TestRequestIDSearchPostgresIndexDefinitionAndResolvedSchema(t *testing.T) {
	db, ctx := isolatedPostgres(t)
	conn, err := db.Conn(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close()) }()
	var schema, extensionSchema string
	require.NoError(t, conn.QueryRowContext(ctx, `SELECT current_schema()`).Scan(&schema))
	err = conn.QueryRowContext(ctx, `SELECT n.nspname FROM pg_catalog.pg_extension e JOIN pg_catalog.pg_namespace n ON n.oid = e.extnamespace WHERE e.extname = 'pg_trgm'`).Scan(&extensionSchema)
	if errors.Is(err, sql.ErrNoRows) {
		t.Skip("local pg_trgm extension is not available; no extension installed by this test")
	}
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, "SET search_path TO "+pq.QuoteIdentifier(schema)+", "+pq.QuoteIdentifier(extensionSchema))
	require.NoError(t, err)
	require.NoError(t, requireRequestIDSearchTrigram(ctx, conn))
	_, err = conn.ExecContext(ctx, `CREATE TABLE usage_logs (request_id TEXT, upstream_request_id TEXT)`)
	require.NoError(t, err)
	name := requestIDSearchIndexes[0].name
	for _, definition := range []string{
		`USING btree (request_id)`,
		`USING gin (upstream_request_id gin_trgm_ops)`,
		`USING gin (request_id gin_trgm_ops) WHERE request_id IS NOT NULL`,
		`USING gin (lower(request_id) gin_trgm_ops)`,
	} {
		_, err = conn.ExecContext(ctx, "CREATE INDEX "+pq.QuoteIdentifier(name)+" ON usage_logs "+definition)
		require.NoError(t, err)
		err = dropInvalidRequestIDSearchIndex(ctx, conn, "usage_logs", name, "request_id")
		require.ErrorContains(t, err, "incompatible definition", definition)
		_, err = conn.ExecContext(ctx, "DROP INDEX "+pq.QuoteIdentifier(schema)+"."+pq.QuoteIdentifier(name))
		require.NoError(t, err)
	}
	_, err = conn.ExecContext(ctx, "CREATE INDEX "+pq.QuoteIdentifier(name)+" ON usage_logs USING gin (request_id gin_trgm_ops)")
	require.NoError(t, err)
	require.NoError(t, dropInvalidRequestIDSearchIndex(ctx, conn, "usage_logs", name, "request_id"))
	var valid bool
	require.NoError(t, conn.QueryRowContext(ctx, `SELECT i.indisvalid FROM pg_catalog.pg_index i WHERE i.indexrelid = $1::regclass`, pq.QuoteIdentifier(schema)+"."+pq.QuoteIdentifier(name)).Scan(&valid))
	require.True(t, valid, "valid matching index must be retained in its non-public schema")
}
