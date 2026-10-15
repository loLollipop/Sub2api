package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeminiCachedContentMigrationRetainsTargetPricingAndLease(t *testing.T) {
	body, err := FS.ReadFile("260_gemini_cached_contents.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(body)), " ")
	for _, column := range []string{"api_key_id", "user_id", "group_id", "account_id", "model", "channel_mapped_model", "upstream_model", "pricing_at", "channel_usage", "mutation_token", "mutation_lease_until"} {
		require.Contains(t, sql, column)
	}
	require.Contains(t, sql, "CHECK ((mutation_token IS NULL) = (mutation_lease_until IS NULL))")
	for _, unsafe := range []string{"DROP TABLE", "DELETE FROM", "UPDATE accounts", "ALTER TABLE usage_logs"} {
		require.NotContains(t, sql, unsafe)
	}
}

func TestGeminiCachedContentEndpointMigrationDoesNotGuessLegacyIdentity(t *testing.T) {
	body, err := FS.ReadFile("261_gemini_cached_content_endpoint.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(body)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS upstream_endpoint_identity VARCHAR(67) NOT NULL DEFAULT ''")
	require.Contains(t, sql, "^v1:[0-9a-f]{64}$")
	for _, unsafe := range []string{"UPDATE ", "DELETE ", "DROP ", "FROM accounts"} {
		require.NotContains(t, sql, unsafe)
	}
}
