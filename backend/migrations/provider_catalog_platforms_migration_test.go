package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProviderCatalogPlatformMigrationPreservesChecks(t *testing.T) {
	body, err := FS.ReadFile("258_add_provider_catalog_platforms.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(body)), " ")
	for _, field := range []string{"platform", "target_platform"} {
		require.Contains(t, sql, "CHECK ("+field+" IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi', 'zhipu', 'deepseek', 'minimax', 'mixed', 'command_code', 'cline'))")
	}
	for _, value := range []string{"opencode_go", "typesafe", "UPDATE accounts", "UPDATE groups", "DELETE FROM", "DROP TABLE"} {
		require.NotContains(t, sql, value)
	}
}
