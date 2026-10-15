package apicompat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDedupeResponsesCallIDsRetainsLaterRealArguments(t *testing.T) {
	for _, blank := range []any{nil, "", " \n ", "null", "{}", " { \n } "} {
		t.Run(testArgumentLabel(t, blank), func(t *testing.T) {
			const arguments = `{"path":"src/main.go","line":9007199254740993}`
			input := []any{
				map[string]any{"type": "function_call", "id": "call_a", "call_id": "call_a", "arguments": blank},
				map[string]any{"type": "function_call_output", "call_id": "call_a", "output": "{}"},
				map[string]any{"type": "function_call", "id": "duplicate_a", "call_id": "call_a", "name": "read", "arguments": arguments},
				map[string]any{"type": "function_call_output", "call_id": "call_a", "output": "discarded"},
				map[string]any{"type": "function_call", "id": "call_b", "call_id": "call_b", "name": "read", "arguments": arguments},
				map[string]any{"type": "function_call_output", "call_id": "call_b", "output": "other"},
				map[string]any{"type": "item_reference", "id": "call_a"},
			}

			out, changed := DedupeResponsesCallIDs(input)
			require.True(t, changed)
			items, ok := out.([]any)
			require.True(t, ok)
			require.Len(t, items, 5)
			require.Equal(t, arguments, responseItemField(t, items, 0, "arguments"))
			require.Equal(t, "read", responseItemField(t, items, 0, "name"))
			require.Equal(t, "call_a", responseItemField(t, items, 0, "id"))
			require.Equal(t, "{}", responseItemField(t, items, 1, "output"))
			require.Equal(t, "call_b", responseItemField(t, items, 2, "call_id"))
			require.Equal(t, "other", responseItemField(t, items, 3, "output"))
			require.Equal(t, "call_a", responseItemField(t, items, 4, "id"))
			same, changed := DedupeResponsesCallIDs(out)
			require.False(t, changed)
			require.Equal(t, out, same)
		})
	}
}

func TestDedupeChatToolCallIDsRetainsLaterRealArguments(t *testing.T) {
	for _, blank := range []any{nil, "", " \n ", "null", "{}", " { \n } "} {
		t.Run(testArgumentLabel(t, blank), func(t *testing.T) {
			const arguments = `{"command":"pwd"}`
			firstFunction := map[string]any{"arguments": blank}
			messages := []any{
				map[string]any{"role": "assistant", "content": "before", "tool_calls": []any{
					map[string]any{"id": "call_a", "type": "function", "function": firstFunction},
				}},
				map[string]any{"role": "tool", "tool_call_id": "call_a", "content": "{}"},
				map[string]any{"role": "assistant", "content": "after", "tool_calls": []any{
					map[string]any{"id": "call_a", "type": "function", "function": map[string]any{"name": "run", "arguments": arguments}},
					map[string]any{"id": "call_b", "type": "function", "function": map[string]any{"name": "run", "arguments": arguments}},
				}},
				map[string]any{"role": "tool", "tool_call_id": "call_a", "content": "discarded"},
				map[string]any{"role": "tool", "tool_call_id": "call_b", "content": "other"},
			}

			out, changed := DedupeChatToolCallIDs(messages)
			require.True(t, changed)
			items, ok := out.([]any)
			require.True(t, ok)
			require.Len(t, items, 4)
			require.Equal(t, arguments, firstFunction["arguments"])
			require.Equal(t, "run", firstFunction["name"])
			require.Equal(t, "before", responseItemField(t, items, 0, "content"))
			require.Equal(t, "{}", responseItemField(t, items, 1, "content"))
			require.Equal(t, "after", responseItemField(t, items, 2, "content"))
			message, ok := items[2].(map[string]any)
			require.True(t, ok)
			calls, ok := message["tool_calls"].([]any)
			require.True(t, ok)
			require.Len(t, calls, 1)
			require.Equal(t, "call_b", responseItemField(t, calls, 0, "id"))
			same, changed := DedupeChatToolCallIDs(out)
			require.False(t, changed)
			require.Equal(t, out, same)
		})
	}
}

func testArgumentLabel(t *testing.T, value any) string {
	t.Helper()
	if value == nil {
		return "null"
	}
	label, ok := value.(string)
	require.True(t, ok)
	return "string_" + label
}

func TestDedupeFunctionArgumentsPreservesMeaningfulAndConflictingCalls(t *testing.T) {
	for _, tc := range []struct {
		name      string
		firstType string
		firstName string
		firstArgs any
		nextType  string
		nextName  string
		nextArgs  any
	}{
		{"real arguments", "function_call", "run", `{"cmd":"first"}`, "function_call", "run", `{"cmd":"later"}`},
		{"conflicting name", "function_call", "run", "{}", "function_call", "delete", `{"path":"file"}`},
		{"partial later JSON", "function_call", "run", "{}", "function_call", "run", `{"cmd":`},
		{"unknown argument shape", "function_call", "run", map[string]any{}, "function_call", "run", `{"cmd":"later"}`},
		{"different call type", "function_call", "run", "{}", "custom_tool_call", "run", `{"cmd":"later"}`},
		{"non-function first call", "custom_tool_call", "run", "{}", "function_call", "run", `{"cmd":"later"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first := map[string]any{"type": tc.firstType, "call_id": "call_a", "name": tc.firstName, "arguments": tc.firstArgs}
			out, changed := DedupeResponsesCallIDs([]any{
				first,
				map[string]any{"type": tc.nextType, "call_id": "call_a", "name": tc.nextName, "arguments": tc.nextArgs},
			})
			require.True(t, changed)
			items, ok := out.([]any)
			require.True(t, ok)
			require.Len(t, items, 1)
			require.Equal(t, tc.firstArgs, first["arguments"])
			require.Equal(t, tc.firstName, first["name"])
		})
		t.Run("chat_"+tc.name, func(t *testing.T) {
			firstFunction := map[string]any{"name": tc.firstName, "arguments": tc.firstArgs}
			chatType := func(responseType string) string {
				if responseType == "function_call" {
					return "function"
				}
				return "custom"
			}
			first := map[string]any{"id": "call_a", "type": chatType(tc.firstType), "function": firstFunction}
			out, changed := DedupeChatToolCallIDs([]any{
				map[string]any{"role": "assistant", "tool_calls": []any{
					first,
					map[string]any{"id": "call_a", "type": chatType(tc.nextType), "function": map[string]any{"name": tc.nextName, "arguments": tc.nextArgs}},
				}},
				map[string]any{"role": "tool", "tool_call_id": "call_a", "content": "{}"},
			})
			require.True(t, changed)
			items, ok := out.([]any)
			require.True(t, ok)
			require.Len(t, items, 2)
			message, ok := items[0].(map[string]any)
			require.True(t, ok)
			calls, ok := message["tool_calls"].([]any)
			require.True(t, ok)
			require.Len(t, calls, 1)
			require.Equal(t, "call_a", responseItemField(t, calls, 0, "id"))
			require.Equal(t, tc.firstArgs, firstFunction["arguments"])
			require.Equal(t, tc.firstName, firstFunction["name"])
			require.Equal(t, "{}", responseItemField(t, items, 1, "content"))
		})
	}
}

func TestDedupeChatToolCallIDsRetainsArgumentsWithinOneTurn(t *testing.T) {
	first := map[string]any{"id": "call_a", "type": "function"}
	const arguments = `{"cmd":"pwd"}`
	out, changed := DedupeChatToolCallIDs([]any{
		map[string]any{"role": "assistant", "tool_calls": []any{
			first,
			map[string]any{"id": "call_a", "type": "function", "function": map[string]any{"name": "run", "arguments": arguments}},
			map[string]any{"id": "call_b", "type": "function", "function": map[string]any{"name": "run", "arguments": arguments}},
		}},
	})
	require.True(t, changed)
	items, ok := out.([]any)
	require.True(t, ok)
	require.Len(t, items, 1)
	message, ok := items[0].(map[string]any)
	require.True(t, ok)
	calls, ok := message["tool_calls"].([]any)
	require.True(t, ok)
	require.Len(t, calls, 2)
	function, ok := first["function"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, arguments, function["arguments"])
	require.Equal(t, "call_a", first["id"])
	require.Equal(t, "call_b", responseItemField(t, calls, 1, "id"))
}
