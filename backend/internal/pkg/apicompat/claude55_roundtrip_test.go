package apicompat

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOpus55ResponsesAdaptiveThinkingAndToolChoice(t *testing.T) {
	for _, effort := range []string{"", "low", "medium", "high", "xhigh", "max"} {
		req := &ResponsesRequest{Model: "claude-opus-5-5", Input: json.RawMessage(`"hello"`), Reasoning: &ResponsesReasoning{Effort: effort}}
		out, err := ResponsesToAnthropicRequest(req)
		require.NoError(t, err)
		require.Equal(t, "adaptive", out.Thinking.Type)
		require.Zero(t, out.Thinking.BudgetTokens)
		if effort == "" {
			effort = "medium"
		}
		require.Equal(t, effort, out.OutputConfig.Effort)
	}
	for _, choice := range []string{`"required"`, `{"type":"function","name":"lookup"}`} {
		_, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-opus-5-5", Input: json.RawMessage(`"hello"`), ToolChoice: json.RawMessage(choice)})
		require.ErrorContains(t, err, "forced tool_choice")
	}
	_, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-opus-5-5", Input: json.RawMessage(`"hello"`), Reasoning: &ResponsesReasoning{Effort: "none"}})
	require.ErrorContains(t, err, "reasoning effort")
	old, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-opus-5", Input: json.RawMessage(`"hello"`), Reasoning: &ResponsesReasoning{Effort: "xhigh"}})
	require.NoError(t, err)
	require.Equal(t, "max", old.OutputConfig.Effort)
	require.Equal(t, "enabled", old.Thinking.Type)
}

func TestOpus55SignedThinkingResponsesRoundTrip(t *testing.T) {
	block := AnthropicContentBlock{Type: "thinking", Thinking: "", Signature: "upstream-signed-block"}
	response := AnthropicToResponsesResponse(&AnthropicResponse{Model: "claude-opus-5-5", Content: []AnthropicContentBlock{block, {Type: "tool_use", ID: "toolu_1", Name: "lookup", Input: json.RawMessage(`{}`)}}})
	require.Len(t, response.Output, 2)
	require.NotEmpty(t, response.Output[0].EncryptedContent)
	raw, err := json.Marshal(response.Output)
	require.NoError(t, err)
	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(raw, &items))
	items = append(items, ResponsesInputItem{Type: "function_call_output", CallID: response.Output[1].CallID, Output: "ok"})
	raw, err = json.Marshal(items)
	require.NoError(t, err)
	converted, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-opus-5-5", Input: raw})
	require.NoError(t, err)
	require.Len(t, converted.Messages, 2)
	var blocks []AnthropicContentBlock
	require.NoError(t, json.Unmarshal(converted.Messages[0].Content, &blocks))
	require.Equal(t, block, blocks[0])
	require.Equal(t, "tool_use", blocks[1].Type)
	// Arbitrary OpenAI ciphertext must never be treated as an Anthropic signature.
	_, _, err = convertResponsesInputToAnthropic("", json.RawMessage(`[{"type":"reasoning","encrypted_content":"anthropic-thinking-v1:!"}]`), true)
	require.Error(t, err)
}

func TestSonnet55ResponsesThinkingAndSampling(t *testing.T) {
	for _, effort := range []string{"", "low", "medium", "high", "xhigh", "max", "none"} {
		req := &ResponsesRequest{Model: "claude-sonnet-5-5", Input: json.RawMessage(`"hello"`), Reasoning: &ResponsesReasoning{Effort: effort}}
		out, err := ResponsesToAnthropicRequest(req)
		require.NoError(t, err, effort)
		require.Zero(t, out.Thinking.BudgetTokens)
		if effort == "none" {
			require.Equal(t, "between_tools", out.Thinking.Type)
			require.Equal(t, "low", out.OutputConfig.Effort)
		} else {
			require.Equal(t, "adaptive", out.Thinking.Type)
			if effort == "" {
				effort = "high"
			}
			require.Equal(t, effort, out.OutputConfig.Effort)
		}
	}
	for _, choice := range []string{`"required"`, `{"type":"function","name":"lookup"}`} {
		_, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-sonnet-5-5", Input: json.RawMessage(`"hello"`), ToolChoice: json.RawMessage(choice)})
		require.ErrorContains(t, err, "forced tool_choice")
	}
	temperature := 0.7
	_, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-sonnet-5-5", Input: json.RawMessage(`"hello"`), Temperature: &temperature})
	require.ErrorContains(t, err, "temperature")
	topP := 0.5
	_, err = ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-sonnet-5-5", Input: json.RawMessage(`"hello"`), TopP: &topP})
	require.ErrorContains(t, err, "top_p")
	_, err = ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-sonnet-5-5", Input: json.RawMessage(`"hello"`), Reasoning: &ResponsesReasoning{Effort: "minimal"}})
	require.ErrorContains(t, err, "reasoning effort")

	temperature, topP = 1, 0.99
	_, err = ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-sonnet-5-5", Input: json.RawMessage(`"hello"`), Temperature: &temperature, TopP: &topP})
	require.NoError(t, err)
}

func TestSonnet55SignedThinkingResponsesRoundTrip(t *testing.T) {
	block := AnthropicContentBlock{Type: "thinking", Thinking: "", Signature: "signed-sonnet-block"}
	response := AnthropicToResponsesResponse(&AnthropicResponse{Model: "claude-sonnet-5-5", Content: []AnthropicContentBlock{block, {Type: "text", Text: "progress"}, {Type: "tool_use", ID: "toolu_1", Name: "lookup", Input: json.RawMessage(`{}`)}}})
	require.Len(t, response.Output, 3)
	require.NotEmpty(t, response.Output[0].EncryptedContent)
	require.Equal(t, "message", response.Output[1].Type)
	raw, err := json.Marshal(response.Output)
	require.NoError(t, err)
	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(raw, &items))
	items = append(items, ResponsesInputItem{Type: "function_call_output", CallID: response.Output[2].CallID, Output: "ok"})
	raw, err = json.Marshal(items)
	require.NoError(t, err)
	converted, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-sonnet-5-5", Input: raw})
	require.NoError(t, err)
	require.Len(t, converted.Messages, 2)
	var blocks []AnthropicContentBlock
	require.NoError(t, json.Unmarshal(converted.Messages[0].Content, &blocks))
	require.Equal(t, block, blocks[0])
	require.Equal(t, "text", blocks[1].Type)
	require.Equal(t, "tool_use", blocks[2].Type)
}

func TestHaiku55ResponsesThinkingAndSampling(t *testing.T) {
	for _, effort := range []string{"", "low", "medium", "high", "xhigh", "max", "none"} {
		req := &ResponsesRequest{Model: "claude-haiku-5-5", Input: json.RawMessage(`"hello"`), Reasoning: &ResponsesReasoning{Effort: effort}}
		out, err := ResponsesToAnthropicRequest(req)
		require.NoError(t, err, effort)
		require.Zero(t, out.Thinking.BudgetTokens)
		if effort == "none" {
			require.Equal(t, "disabled", out.Thinking.Type)
			require.Equal(t, "low", out.OutputConfig.Effort)
		} else {
			require.Equal(t, "adaptive", out.Thinking.Type)
			if effort == "" {
				effort = "medium"
			}
			require.Equal(t, effort, out.OutputConfig.Effort)
		}
	}
	// Haiku 5.5 accepts forced tool_choice, unlike opus/sonnet 5.5.
	for _, choice := range []string{`"required"`, `{"type":"function","name":"lookup"}`} {
		out, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-haiku-5-5", Input: json.RawMessage(`"hello"`), ToolChoice: json.RawMessage(choice)})
		require.NoError(t, err, choice)
		require.NotEmpty(t, out.ToolChoice)
	}
	temperature := 0.7
	_, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-haiku-5-5", Input: json.RawMessage(`"hello"`), Temperature: &temperature})
	require.ErrorContains(t, err, "temperature")
	for _, topP := range []float64{0.5, 0.995, 1} {
		_, err = ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-haiku-5-5", Input: json.RawMessage(`"hello"`), TopP: &topP})
		require.ErrorContains(t, err, "top_p", "top_p=%v", topP)
	}
	_, err = ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-haiku-5-5", Input: json.RawMessage(`"hello"`), Reasoning: &ResponsesReasoning{Effort: "minimal"}})
	require.ErrorContains(t, err, "reasoning effort")

	temperature = 1
	topP := 0.99
	_, err = ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-haiku-5-5", Input: json.RawMessage(`"hello"`), Temperature: &temperature})
	require.NoError(t, err)
	_, err = ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-haiku-5-5", Input: json.RawMessage(`"hello"`), TopP: &topP})
	require.NoError(t, err)
	_, err = ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-haiku-5-5", Input: json.RawMessage(`"hello"`), Temperature: &temperature, TopP: &topP})
	require.ErrorContains(t, err, "together")
}

func TestHaiku55SignedThinkingResponsesRoundTrip(t *testing.T) {
	block := AnthropicContentBlock{Type: "thinking", Thinking: "", Signature: "signed-haiku-block"}
	response := AnthropicToResponsesResponse(&AnthropicResponse{Model: "claude-haiku-5-5", Content: []AnthropicContentBlock{block, {Type: "text", Text: "progress"}, {Type: "tool_use", ID: "toolu_1", Name: "lookup", Input: json.RawMessage(`{}`)}}})
	require.Len(t, response.Output, 3)
	require.NotEmpty(t, response.Output[0].EncryptedContent)
	require.Equal(t, "message", response.Output[1].Type)
	raw, err := json.Marshal(response.Output)
	require.NoError(t, err)
	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(raw, &items))
	items = append(items, ResponsesInputItem{Type: "function_call_output", CallID: response.Output[2].CallID, Output: "ok"})
	raw, err = json.Marshal(items)
	require.NoError(t, err)
	converted, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-haiku-5-5", Input: raw})
	require.NoError(t, err)
	require.Len(t, converted.Messages, 2)
	var blocks []AnthropicContentBlock
	require.NoError(t, json.Unmarshal(converted.Messages[0].Content, &blocks))
	require.Equal(t, block, blocks[0])
	require.Equal(t, "text", blocks[1].Type)
	require.Equal(t, "tool_use", blocks[2].Type)
}

func TestClaude55SignedThinkingStreamRoundTripWithPublicAlias(t *testing.T) {
	for _, block := range []AnthropicContentBlock{
		{Type: "thinking", Thinking: "", Signature: ""},
		{Type: "redacted_thinking", Data: "opaque-redacted-payload"},
	} {
		t.Run(block.Type, func(t *testing.T) {
			state := NewAnthropicEventToResponsesState()
			state.Model = "public-alias"
			state.PreserveThinkingSignatures = true // effective upstream model is Haiku 5.5
			idx := 0
			events := []*AnthropicStreamEvent{
				{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1", Model: "public-alias"}},
				{Type: "content_block_start", Index: &idx, ContentBlock: &block},
			}
			if block.Type == "thinking" {
				events = append(events,
					&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "thinking_delta", Thinking: "reason"}},
					&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "signature_delta", Signature: "signed-"}},
					&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "signature_delta", Signature: "payload"}},
				)
			}
			events = append(events, &AnthropicStreamEvent{Type: "content_block_stop", Index: &idx},
				&AnthropicStreamEvent{Type: "message_delta", Delta: &AnthropicDelta{StopReason: "end_turn"}},
				&AnthropicStreamEvent{Type: "message_stop"})
			var completed *ResponsesResponse
			var doneItem *ResponsesOutput
			for _, event := range events {
				for _, out := range AnthropicEventToResponsesEvents(event, state) {
					if out.Type == "response.output_item.done" {
						doneItem = out.Item
					}
					if out.Type == "response.completed" {
						completed = out.Response
					}
				}
			}
			require.NotNil(t, completed)
			require.NotNil(t, doneItem)
			require.Len(t, completed.Output, 1)
			require.Equal(t, doneItem.EncryptedContent, completed.Output[0].EncryptedContent)
			raw, err := json.Marshal(completed.Output)
			require.NoError(t, err)
			converted, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-haiku-5-5", Input: raw})
			require.NoError(t, err)
			require.Len(t, converted.Messages, 1)
			var blocks []AnthropicContentBlock
			require.NoError(t, json.Unmarshal(converted.Messages[0].Content, &blocks))
			require.Len(t, blocks, 1)
			expected := block
			if block.Type == "thinking" {
				expected.Thinking = "reason"
				expected.Signature = "signed-payload"
			}
			require.Equal(t, expected, blocks[0])
		})
	}
}

func TestClaude55BufferedSignedThinkingUsesEffectiveModel(t *testing.T) {
	resp := &AnthropicResponse{Model: "public-alias", Content: []AnthropicContentBlock{
		{Type: "thinking", Signature: "signature"}, {Type: "text", Text: "hello"},
	}}
	converted := AnthropicToResponsesResponse(resp, "claude-haiku-5-5")
	require.Equal(t, "public-alias", converted.Model)
	require.Len(t, converted.Output, 2)
	require.Contains(t, converted.Output[0].EncryptedContent, anthropicThinkingEnvelopePrefix)
	require.Equal(t, "public-alias", resp.Model, "conversion must not mutate the upstream response")
}
