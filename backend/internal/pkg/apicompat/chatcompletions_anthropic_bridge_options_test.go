package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDirectAnthropicBridgeOptionsAndTokenFloorWire(t *testing.T) {
	var req AnthropicRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"deepseek-v4-pro","max_tokens":16,"cache_control":{"type":"ephemeral","ttl":"1h"},"messages":[{"role":"user","content":"question"},{"role":"assistant","content":[{"type":"thinking","thinking":"first"},{"type":"redacted_thinking","data":"hidden"},{"type":"thinking","signature":"only"},{"type":"thinking","thinking":"second"},{"type":"text","text":"answer"}]},{"role":"user","content":"next"}]}`), &req))
	for _, preserve := range []bool{false, true} {
		out, err := AnthropicToChatCompletionsRequestWithOptions(&req, 256, &AnthropicToChatCompletionsOptions{PreserveThinking: preserve})
		require.NoError(t, err)
		wire, err := json.Marshal(out)
		require.NoError(t, err)
		require.Equal(t, int64(256), gjson.GetBytes(wire, "max_completion_tokens").Int())
		require.False(t, gjson.GetBytes(wire, "cache_control").Exists(), "Anthropic top-level fields are not part of the generic Chat Completions protocol")
		if preserve {
			require.Equal(t, "first\nsecond", gjson.GetBytes(wire, "messages.1.reasoning_content").String())
		} else {
			require.False(t, gjson.GetBytes(wire, "messages.1.reasoning_content").Exists())
		}
	}
	out, err := AnthropicToChatCompletionsRequestWithMinOutputTokens(&req, 512)
	require.NoError(t, err)
	require.Equal(t, 512, *out.MaxCompletionTokens)
	require.Empty(t, out.Messages[1].ReasoningContent)
	_, err = AnthropicToChatCompletionsRequestWithOptions(nil, 256, nil)
	require.Error(t, err)
}

func TestDirectAnthropicBridgeThinkingOnlyAssistantReplay(t *testing.T) {
	req := &AnthropicRequest{Model: "deepseek-v4-pro", Messages: []AnthropicMessage{
		{Role: "assistant", Content: json.RawMessage(`[{"type":"thinking","thinking":"reasoning only"}]`)},
		{Role: "user", Content: json.RawMessage(`"continue"`)},
	}}
	out, err := AnthropicToChatCompletionsRequestWithOptions(req, 256, &AnthropicToChatCompletionsOptions{PreserveThinking: true})
	require.NoError(t, err)
	require.Len(t, out.Messages, 2)
	wire, err := json.Marshal(out)
	require.NoError(t, err)
	require.Equal(t, "reasoning only", gjson.GetBytes(wire, "messages.0.reasoning_content").String())
	generic, err := AnthropicToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Empty(t, generic.Messages[0].ReasoningContent)
}
