//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestFilterThinkingBlocks_MixedSignedHistory(t *testing.T) {
	for _, model := range []string{"claude-opus-5", "claude-opus-5-5"} {
		for _, setting := range []string{"", `,"thinking":{"type":"disabled"}`, `,"thinking":{"type":"adaptive"}`} {
			t.Run(model+setting, func(t *testing.T) {
				body := []byte(`{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"signed history","signature":"historical-signature"},{"type":"thinking","thinking":"unsigned history"},{"type":"text","text":"answer"}]}]` + setting + `}`)
				out := FilterThinkingBlocks(body, model)
				require.True(t, gjson.ValidBytes(out))
				content := gjson.GetBytes(out, "messages.0.content").Array()
				require.Len(t, content, 2)
				require.Equal(t, "historical-signature", content[0].Get("signature").String())
				require.Equal(t, "answer", content[1].Get("text").String())
			})
		}
	}
}

func TestFilterThinkingBlocks_Claude55RedactedAndEmptyHistory(t *testing.T) {
	body := []byte(`{"messages":[{"role":"assistant","content":[{"type":"redacted_thinking","data":"opaque-history"},{"type":"thinking","thinking":"unsigned"}]},{"role":"assistant","content":[{"type":"thinking","thinking":"unsigned"}]}]}`)
	out := FilterThinkingBlocks(body, "claude-opus-5-5")
	require.True(t, gjson.ValidBytes(out))
	require.Equal(t, "opaque-history", gjson.GetBytes(out, "messages.0.content.0.data").String())
	require.Equal(t, int64(1), gjson.GetBytes(out, "messages.0.content.#").Int())
	require.Equal(t, "(assistant content removed)", gjson.GetBytes(out, "messages.1.content.0.text").String())
}
