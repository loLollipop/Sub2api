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
