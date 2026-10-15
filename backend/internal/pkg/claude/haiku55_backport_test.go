package claude

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestHaiku55NormalizationAndEfforts(t *testing.T) {
	for _, model := range []string{"claude-haiku-5-5", "anthropic/claude-haiku-5.5", " models/claude-haiku-5-5 ", "claude-haiku-5-5-20261001-thinking"} {
		require.True(t, IsHaiku55(model), model)
		require.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, EffortLevelsForModel(model))
	}
	for _, model := range []string{"claude-haiku-4-5", "claude-haiku-5-50", "qwen3.8-27b"} {
		require.False(t, IsHaiku55(model), model)
	}
	for _, model := range DefaultModels {
		if model.ID == "claude-haiku-5-5" {
			require.Equal(t, "Claude Haiku 5.5", model.DisplayName)
			return
		}
	}
	t.Fatal("Haiku 5.5 missing from default model catalog")
}
