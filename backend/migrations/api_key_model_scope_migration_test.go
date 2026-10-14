package migrations

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestAPIKeyModelScopeMigration(t *testing.T) {
	body, err := FS.ReadFile("256_api_key_model_scope.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(body)), " ")
	require.Contains(t, sql, "SET LOCAL lock_timeout = '5s'")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS model_scope VARCHAR(16) NOT NULL DEFAULT 'all'")
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION enqueue_api_key_auth_cache_invalidation()")
	for _, field := range []string{"key", "status", "deleted_at", "user_id", "group_id", "ip_whitelist", "ip_blacklist", "expires_at", "model_scope"} {
		require.Contains(t, sql, "OLD."+field+" IS DISTINCT FROM NEW."+field)
	}
	require.Contains(t, sql, "IF TG_OP = 'DELETE' THEN PERFORM enqueue_auth_cache_invalidation(OLD.key); RETURN OLD;")
	require.Contains(t, sql, "IF NEW.deleted_at IS NULL AND NEW.key IS DISTINCT FROM OLD.key THEN PERFORM enqueue_auth_cache_invalidation(NEW.key);")
	require.NotContains(t, sql, "UPDATE accounts")
	require.NotContains(t, sql, "DROP")
}
