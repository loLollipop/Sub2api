package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMixedPlatformMigrationIsAdditive(t *testing.T) {
	content, err := FS.ReadFile("255_add_mixed_platform.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")
	for _, column := range []string{"platform", "target_platform"} {
		require.Contains(t, sql, "CHECK ("+column+" IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi', 'zhipu', 'deepseek', 'minimax', 'mixed'))")
	}
	for _, mutation := range []string{"UPDATE accounts", "UPDATE groups", "DELETE FROM", "DROP TABLE"} {
		require.NotContains(t, sql, mutation)
	}
}
