package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInlineThinkStreamSelectsOneReasoningSource(t *testing.T) {
	for _, tc := range []struct {
		name     string
		deltas   []string
		thinking string
	}{
		{"same chunk", []string{`{"reasoning_content":"计划","content":"<think>计划</think>答案"}`}, "计划"},
		{"explicit first", []string{`{"reasoning_content":"计划"}`, `{"content":"<think>另一份</think>答案"}`}, "计划"},
		{"inline first", []string{`{"content":"<think>计划"}`, `{"reasoning_content":"另一份","content":"</think>答案"}`}, "计划"},
		{"empty explicit", []string{`{"reasoning_content":"","content":"<think>计划</think>答案"}`}, "计划"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			responses := NewChatCompletionsToResponsesStreamState("model")
			anthropic := NewChatCompletionsToAnthropicStreamState("model")
			var resEvents []ResponsesStreamEvent
			var anthEvents []AnthropicStreamEvent
			for _, delta := range tc.deltas {
				var chunk ChatCompletionsChunk
				require.NoError(t, json.Unmarshal([]byte(`{"choices":[{"index":0,"delta":`+delta+`}]}`), &chunk))
				resEvents = append(resEvents, ChatCompletionsChunkToResponsesEvents(&chunk, responses)...)
				anthEvents = append(anthEvents, ChatCompletionsChunkToAnthropicEvents(&chunk, anthropic)...)
			}
			resEvents = append(resEvents, FinalizeChatCompletionsResponsesStream(responses)...)
			anthEvents = append(anthEvents, FinalizeChatCompletionsAnthropicStream(anthropic)...)
			var resThinking, resText, anthThinking, anthText strings.Builder
			for _, event := range resEvents {
				switch event.Type {
				case "response.reasoning_summary_text.delta":
					_, _ = resThinking.WriteString(event.Delta)
				case "response.output_text.delta":
					_, _ = resText.WriteString(event.Delta)
				}
			}
			for _, event := range anthEvents {
				if event.Delta == nil {
					continue
				}
				switch event.Delta.Type {
				case "thinking_delta":
					_, _ = anthThinking.WriteString(event.Delta.Thinking)
				case "text_delta":
					_, _ = anthText.WriteString(event.Delta.Text)
				}
			}
			require.Equal(t, tc.thinking, resThinking.String())
			require.Equal(t, tc.thinking, anthThinking.String())
			require.Equal(t, "答案", resText.String())
			require.Equal(t, "答案", anthText.String())
			require.Empty(t, FinalizeChatCompletionsResponsesStream(responses))
			require.Empty(t, FinalizeChatCompletionsAnthropicStream(anthropic))
		})
	}
}

func TestAnthropicInlineThinkOnlyFallbackAndToolExclusion(t *testing.T) {
	for _, withTool := range []bool{false, true} {
		state := NewChatCompletionsToAnthropicStreamState("model")
		var events []AnthropicStreamEvent
		for _, content := range []string{"<th", "ink>最终", "答案</think>"} {
			chunk := &ChatCompletionsChunk{Choices: []ChatChunkChoice{{Delta: ChatDelta{Content: &content}}}}
			events = append(events, ChatCompletionsChunkToAnthropicEvents(chunk, state)...)
		}
		if withTool {
			var chunk ChatCompletionsChunk
			require.NoError(t, json.Unmarshal([]byte(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]}}]}`), &chunk))
			events = append(events, ChatCompletionsChunkToAnthropicEvents(&chunk, state)...)
		}
		events = append(events, FinalizeChatCompletionsAnthropicStream(state)...)
		var text strings.Builder
		starts, stops := map[int]int{}, map[int]int{}
		for _, event := range events {
			if event.Type == "content_block_start" {
				starts[*event.Index]++
			}
			if event.Type == "content_block_stop" {
				stops[*event.Index]++
			}
			if event.Delta != nil && event.Delta.Type == "text_delta" {
				_, _ = text.WriteString(event.Delta.Text)
			}
		}
		require.Equal(t, starts, stops, "all thinking, fallback and tool blocks must close")
		if withTool {
			require.Empty(t, text.String(), "tool turns must not gain a fallback answer")
		} else {
			require.Equal(t, "最终答案", text.String())
		}
	}
}

func TestAssistantReasoningHistoryPreservesWhitespaceAndPortableMessage(t *testing.T) {
	reasoning := "\n  缩进计划\n\u3000"
	request := &ChatCompletionsRequest{Model: "model", Messages: []ChatMessage{{Role: "assistant", ReasoningContent: reasoning, Content: json.RawMessage(`"answer"`)}}}
	converted, err := ChatCompletionsToResponses(request)
	require.NoError(t, err)
	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(converted.Input, &items))
	require.Len(t, items, 1)
	require.Equal(t, "message", items[0].Type)
	var parts []ResponsesContentPart
	require.NoError(t, json.Unmarshal(items[0].Content, &parts))
	require.Equal(t, "<thinking>"+reasoning+"</thinking>\nanswer", parts[0].Text)
}

func TestLocalShellAnthropicActualWireLifecycle(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	var events []AnthropicStreamEvent
	for _, raw := range []string{
		`{"type":"response.output_item.added","output_index":0,"item":{"id":"shell_1","type":"local_shell_call","call_id":"call_1","status":"in_progress"}}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"id":"shell_1","type":"local_shell_call","call_id":"call_1","status":"completed","action":{"type":"exec","command":["pwd"],"working_directory":"/tmp"}}}`,
		`{"type":"response.completed","response":{"id":"resp_1","status":"completed","usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}}`,
	} {
		var event ResponsesStreamEvent
		require.NoError(t, json.Unmarshal([]byte(raw), &event))
		events = append(events, ResponsesEventToAnthropicEvents(&event, state)...)
	}
	starts, stops, arguments, ends := 0, 0, 0, 0
	for _, event := range events {
		switch event.Type {
		case "content_block_start":
			starts++
			require.Equal(t, "local_shell", event.ContentBlock.Name)
			require.Equal(t, "call_1", event.ContentBlock.ID)
		case "content_block_delta":
			if event.Delta.Type == "input_json_delta" {
				arguments++
				require.JSONEq(t, `{"type":"exec","command":["pwd"],"working_directory":"/tmp"}`, event.Delta.PartialJSON)
			}
		case "content_block_stop":
			stops++
		case "message_stop":
			ends++
		}
	}
	require.Equal(t, 1, starts)
	require.Equal(t, 1, stops)
	require.Equal(t, 1, arguments)
	require.Equal(t, 1, ends)
}

func TestLocalShellAnthropicInterleavedCallsKeepArgumentsAndLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  []string
		want map[string]string
	}{
		{"two shells", []string{
			`{"type":"response.output_item.added","output_index":0,"item":{"type":"local_shell_call","call_id":"call_a"}}`,
			`{"type":"response.output_item.added","output_index":1,"item":{"type":"local_shell_call","call_id":"call_b"}}`,
			`{"type":"response.output_item.done","output_index":0,"item":{"type":"local_shell_call","call_id":"call_a","action":{"type":"exec","command":["pwd"]}}}`,
			`{"type":"response.output_item.done","output_index":1,"item":{"type":"local_shell_call","call_id":"call_b","action":{"type":"exec","command":["ls"]}}}`,
		}, map[string]string{"call_a": `{"type":"exec","command":["pwd"]}`, "call_b": `{"type":"exec","command":["ls"]}`}},
		{"shell done during function arguments", []string{
			`{"type":"response.output_item.added","output_index":0,"item":{"type":"local_shell_call","call_id":"call_a"}}`,
			`{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","call_id":"call_b","name":"lookup"}}`,
			`{"type":"response.function_call_arguments.delta","output_index":1,"delta":"{\"x\":"}`,
			`{"type":"response.output_item.done","output_index":0,"item":{"type":"local_shell_call","call_id":"call_a","action":{"type":"exec","command":["pwd"]}}}`,
			`{"type":"response.function_call_arguments.delta","output_index":1,"delta":"1}"}`,
			`{"type":"response.function_call_arguments.done","output_index":1,"arguments":"{\"x\":1}"}`,
			`{"type":"response.output_item.done","output_index":1,"item":{"type":"function_call","call_id":"call_b","name":"lookup","arguments":"{\"x\":1}"}}`,
		}, map[string]string{"call_a": `{"type":"exec","command":["pwd"]}`, "call_b": `{"x":1}`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, syntheticEnd := range []bool{false, true} {
				state := NewResponsesEventToAnthropicState()
				var events []AnthropicStreamEvent
				created := &ResponsesStreamEvent{Type: "response.created"}
				events = append(events, ResponsesEventToAnthropicEvents(created, state)...)
				for _, raw := range tc.raw {
					var event ResponsesStreamEvent
					require.NoError(t, json.Unmarshal([]byte(raw), &event))
					events = append(events, ResponsesEventToAnthropicEvents(&event, state)...)
				}
				if syntheticEnd {
					events = append(events, FinalizeResponsesAnthropicStream(state)...)
				} else {
					completed := &ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{Status: "completed"}}
					events = append(events, ResponsesEventToAnthropicEvents(completed, state)...)
				}
				open := map[int]bool{}
				ids := map[int]string{}
				args := map[string]string{}
				stopped := false
				for _, event := range events {
					require.False(t, stopped, "no events after message_stop")
					switch event.Type {
					case "content_block_start":
						require.Empty(t, open, "Anthropic blocks must remain sequential")
						idx := *event.Index
						open[idx] = true
						ids[idx] = event.ContentBlock.ID
						args[ids[idx]] = ""
					case "content_block_delta":
						require.True(t, open[*event.Index], "delta must target an open block")
						if event.Delta.Type == "input_json_delta" {
							args[ids[*event.Index]] += event.Delta.PartialJSON
						}
					case "content_block_stop":
						require.True(t, open[*event.Index])
						delete(open, *event.Index)
					case "message_stop":
						stopped = true
					}
				}
				require.Empty(t, open)
				require.True(t, stopped)
				require.Len(t, args, len(tc.want))
				for id, want := range tc.want {
					require.JSONEq(t, want, args[id], id)
				}
				require.Empty(t, FinalizeResponsesAnthropicStream(state))
			}
		})
	}
}
