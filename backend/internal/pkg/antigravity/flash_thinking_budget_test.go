package antigravity

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildGenerationConfig_FlashThinkingBudget(t *testing.T) {
	for _, tc := range []struct {
		name, model, thinkingType                    string
		budget, maxTokens, wantBudget, wantMaxTokens int
	}{
		{"flash 2.5", "gemini-2.5-flash", "enabled", 42000, 64000, 24576, 64000},
		{"flash 3", "gemini-3-flash", "enabled", 42000, 64000, 24576, 64000},
		{"flash 3.6 high", "gemini-3.6-flash-high", "enabled", 42000, 64000, 24576, 64000},
		{"flash 3.7 high", "gemini-3.7-flash-high", "enabled", 42000, 64000, 24576, 64000},
		{"flash 3.8 high", "gemini-3.8-flash-high", "enabled", 42000, 64000, 24576, 64000},
		{"flash lite", "gemini-2.5-flash-lite", "enabled", 42000, 64000, 24576, 64000},
		{"future flash", "gemini-4-flash", "enabled", 42000, 64000, 24576, 64000},
		{"small explicit budget", "gemini-3.8-flash-high", "enabled", 1024, 64000, 1024, 64000},
		{"budget at limit", "gemini-3.8-flash-high", "enabled", 24576, 64000, 24576, 64000},
		{"output padded after cap", "gemini-3.8-flash-high", "enabled", 42000, 1024, 24576, 25576},
		{"dynamic budget", "gemini-3.8-flash-high", "enabled", -1, 64000, -1, 64000},
		{"adaptive dynamic budget", "gemini-3.8-flash-high", "adaptive", 0, 64000, -1, 64000},
		{"adaptive explicit budget", "gemini-3.8-flash-high", "adaptive", 42000, 64000, 24576, 64000},
		{"pro budget unchanged", "gemini-3.1-pro-high", "enabled", 42000, 64000, 42000, 64000},
		{"argon budget unchanged", "gemini-4-argon-high", "enabled", 42000, 64000, 42000, 64000},
		{"claude budget unchanged", "claude-sonnet-4-5-thinking", "enabled", 42000, 64000, 42000, 64000},
		{"other flash alias unchanged", "claude-flash-custom", "enabled", 42000, 64000, 42000, 64000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &ClaudeRequest{Model: tc.model, MaxTokens: tc.maxTokens,
				Thinking: &ThinkingConfig{Type: tc.thinkingType, BudgetTokens: tc.budget}}
			config := buildGenerationConfig(req)
			require.NotNil(t, config.ThinkingConfig)
			require.True(t, config.ThinkingConfig.IncludeThoughts)
			require.Equal(t, tc.wantBudget, config.ThinkingConfig.ThinkingBudget)
			require.Equal(t, tc.wantMaxTokens, config.MaxOutputTokens)
			require.Equal(t, tc.budget, req.Thinking.BudgetTokens, "conversion must not mutate the caller's budget")
		})
	}
}

func TestTransformClaudeToGemini_FlashBudgetUsesFinalModel(t *testing.T) {
	for _, tc := range []struct {
		name, requested, mapped, wantModel string
		search                             bool
		wantBudget                         int
	}{
		{"mapped to flash", "claude-sonnet-4-5-thinking", "gemini-3.8-flash-high", "gemini-3.8-flash-high", false, 24576},
		{"mapped away from flash", "gemini-3.8-flash-high", "gemini-3.1-pro-high", "gemini-3.1-pro-high", false, 42000},
		{"search fallback to flash", "gemini-3.1-pro-high", "gemini-3.1-pro-high", "gemini-2.5-flash", true, 24576},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &ClaudeRequest{Model: tc.requested, MaxTokens: 64000,
				Thinking: &ThinkingConfig{Type: "enabled", BudgetTokens: 42000},
				Messages: []ClaudeMessage{{Role: "user", Content: json.RawMessage(`"hello"`)}}}
			if tc.search {
				req.Tools = []ClaudeTool{{Type: "web_search_20250305", Name: "web_search"}}
			}
			body, err := TransformClaudeToGeminiWithOptions(req, "fixture-project", tc.mapped, DefaultTransformOptions())
			require.NoError(t, err)
			var outgoing V1InternalRequest
			require.NoError(t, json.Unmarshal(body, &outgoing))
			require.Equal(t, tc.wantModel, outgoing.Model)
			require.NotNil(t, outgoing.Request.GenerationConfig)
			require.NotNil(t, outgoing.Request.GenerationConfig.ThinkingConfig)
			require.Equal(t, tc.wantBudget, outgoing.Request.GenerationConfig.ThinkingConfig.ThinkingBudget)
			require.Equal(t, 64000, outgoing.Request.GenerationConfig.MaxOutputTokens)
			require.Equal(t, tc.requested, req.Model)
			require.Equal(t, 42000, req.Thinking.BudgetTokens)
		})
	}
}
