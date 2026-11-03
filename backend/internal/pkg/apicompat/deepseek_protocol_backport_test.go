package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeepSeekBackportTerminalReasoningAndBufferedSeparation(t *testing.T) {
	item := ResponsesOutput{Type: "reasoning", ID: "rs_one", Summary: []ResponsesSummary{{Type: "summary_text", Text: "display"}}, Content: []ResponsesContentPart{{Type: "reasoning_text", Text: "first"}, {Type: "reasoning_text", Text: "second"}}}
	terminal := ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{Status: "completed", Output: []ResponsesOutput{item}}}
	for _, mode := range []string{"terminal", "done", "delta", "item"} {
		t.Run(mode, func(t *testing.T) {
			chat := NewResponsesEventToChatState()
			chat.RequireReasoningContent = true
			anth := NewResponsesEventToAnthropicState()
			anth.RequireReasoningContent = true
			var events []ResponsesStreamEvent
			if mode != "terminal" {
				events = append(events, ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: 4, Item: &ResponsesOutput{Type: "reasoning", ID: "rs_one"}})
			}
			events = append(events, ResponsesStreamEvent{Type: "response.reasoning_summary_text.delta", OutputIndex: 4, Delta: "display"})
			if mode == "delta" {
				events = append(events, ResponsesStreamEvent{Type: "response.reasoning_text.delta", OutputIndex: 4, Delta: "first"})
			}
			if mode == "delta" || mode == "done" {
				events = append(events, ResponsesStreamEvent{Type: "response.reasoning_text.done", OutputIndex: 4, Text: "first"})
			}
			if mode != "terminal" {
				events = append(events, ResponsesStreamEvent{Type: "response.output_item.done", OutputIndex: 4, Item: &item})
			}
			events = append(events, terminal)
			var chunks []ChatCompletionsChunk
			var blocks []AnthropicStreamEvent
			for index := range events {
				chunks = append(chunks, ResponsesEventToChatChunks(&events[index], chat)...)
				blocks = append(blocks, ResponsesEventToAnthropicEvents(&events[index], anth)...)
			}
			require.Equal(t, "firstsecond", chatReasoningText(chunks))
			require.Equal(t, "firstsecond", anthropicReasoningText(blocks))
		})
	}
	for _, terminalOutput := range [][]ResponsesOutput{nil, {{Type: "reasoning", ID: "rs_one", Summary: item.Summary}}, {{Type: "message", Role: "assistant", Content: []ResponsesContentPart{{Type: "output_text", Text: "answer"}}}}} {
		acc := NewBufferedResponseAccumulator()
		for _, event := range []ResponsesStreamEvent{
			{Type: "response.output_item.added", OutputIndex: 0, Item: &ResponsesOutput{Type: "reasoning", ID: "rs_one"}},
			{Type: "response.reasoning_summary_text.delta", Delta: "display"},
			{Type: "response.reasoning_text.delta", Delta: "first"},
			{Type: "response.reasoning_text.done", Text: "first"},
			{Type: "response.reasoning_text.done", ContentIndex: 1, Text: "second"},
		} {
			acc.ProcessEvent(&event)
		}
		response := &ResponsesResponse{Output: terminalOutput}
		acc.SupplementResponseOutput(response)
		converted := ResponsesToChatCompletionsWithOptions(response, "alias", true)
		require.Equal(t, "first\nsecond", converted.Choices[0].Message.ReasoningContent)
	}
}

func TestDeepSeekBackportDoneWithoutAddedAndRequestProviderGate(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	state.RequireReasoningContent = true
	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.reasoning_text.done", OutputIndex: 2, Text: "genuine"}, state)
	require.Equal(t, "genuine", anthropicReasoningText(events))
	maxTokens := 17
	req := &ChatCompletionsRequest{Model: "public-alias", MaxTokens: &maxTokens, Messages: []ChatMessage{{Role: "assistant", ReasoningContent: "  genuine  ", Content: json.RawMessage(`"answer"`)}}}
	for _, preserve := range []bool{false, true} {
		converted, err := ChatCompletionsToResponsesWithMinOutputTokens(req, 512, &ChatCompletionsToResponsesOptions{PreserveReasoningContent: preserve})
		require.NoError(t, err)
		require.Equal(t, 512, *converted.MaxOutputTokens)
		if preserve {
			var items []ResponsesInputItem
			require.NoError(t, json.Unmarshal(converted.Input, &items))
			require.Equal(t, "  genuine  ", responsesInputReasoningContentText(items[0].Content))
			require.NotContains(t, string(items[1].Content), "genuine")
		} else {
			require.NotContains(t, string(converted.Input), "reasoning_text")
			require.Contains(t, string(converted.Input), "thinking")
		}
	}
	responses := &ResponsesRequest{Model: "public-alias", Reasoning: &ResponsesReasoning{Effort: "none"}, Input: json.RawMessage(`[{"type":"reasoning","content":[{"type":"reasoning_text","text":"genuine"}],"summary":[{"type":"summary_text","text":"display"}],"encrypted_content":"foreign"},{"role":"assistant","content":"answer"}]`)}
	converted, err := ResponsesToAnthropicRequestWithOptions(responses, &ResponsesToAnthropicOptions{PreserveReasoningText: true})
	require.NoError(t, err)
	require.Equal(t, "disabled", converted.Thinking.Type)
	require.Nil(t, converted.OutputConfig)
	require.Contains(t, string(converted.Messages[0].Content), "genuine")
	require.NotContains(t, string(converted.Messages[0].Content), "display")
	require.NotContains(t, string(converted.Messages[0].Content), "foreign")
	legacy, err := ResponsesToAnthropicRequest(responses)
	require.NoError(t, err)
	require.NotContains(t, string(legacy.Messages[0].Content), "genuine")
}

func TestDeepSeekBackportReasoningItemIDSurvivesCompactedTerminal(t *testing.T) {
	for _, mode := range []string{"delta", "done"} {
		t.Run(mode, func(t *testing.T) {
			event := ResponsesStreamEvent{Type: "response.reasoning_text." + mode, OutputIndex: 4, ItemID: "rs_one", Delta: "real", Text: "real"}
			response := &ResponsesResponse{Status: "completed", Output: []ResponsesOutput{{Type: "reasoning", ID: "rs_one", Content: []ResponsesContentPart{{Type: "reasoning_text", Text: "real"}}}}}
			acc := NewBufferedResponseAccumulator()
			acc.ProcessEvent(&event)
			acc.SupplementResponseOutput(response)
			require.Len(t, response.Output, 1, "same item at a compacted terminal index must not be appended twice")
			require.Equal(t, "real", ResponsesToChatCompletionsWithOptions(response, "alias", true).Choices[0].Message.ReasoningContent)
			state := NewResponsesEventToChatState()
			state.RequireReasoningContent = true
			chunks := ResponsesEventToChatChunks(&event, state)
			chunks = append(chunks, ResponsesEventToChatChunks(&ResponsesStreamEvent{Type: "response.completed", Response: response}, state)...)
			require.Equal(t, "real", chatReasoningText(chunks))
			anth := NewResponsesEventToAnthropicState()
			anth.RequireReasoningContent = true
			blocks := ResponsesEventToAnthropicEvents(&event, anth)
			blocks = append(blocks, ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.completed", Response: response}, anth)...)
			require.Equal(t, "real", anthropicReasoningText(blocks))
			// A later sparse snapshot must not erase the ID learned from a delta.
			acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.output_item.done", OutputIndex: 4, Item: &ResponsesOutput{Type: "reasoning"}})
			acc.SupplementResponseOutput(response)
			require.Len(t, response.Output, 1)
		})
	}
}

func TestDeepSeekBackportTerminalThinkingAfterBodyClosesEmptyBlock(t *testing.T) {
	for _, mode := range []string{"terminal_only", "item_done", "late_delta", "late_done"} {
		t.Run(mode, func(t *testing.T) {
			state := NewResponsesEventToAnthropicState()
			state.RequireReasoningContent = true
			item := &ResponsesOutput{Type: "reasoning", ID: "rs_one", Content: []ResponsesContentPart{{Type: "reasoning_text", Text: "real"}, {Type: "reasoning_text", Text: "second"}}}
			events := []ResponsesStreamEvent{
				{Type: "response.output_item.added", OutputIndex: 4, Item: &ResponsesOutput{Type: "reasoning", ID: "rs_one"}},
				{Type: "response.reasoning_summary_text.delta", OutputIndex: 4, Delta: "display only"},
				{Type: "response.output_text.delta", OutputIndex: 5, Delta: "answer"},
			}
			if mode == "late_delta" || mode == "late_done" {
				suffix := map[string]string{"late_delta": "delta", "late_done": "done"}[mode]
				events = append(events, ResponsesStreamEvent{Type: "response.reasoning_text." + suffix, OutputIndex: 4, ItemID: "rs_one", Delta: "real", Text: "real"})
			}
			if mode == "item_done" {
				events = append(events, ResponsesStreamEvent{Type: "response.output_item.done", OutputIndex: 4, Item: item})
			}
			events = append(events, ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{Status: "completed", Output: []ResponsesOutput{*item}}})
			var blocks []AnthropicStreamEvent
			open := make(map[int]bool)
			for index := range events {
				for _, block := range ResponsesEventToAnthropicEvents(&events[index], state) {
					blocks = append(blocks, block)
					if block.Index == nil {
						continue
					}
					switch block.Type {
					case "content_block_start":
						open[*block.Index] = true
					case "content_block_delta":
						require.True(t, open[*block.Index], "delta cannot target a closed block")
					case "content_block_stop":
						require.True(t, open[*block.Index])
						delete(open, *block.Index)
					}
				}
			}
			require.Empty(t, open)
			require.Equal(t, "realsecond", anthropicReasoningText(blocks))
		})
	}
}

func TestDeepSeekBackportLateThinkingDoesNotInterruptToolArguments(t *testing.T) {
	for _, mode := range []string{"delta", "done", "item_done", "terminal_only"} {
		for _, deltaArgs := range []bool{false, true} {
			t.Run(mode+map[bool]string{false: "/arguments_done", true: "/arguments_delta"}[deltaArgs], func(t *testing.T) {
				state := NewResponsesEventToAnthropicState()
				state.RequireReasoningContent = true
				reasoning := &ResponsesOutput{Type: "reasoning", ID: "rs_one", EncryptedContent: "opaque-signature", Content: []ResponsesContentPart{{Type: "reasoning_text", Text: "real"}}}
				tool := &ResponsesOutput{Type: "function_call", ID: "fc_one", CallID: "call_one", Name: "lookup", Arguments: `{"city":"Wuhan"}`}
				events := []ResponsesStreamEvent{
					{Type: "response.output_item.added", OutputIndex: 4, Item: &ResponsesOutput{Type: "reasoning", ID: "rs_one"}},
					{Type: "response.output_item.added", OutputIndex: 5, Item: tool},
				}
				switch mode {
				case "delta", "done":
					events = append(events, ResponsesStreamEvent{Type: "response.reasoning_text." + mode, OutputIndex: 4, ItemID: "rs_one", Delta: "real", Text: "real"})
				case "item_done":
					events = append(events, ResponsesStreamEvent{Type: "response.output_item.done", OutputIndex: 4, Item: reasoning})
				}
				if deltaArgs {
					events = append(events, ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 5, Delta: tool.Arguments})
				}
				events = append(events,
					ResponsesStreamEvent{Type: "response.function_call_arguments.done", OutputIndex: 5, Arguments: tool.Arguments},
					ResponsesStreamEvent{Type: "response.output_item.done", OutputIndex: 5, Item: tool},
					ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{Status: "completed", Output: []ResponsesOutput{*reasoning, *tool}}},
				)
				var blocks []AnthropicStreamEvent
				open := make(map[int]bool)
				var arguments string
				var signatures []string
				for index := range events {
					for _, block := range ResponsesEventToAnthropicEvents(&events[index], state) {
						blocks = append(blocks, block)
						if block.Index == nil {
							continue
						}
						switch block.Type {
						case "content_block_start":
							open[*block.Index] = true
						case "content_block_delta":
							require.True(t, open[*block.Index], "delta cannot target a closed block")
							if block.Delta.Type == "input_json_delta" {
								arguments += block.Delta.PartialJSON
							}
							if block.Delta.Type == "signature_delta" {
								signatures = append(signatures, block.Delta.Signature)
							}
						case "content_block_stop":
							require.True(t, open[*block.Index])
							delete(open, *block.Index)
						}
					}
				}
				require.Empty(t, open)
				require.JSONEq(t, tool.Arguments, arguments)
				require.Equal(t, "real", anthropicReasoningText(blocks))
				require.Equal(t, []string{"opaque-signature"}, signatures)
				require.Empty(t, state.pendingReasoning)
			})
		}
	}
}

func TestDeepSeekBackportDeferredThinkingDrainsOnTermination(t *testing.T) {
	for _, mode := range []string{"completed", "completed_terminal_only", "eof"} {
		t.Run(mode, func(t *testing.T) {
			state := NewResponsesEventToAnthropicState()
			state.RequireReasoningContent = true
			var blocks []AnthropicStreamEvent
			for _, event := range []ResponsesStreamEvent{
				{Type: "response.created"},
				{Type: "response.output_item.added", OutputIndex: 4, Item: &ResponsesOutput{Type: "reasoning", ID: "rs_one"}},
				{Type: "response.output_item.added", OutputIndex: 5, Item: &ResponsesOutput{Type: "function_call", CallID: "call_one", Name: "lookup"}},
				{Type: "response.function_call_arguments.delta", OutputIndex: 5, Delta: `{}`},
			} {
				blocks = append(blocks, ResponsesEventToAnthropicEvents(&event, state)...)
			}
			if mode != "completed_terminal_only" {
				blocks = append(blocks, ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.reasoning_text.done", OutputIndex: 4, ItemID: "rs_one", Text: "real"}, state)...)
				require.NotEmpty(t, state.pendingReasoning)
			}
			if mode == "eof" {
				blocks = append(blocks, FinalizeResponsesAnthropicStream(state)...)
			} else {
				blocks = append(blocks, ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{Status: "completed", Usage: &ResponsesUsage{InputTokens: 22, OutputTokens: 9}, Output: []ResponsesOutput{{Type: "reasoning", ID: "rs_one", EncryptedContent: "terminal-signature", Content: []ResponsesContentPart{{Type: "reasoning_text", Text: "real"}, {Type: "reasoning_text", Text: "second"}}}}}}, state)...)
				require.Equal(t, 22, state.InputTokens)
				require.Equal(t, 9, state.OutputTokens)
			}
			want := "real"
			if mode != "eof" {
				want += "second"
				var signatures []string
				for _, block := range blocks {
					if block.Delta != nil && block.Delta.Type == "signature_delta" {
						signatures = append(signatures, block.Delta.Signature)
					}
				}
				require.Equal(t, []string{"terminal-signature"}, signatures)
			}
			require.Equal(t, want, anthropicReasoningText(blocks))
			require.Equal(t, "message_stop", blocks[len(blocks)-1].Type)
			require.False(t, state.ContentBlockOpen)
			require.Empty(t, state.pendingReasoning)
			require.Empty(t, FinalizeResponsesAnthropicStream(state))
		})
	}
}

func TestResponsesAnthropicLegacyLateReasoningRecoveryTargetsOpenThinkingOnly(t *testing.T) {
	for _, kind := range []string{"summary_done", "item_done"} {
		for _, sibling := range []string{"text", "tool"} {
			t.Run(kind+"/"+sibling, func(t *testing.T) {
				state := NewResponsesEventToAnthropicState()
				events := []ResponsesStreamEvent{{Type: "response.output_item.added", OutputIndex: 0, Item: &ResponsesOutput{Type: "reasoning"}}}
				if sibling == "text" {
					events = append(events, ResponsesStreamEvent{Type: "response.output_text.delta", OutputIndex: 1, Delta: "answer"})
				} else {
					events = append(events, ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: 1, Item: &ResponsesOutput{Type: "function_call", Name: "lookup", CallID: "call_one"}})
				}
				late := ResponsesStreamEvent{Type: "response.reasoning_summary_text.done", OutputIndex: 0, Text: "late summary"}
				if kind == "item_done" {
					late = ResponsesStreamEvent{Type: "response.output_item.done", OutputIndex: 0, Item: &ResponsesOutput{Type: "reasoning", Summary: []ResponsesSummary{{Type: "summary_text", Text: "late summary"}}}}
				}
				events = append(events, late)
				open := make(map[int]string)
				var blocks []AnthropicStreamEvent
				for index := range events {
					for _, block := range ResponsesEventToAnthropicEvents(&events[index], state) {
						blocks = append(blocks, block)
						if block.Index == nil {
							continue
						}
						switch block.Type {
						case "content_block_start":
							open[*block.Index] = block.ContentBlock.Type
						case "content_block_delta":
							blockType, exists := open[*block.Index]
							require.True(t, exists, "default-mode recovery must not write to a closed block")
							if block.Delta.Type == "thinking_delta" {
								require.Equal(t, "thinking", blockType)
							}
						case "content_block_stop":
							delete(open, *block.Index)
						}
					}
				}
				require.Empty(t, anthropicReasoningText(blocks), "preserve legacy omission once the reasoning block has closed")
			})
		}
	}
}
