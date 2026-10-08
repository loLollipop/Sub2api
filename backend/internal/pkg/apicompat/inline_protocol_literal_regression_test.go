package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Text examples are not a tool-call transport. Keep this regression while
// the unscoped upstream DSML converter is deferred.
func TestChatBridgePreservesLiteralProtocolExamples(t *testing.T) {
	for _, text := range []string{
		`<details><summary>标题</summary>正文</details>`,
		`<analysis>literal analysis tag</analysis><summary>literal summary</summary>`,
		"```xml\n<｜DSML｜calls><｜DSML｜invoke name=\"local_shell\"><｜DSML｜parameter name=\"command\" string=\"false\">[\"echo\",\"example\"]</｜DSML｜parameter></｜DSML｜invoke></｜DSML｜calls>\n```",
		`<｜DSML｜calls><｜DSML｜invoke name="local_shell">`,
	} {
		t.Run(text, func(t *testing.T) {
			content, err := json.Marshal(text)
			require.NoError(t, err)
			outputs := chatMessageToResponsesOutput(ChatMessage{Role: "assistant", Content: content}, nil, nil, false, nil, map[string]bool{"local_shell": true})
			require.Len(t, outputs, 1)
			require.Equal(t, "message", outputs[0].Type)
			require.Equal(t, text, outputs[0].Content[0].Text)

			responses := NewChatCompletionsToResponsesStreamState("test")
			anthropic := NewChatCompletionsToAnthropicStreamState("test")
			var resEvents []ResponsesStreamEvent
			var anthEvents []AnthropicStreamEvent
			for _, part := range strings.Split(text, "") {
				delta, err := json.Marshal(part)
				require.NoError(t, err)
				var chunk ChatCompletionsChunk
				require.NoError(t, json.Unmarshal([]byte(`{"choices":[{"index":0,"delta":{"content":`+string(delta)+`}}]}`), &chunk))
				resEvents = append(resEvents, ChatCompletionsChunkToResponsesEvents(&chunk, responses)...)
				anthEvents = append(anthEvents, ChatCompletionsChunkToAnthropicEvents(&chunk, anthropic)...)
			}
			resEvents = append(resEvents, FinalizeChatCompletionsResponsesStream(responses)...)
			anthEvents = append(anthEvents, FinalizeChatCompletionsAnthropicStream(anthropic)...)
			var resText, anthText strings.Builder
			for _, event := range resEvents {
				if event.Item != nil {
					require.Equal(t, "message", event.Item.Type, "must not synthesize reasoning or tools from examples")
				}
				if event.Type == "response.output_text.delta" {
					_, _ = resText.WriteString(event.Delta)
				}
			}
			for _, event := range anthEvents {
				if event.ContentBlock != nil {
					require.Equal(t, "text", event.ContentBlock.Type)
				}
				if event.Delta != nil && event.Delta.Type == "text_delta" {
					_, _ = anthText.WriteString(event.Delta.Text)
				}
			}
			require.Equal(t, text, resText.String())
			require.Equal(t, text, anthText.String())
		})
	}
}
