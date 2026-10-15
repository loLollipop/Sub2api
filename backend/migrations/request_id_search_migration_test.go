package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequestIDSearchMigrationRequiresTrigramBeforeConcurrentIndexes(t *testing.T) {
	body, err := FS.ReadFile("263_require_request_id_search_trgm.sql")
	require.NoError(t, err)
	preflight := string(body)
	require.Contains(t, preflight, "CREATE EXTENSION IF NOT EXISTS pg_trgm;")
	require.Contains(t, preflight, "pg_catalog.pg_opclass_is_visible(o.oid)")
	require.Contains(t, preflight, "pg_catalog.has_schema_privilege(n.oid, 'USAGE')")
	require.Contains(t, preflight, "RAISE EXCEPTION")
	require.NotContains(t, preflight, "RAISE NOTICE")
	require.NotContains(t, preflight, "WHEN OTHERS")
	body, err = FS.ReadFile("264_add_request_id_search_indexes_notx.sql")
	require.NoError(t, err)
	indexes := string(body)
	for _, name := range []string{"idx_usage_logs_request_id_trgm", "idx_usage_logs_upstream_request_id_trgm", "idx_ops_error_logs_request_id_trgm", "idx_ops_error_logs_client_request_id_trgm"} {
		require.Contains(t, indexes, "CREATE INDEX CONCURRENTLY IF NOT EXISTS "+name)
	}
	require.Equal(t, 4, strings.Count(indexes, "gin_trgm_ops"))
	for _, sql := range []string{preflight, indexes} {
		for _, forbidden := range []string{"UPDATE users", "UPDATE accounts", "DELETE FROM", "DROP TABLE", "DROP INDEX", "ALTER TABLE"} {
			require.NotContains(t, sql, forbidden)
		}
	}
}
