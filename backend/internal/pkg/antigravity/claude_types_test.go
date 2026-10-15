package antigravity

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClaudeContentItem_ThinkingAlwaysHasSignature(t *testing.T) {
	t.Parallel()
	for _, thinking := range []string{"", "plan"} {
		for _, signature := range []string{"", "signed"} {
			data, err := json.Marshal(ClaudeContentItem{Type: "thinking", Thinking: thinking, Signature: signature})
			require.NoError(t, err)
			var item map[string]any
			require.NoError(t, json.Unmarshal(data, &item))
			require.Equal(t, map[string]any{"type": "thinking", "thinking": thinking, "signature": signature}, item)
		}
	}
	data, err := json.Marshal(ClaudeContentItem{Type: "text", Text: "hi"})
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"text","text":"hi"}`, string(data))
	data, err = json.Marshal(ClaudeContentItem{Type: "tool_use", ID: "tool-1", Name: "search", Input: map[string]any{"query": "test"}})
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"tool_use","id":"tool-1","name":"search","input":{"query":"test"}}`, string(data))
}

func TestDefaultModels_ContainsNewAndLegacyImageModels(t *testing.T) {
	t.Parallel()

	models := DefaultModels()
	byID := make(map[string]ClaudeModel, len(models))
	for _, m := range models {
		byID[m.ID] = m
	}

	requiredIDs := []string{
		"claude-fable-5-1",
		"claude-fable-5",
		"claude-opus-4-8",
		"claude-opus-4-6-thinking",
		"gemini-2.5-flash-image",
		"gemini-2.5-flash-image-preview",
		"gemini-3.1-flash-image",
		"gemini-3.1-flash-image-preview",
		"gemini-3-pro-image", // legacy compatibility
		"gemini-3.6-flash",
		"gemini-3.6-flash-high",
		"gemini-3.6-flash-low",
		"gemini-3.6-flash-medium",
		"gemini-3.6-flash-tiered",
		"gemini-3.7-flash",
		"gemini-3.7-flash-high",
		"gemini-3.7-flash-low",
		"gemini-3.7-flash-medium",
		"gemini-3.7-flash-tiered",
		"gemini-3.8-flash",
		"gemini-3.8-flash-high",
		"gemini-3.8-flash-low",
		"gemini-3.8-flash-medium",
		"gemini-3.8-flash-tiered",
		"gemini-4-argon-high",
		"gemini-4-argon-low",
		"gemini-4-argon-medium",
		"claude-opus-5-5-high",
		"claude-opus-5-5-low",
		"claude-opus-5-5-medium",
		"claude-sonnet-5-5-high",
		"claude-sonnet-5-5-low",
		"claude-sonnet-5-5-medium",
	}

	for _, id := range requiredIDs {
		if _, ok := byID[id]; !ok {
			t.Fatalf("expected model %q to be exposed in DefaultModels", id)
		}
	}
}
