package claude

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultModelsContainsClaudeFable51(t *testing.T) {
	t.Parallel()

	for _, model := range DefaultModels {
		if model.ID == "claude-fable-5-1" {
			require.Equal(t, "Claude Fable 5.1", model.DisplayName)
			require.Equal(t, "2026-09-01T00:00:00Z", model.CreatedAt)
			return
		}
	}
	t.Fatal("claude-fable-5-1 missing from DefaultModels")
}

func TestDefaultModelsRetainAliasesAndUniqueIDs(t *testing.T) {
	t.Parallel()
	ids := make(map[string]bool, len(DefaultModels))
	for _, model := range DefaultModels {
		require.False(t, ids[model.ID], "duplicate model: %s", model.ID)
		ids[model.ID] = true
	}
	for _, id := range []string{
		"claude-sonnet-5-5", "claude-haiku-5-5", "claude-opus-4-5",
		"claude-sonnet-4-5", "claude-haiku-4-5", "claude-fable-5-1",
		"claude-opus-4-5-20251101", "claude-sonnet-4-5-20250929", "claude-haiku-4-5-20251001",
	} {
		require.True(t, ids[id], "missing model: %s", id)
	}
}
