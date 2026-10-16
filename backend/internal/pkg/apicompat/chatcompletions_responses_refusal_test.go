package apicompat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChatResponsesStreamRefusalLifecycle(t *testing.T) {
	for _, tt := range []struct {
		name   string
		chunks []string
		types  []string
	}{
		{
			name: "refusal only",
			chunks: []string{
				`{"choices":[{"delta":{"role":"assistant","refusal":""}}]}`,
				`{"choices":[{"delta":{"refusal":"I cannot "}}]}`,
				`{"choices":[{"delta":{"refusal":"help."},"finish_reason":"stop"}]}`,
			},
			types: []string{"refusal"},
		},
		{
			name: "text then refusal",
			chunks: []string{
				`{"choices":[{"delta":{"content":"answer"}}]}`,
				`{"choices":[{"delta":{"refusal":"I cannot help."},"finish_reason":"stop"}]}`,
			},
			types: []string{"output_text", "refusal"},
		},
		{
			name: "refusal then text",
			chunks: []string{
				`{"choices":[{"delta":{"refusal":"I cannot help."}}]}`,
				`{"choices":[{"delta":{"content":"answer"},"finish_reason":"stop"}]}`,
			},
			types: []string{"refusal", "output_text"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			events := collectStreamEvents(t, tt.chunks)
			parts := map[int]string{}
			var refusalDeltas string
			var refusalDone, completed bool
			var messageID string
			for i, event := range events {
				require.Equal(t, i, event.SequenceNumber)
				wire := marshalEvent(t, event)
				switch event.Type {
				case "response.output_item.added":
					messageID = event.Item.ID
				case "response.content_part.added":
					require.Equal(t, messageID, event.ItemID)
					require.NotContains(t, parts, event.ContentIndex)
					parts[event.ContentIndex] = event.Part.Type
					if event.Part.Type == "refusal" {
						require.Equal(t, map[string]any{"type": "refusal", "refusal": ""}, wire["part"])
					}
				case "response.refusal.delta", "response.refusal.done":
					require.Equal(t, "refusal", parts[event.ContentIndex])
					require.Equal(t, messageID, event.ItemID)
					require.EqualValues(t, 0, wire["output_index"])
					require.EqualValues(t, event.ContentIndex, wire["content_index"])
					if event.Type == "response.refusal.delta" {
						refusalDeltas += event.Delta
					} else {
						require.False(t, refusalDone)
						refusalDone = true
						require.Equal(t, "I cannot help.", wire["refusal"])
					}
				case "response.content_part.done":
					if event.Part.Type == "refusal" {
						require.True(t, refusalDone)
						require.Equal(t, map[string]any{"type": "refusal", "refusal": "I cannot help."}, wire["part"])
					}
				case "response.output_item.done", "response.completed":
					var content []ResponsesContentPart
					if event.Item != nil {
						content = event.Item.Content
						itemWire, ok := wire["item"].(map[string]any)
						require.True(t, ok)
						contentWire, ok := itemWire["content"].([]any)
						require.True(t, ok)
						for j, part := range content {
							if part.Type == "refusal" {
								require.Equal(t, map[string]any{"type": "refusal", "refusal": "I cannot help."}, contentWire[j])
							}
						}
					} else {
						completed = true
						require.Equal(t, "completed", event.Response.Status)
						require.Len(t, event.Response.Output, 1)
						content = event.Response.Output[0].Content
					}
					require.Len(t, content, len(tt.types))
					for j, part := range content {
						require.Equal(t, tt.types[j], part.Type)
						if part.Type == "refusal" {
							require.Equal(t, "I cannot help.", part.Refusal)
						} else {
							require.Equal(t, "answer", part.Text)
						}
					}
				}
			}
			require.True(t, completed)
			require.True(t, refusalDone)
			require.Equal(t, "I cannot help.", refusalDeltas)
		})
	}
}
