package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGatewayStabilityResponsesDuplicatePairs(t *testing.T) {
	var input any
	require.NoError(t, json.Unmarshal([]byte(`[
		{"type":"function_call","call_id":"a","name":"lookup"},
		{"type":"function_call","call_id":"b","name":"lookup"},
		{"type":"function_call_output","call_id":"a","output":"first"},
		{"type":"function_call","call_id":"a","name":"lookup"},
		{"type":"function_call_output","call_id":"a","output":"second"},
		{"type":"function_call_output","call_id":"b","output":"parallel"},
		{"type":"message","role":"user","content":"next"}
	]`), &input))
	out, changed := DedupeResponsesCallIDs(input)
	require.True(t, changed)
	items, ok := out.([]any)
	require.True(t, ok)
	require.Len(t, items, 5)
	require.Equal(t, "first", responseItemField(t, items, 2, "output"))
	require.Equal(t, "parallel", responseItemField(t, items, 3, "output"))
	again, changed := DedupeResponsesCallIDs(out)
	require.False(t, changed)
	require.Equal(t, out, again)
}

func TestGatewayStabilityChatDuplicatePairsLeaveNoEmptyTurn(t *testing.T) {
	for _, content := range []string{``, `,"content":null`, `,"content":" "`, `,"content":"visible"`, `,"text":"visible"`} {
		t.Run(content, func(t *testing.T) {
			var input any
			require.NoError(t, json.Unmarshal([]byte(`[
				{"role":"assistant","tool_calls":[{"id":"a","type":"function"},{"id":"b","type":"function"}]},
				{"role":"tool","tool_call_id":"a","content":"first"},
				{"role":"assistant","tool_calls":[{"id":"a","type":"function"}]`+content+`},
				{"role":"tool","tool_call_id":"a","content":"duplicate"},
				{"role":"tool","tool_call_id":"b","content":"parallel"}
			]`), &input))
			out, changed := DedupeChatToolCallIDs(input)
			require.True(t, changed)
			items, ok := out.([]any)
			require.True(t, ok)
			want := 3
			if content == `,"content":"visible"` || content == `,"text":"visible"` {
				want++
				message, ok := items[2].(map[string]any)
				require.True(t, ok)
				require.NotContains(t, message, "tool_calls")
			}
			require.Len(t, items, want)
			require.Equal(t, "first", responseItemField(t, items, 1, "content"))
			require.Equal(t, "parallel", responseItemField(t, items, len(items)-1, "content"))
			again, changed := DedupeChatToolCallIDs(out)
			require.False(t, changed)
			require.Equal(t, out, again)
		})
	}
}

func TestGatewayStabilityChatSingleTurnDuplicatePreservesFirst(t *testing.T) {
	input := []any{map[string]any{"role": "assistant", "tool_calls": []any{
		map[string]any{"id": "a"}, map[string]any{"id": "a"},
	}}}
	out, changed := DedupeChatToolCallIDs(input)
	require.True(t, changed)
	items, ok := out.([]any)
	require.True(t, ok)
	require.Len(t, items, 1)
	require.Len(t, responseItemField(t, items, 0, "tool_calls"), 1)
}

func TestGatewayStabilityDuplicateOrphans(t *testing.T) {
	for _, itemType := range []string{"function_call", "function_call_output"} {
		input := []any{map[string]any{"type": itemType, "call_id": "a"}, map[string]any{"type": itemType, "call_id": "a"}}
		out, changed := DedupeResponsesCallIDs(input)
		require.True(t, changed)
		require.Len(t, out, 1)
	}
	input := []any{
		map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "a"}}},
		map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "a"}}},
	}
	out, changed := DedupeChatToolCallIDs(input)
	require.True(t, changed)
	require.Len(t, out, 1)
}
