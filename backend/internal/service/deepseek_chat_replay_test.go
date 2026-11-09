//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDeepSeekChatReplayPairsToolsWithoutLosingVendorFields(t *testing.T) {
	body := []byte(`{"metadata":{"id":9007199254740993},"messages":[
		{"role":"tool","tool_call_id":"orphan","content":"old"},
		{"role":"assistant","reasoning_content":"real","vendor":{"id":9007199254740993},"tool_calls":[{"id":"a","type":"function","function":{"name":"f","arguments":"{}"},"extra":true},{"id":"b"},{"id":"missing"}]},
		{"role":"system","content":"notice","custom":true},
		{"role":"tool","tool_call_id":"b","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,abc"}}],"vendor":9007199254740993},
		{"role":"tool","tool_call_id":"a","content":"first"},
		{"role":"tool","tool_call_id":"a","content":"duplicate"},
		{"role":"tool","content":"no id"},
		{"role":"user","content":"continue"}
	]}`)
	out := prepareDeepSeekChatReplay(&Account{Platform: PlatformDeepseek}, "https://relay.example/chat/completions", body)
	messages := gjson.GetBytes(out, "messages").Array()
	require.Len(t, messages, 5)
	require.Equal(t, "real", messages[0].Get("reasoning_content").String())
	require.Equal(t, "9007199254740993", messages[0].Get("vendor.id").Raw)
	require.Equal(t, "9007199254740993", gjson.GetBytes(out, "metadata.id").Raw)
	require.Len(t, messages[0].Get("tool_calls").Array(), 2)
	require.True(t, messages[0].Get("tool_calls.0.extra").Bool())
	require.Equal(t, "a", messages[1].Get("tool_call_id").String())
	require.Equal(t, "first", messages[1].Get("content").String())
	require.Equal(t, "b", messages[2].Get("tool_call_id").String())
	require.Equal(t, "9007199254740993", messages[2].Get("vendor").Raw)
	require.Equal(t, "image_url", messages[2].Get("content.0.type").String())
	require.Equal(t, "system", messages[3].Get("role").String())
	require.Equal(t, "user", messages[4].Get("role").String())
	require.Equal(t, out, prepareDeepSeekChatReplay(&Account{Platform: PlatformDeepseek}, "https://relay.example/chat/completions", out))
}

func TestDeepSeekChatReplayScopesRepeatedCallIDsToTheirTurn(t *testing.T) {
	body := []byte(`{"messages":[
		{"role":"assistant","content":"old","tool_calls":[{"id":"a"}]},
		{"role":"tool","tool_call_id":"a","content":"old result"},
		{"role":"assistant","content":"new","tool_calls":[{"id":"a"}]},
		{"role":"user","content":"interleaved"},
		{"role":"tool","tool_call_id":"a","content":"new result"},
		{"role":"assistant","tool_calls":[{"id":"a"}]}
	]}`)
	out := normalizeDeepSeekChatMessages(body)
	messages := gjson.GetBytes(out, "messages").Array()
	require.Len(t, messages, 5, "an unanswered repeated ID must not reuse an older result")
	require.Equal(t, "old result", messages[1].Get("content").String())
	require.Equal(t, "new result", messages[3].Get("content").String())
	require.Equal(t, "interleaved", messages[4].Get("content").String())

	before := []byte(`{"messages":[{"role":"tool","tool_call_id":"a","content":"earlier"},{"role":"assistant","content":"keep","tool_calls":[{"id":"a"}]}]}`)
	after := gjson.GetBytes(normalizeDeepSeekChatMessages(before), "messages").Array()
	require.Len(t, after, 1)
	require.False(t, after[0].Get("tool_calls").Exists(), "a result before its announcement cannot answer it")
	require.Equal(t, "keep", after[0].Get("content").String())
}

func TestDeepSeekChatReplayGatingAndPlaceholders(t *testing.T) {
	body := []byte(`{"messages":[{"role":"assistant","content":"answer","vendor":9007199254740993},{"role":"assistant","reasoning_content":"real"},{"role":"assistant","reasoning_content":{"vendor":"opaque"}}]}`)
	for _, tc := range []struct {
		name, platform, endpoint string
		want                     bool
	}{
		{"native", PlatformDeepseek, "https://relay.example/chat/completions", true},
		{"mapped", PlatformOpenAI, "https://API.DEEPSEEK.COM/chat/completions", true},
		{"other-provider", PlatformOpenAI, "https://api.openai.com/v1/chat/completions", false},
		{"lookalike", PlatformOpenAI, "https://api.deepseek.com.example/chat/completions", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := prepareDeepSeekChatReplay(&Account{Platform: tc.platform}, tc.endpoint, body)
			if !tc.want {
				require.Equal(t, body, out)
				return
			}
			require.Equal(t, " ", gjson.GetBytes(out, "messages.0.reasoning_content").String())
			require.Equal(t, "real", gjson.GetBytes(out, "messages.1.reasoning_content").String())
			require.Equal(t, `{"vendor":"opaque"}`, gjson.GetBytes(out, "messages.2.reasoning_content").Raw)
			require.Equal(t, "9007199254740993", gjson.GetBytes(out, "messages.0.vendor").Raw)
		})
	}
	disabled := []byte(`{"thinking":{"type":"disabled"},"messages":[{"role":"assistant","content":"answer"}]}`)
	require.Equal(t, disabled, prepareDeepSeekChatReplay(&Account{Platform: PlatformDeepseek}, "https://api.deepseek.com", disabled))
	clean := []byte(`{ "messages": [{"role":"assistant", "reasoning_content":"real", "tool_calls":[{"id":"a"}]},{"role":"tool","tool_call_id":"a","content":"ok"}] }`)
	require.Equal(t, clean, prepareDeepSeekChatReplay(&Account{Platform: PlatformDeepseek}, "https://api.deepseek.com", clean))
}

func TestDeepSeekChatReplayRunsAtSharedOutboundBoundary(t *testing.T) {
	body := []byte(`{"model":"alias","messages":[{"role":"assistant","tool_calls":[{"id":"a"}]},{"role":"user","content":"notice"},{"role":"tool","tool_call_id":"a","content":"ok"}]}`)
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	c := newOpenCodeSessionTestContext(t, "")
	account := &Account{ID: 7, Platform: PlatformDeepseek, Type: AccountTypeAPIKey}
	response, err := svc.sendCCUpstreamRequest(context.Background(), c, account, "https://api.deepseek.com/chat/completions", body, false, "test-key", "", "")
	require.NoError(t, err)
	defer response.Body.Close()
	require.Len(t, upstream.requests, 1)
	require.Equal(t, " ", gjson.GetBytes(upstream.lastBody, "messages.0.reasoning_content").String())
	require.Equal(t, "tool", gjson.GetBytes(upstream.lastBody, "messages.1.role").String())
	require.Equal(t, "notice", gjson.GetBytes(upstream.lastBody, "messages.2.content").String())
}
