//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDeepSeekThinkingReplayPlaceholders(t *testing.T) {
	for _, tc := range []struct {
		name, model, preference, content string
		changed                          bool
	}{
		{"missing", "deepseek-flash", "", `[{"type":"tool_use","id":"a","name":"lookup","input":{}}]`, true},
		{"empty", "deepseek-flash", "", `[{"type":"thinking","thinking":"","signature":"keep"},{"type":"tool_use","id":"a"}]`, true},
		{"missing text", "deepseek-flash", "", `[{"type":"thinking","signature":"keep"},{"type":"tool_use","id":"a"}]`, true},
		{"null", "deepseek-flash", "", `[{"type":"thinking","thinking":null},{"type":"tool_use","id":"a"}]`, true},
		{"real", "deepseek-flash", "", `[{"type":"thinking","thinking":"real"},{"type":"tool_use","id":"a"}]`, false},
		{"real plus empty", "deepseek-flash", "", `[{"type":"thinking","thinking":""},{"type":"thinking","thinking":"real"},{"type":"tool_use","id":"a"}]`, false},
		{"redacted", "deepseek-flash", "", `[{"type":"redacted_thinking","data":"opaque"},{"type":"tool_use","id":"a"}]`, false},
		{"redacted plus empty", "deepseek-flash", "", `[{"type":"thinking","thinking":""},{"type":"redacted_thinking","data":"opaque"},{"type":"tool_use","id":"a"}]`, false},
		{"vendor thinking", "deepseek-flash", "", `[{"type":"thinking","thinking":{"vendor":"opaque"}},{"type":"tool_use","id":"a"}]`, false},
		{"server tool", "deepseek-flash", "", `[{"type":"server_tool_use","id":"a"}]`, false},
		{"text", "deepseek-flash", "", `[{"type":"text","text":"hello"}]`, false},
		{"disabled", "deepseek-flash", `"thinking":{"type":"disabled"},`, `[{"type":"tool_use","id":"a"}]`, false},
		{"other vendor", "glm-5.3", "", `[{"type":"tool_use","id":"a"}]`, false},
		{"other deepseek name", "deepseekish", "", `[{"type":"tool_use","id":"a"}]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{%s"messages":[{"role":"assistant","content":%s},{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":"42"}]}],"vendor":9007199254740993}`, tc.preference, tc.content))
			out, changed := ensureDeepSeekThinkingReplayPlaceholders(body, tc.model)
			require.Equal(t, tc.changed, changed)
			if !changed {
				require.Equal(t, body, out)
				return
			}
			require.Equal(t, " ", gjson.GetBytes(out, "messages.0.content.0.thinking").String())
			require.Equal(t, "a", gjson.GetBytes(out, "messages.0.content.1.id").String())
			require.Equal(t, "9007199254740993", gjson.GetBytes(out, "vendor").Raw)
			require.Equal(t, gjson.GetBytes(body, "messages.1").Raw, gjson.GetBytes(out, "messages.1").Raw)
			if tc.name == "empty" || tc.name == "missing text" {
				require.Equal(t, "keep", gjson.GetBytes(out, "messages.0.content.0.signature").String())
			}
			again, changed := ensureDeepSeekThinkingReplayPlaceholders(out, tc.model)
			require.False(t, changed)
			require.Equal(t, out, again)
		})
	}
}

func TestDeepSeekThinkingReplayGatewayForwardMappedModel(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, model := range []string{"deepseek-flash", "claude-sonnet-4-5"} {
			t.Run(fmt.Sprintf("passthrough=%t/%s", passthrough, model), func(t *testing.T) {
				body := []byte(`{"model":"public-alias","max_tokens":32,"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"a","name":"lookup","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":"42"}]}]}`)
				account := newAnthropicAPIKeyAccountForTest()
				account.Credentials["model_mapping"] = map[string]any{"public-alias": model}
				account.Extra["anthropic_passthrough"] = passthrough
				upstream := &anthropicHTTPUpstreamRecorder{resp: nativeAnthropicBufferedResponse()}
				svc := &GatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream, tlsFPProfileService: &TLSFingerprintProfileService{}, rateLimitService: &RateLimitService{}}
				c := adaptiveProtocolTestContext("/v1/messages", body)
				result, err := svc.Forward(context.Background(), c, account, &ParsedRequest{Body: NewRequestBodyRef(body), Model: "public-alias"})
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, model, gjson.GetBytes(upstream.lastBody, "model").String())
				if model == "deepseek-flash" {
					require.Equal(t, " ", gjson.GetBytes(upstream.lastBody, "messages.0.content.0.thinking").String())
					require.Equal(t, "tool_use", gjson.GetBytes(upstream.lastBody, "messages.0.content.1.type").String())
				} else {
					require.Equal(t, gjson.GetBytes(body, "messages").Raw, gjson.GetBytes(upstream.lastBody, "messages").Raw)
				}
			})
		}
	}
}
