//go:build unit

package service

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDeepSeekNativeReasoningPlaceholdersPreserveVendorContent(t *testing.T) {
	for _, content := range []string{
		`{"vendor":"opaque"}`, `""`, `false`,
		`[{"type":"vendor","text":""}]`,
		`[{"type":"reasoning_text","text":""},{"type":"vendor","opaque":9007199254740993}]`,
		`[{"type":"reasoning_text","text":"real"}]`,
		`[{"type":"reasoning_text","text":" "}]`,
		`[{"type":"reasoning_text","text":{"vendor":"opaque"}}]`,
	} {
		t.Run(content, func(t *testing.T) {
			body := []byte(`{"input":[{"type":"reasoning","content":` + content + `}]}`)
			require.Equal(t, body, ensureDeepSeekNativeReasoningPlaceholders(deepSeekNativeReplayAccount(), body))
		})
	}
}

func TestDeepSeekNativeReasoningPlaceholdersOnlyFillEmptySlots(t *testing.T) {
	for _, fields := range []string{"", `,"content":null`, `,"content":[]`, `,"content":[{"type":"reasoning_text","text":"","vendor":9007199254740993}]`} {
		t.Run(fields, func(t *testing.T) {
			body := []byte(`{"model":"deepseek-flash","input":[{"type":"reasoning","summary":[{"type":"summary_text","text":"display only"}],"encrypted_content":"opaque"` + fields + `},{"type":"function_call","call_id":"call_a"}]}`)
			// Even without a cache or authenticated conversation, the placeholder
			// contains no recovered text and needs no shared-cache authorization.
			var svc *OpenAIGatewayService
			out := svc.prepareDeepSeekNativeReasoningReplay(nil, deepSeekNativeReplayAccount(), body)
			require.Equal(t, " ", gjson.GetBytes(out, "input.0.content.0.text").String())
			require.Equal(t, "display only", gjson.GetBytes(out, "input.0.summary.0.text").String())
			require.Equal(t, "opaque", gjson.GetBytes(out, "input.0.encrypted_content").String())
			require.Equal(t, `{"type":"function_call","call_id":"call_a"}`, gjson.GetBytes(out, "input.1").Raw)
			if fields != "" && gjson.GetBytes(body, "input.0.content.0.vendor").Exists() {
				require.Equal(t, "9007199254740993", gjson.GetBytes(out, "input.0.content.0.vendor").Raw)
			}
			require.Equal(t, out, svc.prepareDeepSeekNativeReasoningReplay(nil, deepSeekNativeReplayAccount(), out))
		})
	}
}

func TestDeepSeekNativeReasoningReplayRestoresExplicitlyEmptyTextFirst(t *testing.T) {
	account := deepSeekNativeReplayAccount()
	cache := &reasoningRecordingCache{}
	svc := &OpenAIGatewayService{cache: cache}
	c, _ := deepSeekNativeReplayContext(21, "empty-text")
	svc.prepareDeepSeekNativeReasoningReplay(c, account, []byte(`{"model":"deepseek-flash","input":"q"}`))
	svc.cacheDeepSeekNativeReasoningBody(c, account, []byte(`{"status":"completed","output":[{"type":"reasoning","id":"rs_1","encrypted_content":"opaque","content":[{"type":"reasoning_text","text":"real reasoning"}]}]}`))
	svc.cache = &reasoningRecordingCache{getResp: cache.snapshotSets()}
	for _, content := range []string{`null`, `[]`, `[{"type":"reasoning_text","text":""}]`} {
		body := []byte(fmt.Sprintf(`{"model":"deepseek-flash","input":[{"type":"reasoning","id":"rs_1","encrypted_content":"opaque","content":%s}]}`, content))
		out := svc.prepareDeepSeekNativeReasoningReplay(c, account, body)
		require.Equal(t, "real reasoning", gjson.GetBytes(out, "input.0.content.0.text").String())
	}
}
