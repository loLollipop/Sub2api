package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// An operator reading a block wants the text that triggered it, parsed out of
// the request body — not a summary and not every earlier turn.
func TestExtractContentModerationLastUserTurn_ParsesOnlyLatestTurn(t *testing.T) {
	body := []byte(`{"messages":[
		{"role":"system","content":"system prompt"},
		{"role":"user","content":"第一轮：请帮我写个脚本"},
		{"role":"assistant","content":"好的"},
		{"role":"user","content":[{"type":"text","text":"第二轮：这段内容应该被审计看到"}]}
	]}`)
	got := ExtractContentModerationLastUserTurn(ContentModerationProtocolOpenAIChat, body)
	require.Equal(t, "第二轮：这段内容应该被审计看到", got)
	require.NotContains(t, got, "第一轮")
	require.NotContains(t, got, "system prompt")
}

// The log excerpt must not be cut at the moderation-input ceiling: a long last
// turn has to come back whole so the operator can read it.
func TestExtractContentModerationLastUserTurn_KeepsLongTurn(t *testing.T) {
	long := strings.Repeat("长", maxModerationInputRunes+500)
	body := []byte(`{"messages":[{"role":"user","content":"` + long + `"}]}`)
	got := ExtractContentModerationLastUserTurn(ContentModerationProtocolOpenAIChat, body)
	require.Equal(t, len([]rune(long)), len([]rune(got)))
}

// Responses-style payloads resolve the same way.
func TestExtractContentModerationLastUserTurn_Responses(t *testing.T) {
	body := []byte(`{"input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"old turn"}]},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"reply"}]},
		{"type":"message","role":"user","content":[{"type":"input_text","text":"latest turn"}]}
	]}`)
	require.Equal(t, "latest turn", ExtractContentModerationLastUserTurn(ContentModerationProtocolOpenAIResponses, body))
}

func TestExtractContentModerationLastUserTurn_ProtocolsAndImagePrivacy(t *testing.T) {
	for _, tc := range []struct{ name, protocol, body, want string }{
		{"anthropic", ContentModerationProtocolAnthropicMessages, `{"messages":[{"role":"user","content":"old"},{"role":"assistant","content":"reply"},{"role":"user","content":[{"type":"text","text":"latest\nline"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"c2VjcmV0"}}]}]}`, "latest\nline"},
		{"gemini", ContentModerationProtocolGemini, `{"contents":[{"role":"user","parts":[{"text":"old"}]},{"role":"model","parts":[{"text":"reply"}]},{"role":"user","parts":[{"text":"latest\nline"},{"inlineData":{"mimeType":"image/png","data":"c2VjcmV0"}}]}]}`, "latest\nline"},
		{"image-only", ContentModerationProtocolOpenAIChat, `{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://private.example/secret"}}]}]}`, "[images: 1]"},
		{"assistant-final", ContentModerationProtocolOpenAIChat, `{"messages":[{"role":"user","content":"old"},{"role":"assistant","content":"reply"}]}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractContentModerationLastUserTurn(tc.protocol, []byte(tc.body))
			require.Equal(t, tc.want, got)
			require.NotContains(t, got, "c2VjcmV0")
			require.NotContains(t, got, "private.example")
		})
	}
}
