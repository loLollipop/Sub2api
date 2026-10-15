//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDeepSeekDedupeForwardRetainsArgumentsAndIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"deepseek-v4","stream":false,"input":[
		{"type":"function_call","id":"fc_a","call_id":"call_a","name":"read","arguments":"{}"},
		{"type":"function_call_output","call_id":"call_a","output":"{}"},
		{"type":"function_call","id":"duplicate_a","call_id":"call_a","name":"read","arguments":"{\"line\":9007199254740993}"},
		{"type":"function_call_output","call_id":"call_a","output":"discarded"},
		{"type":"function_call","id":"fc_b","call_id":"call_b","name":"read","arguments":"{\"line\":9007199254740993}"},
		{"type":"function_call_output","call_id":"call_b","output":"other"}
	]}`)
	for _, mode := range []string{"responses", "passthrough", "chat-fallback"} {
		t.Run(mode, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			responseBody := `{"id":"resp_test","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`
			account := &Account{ID: 7716, Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Credentials: map[string]any{
				"api_key": "test-key", "api_protocol": APIProtocolResponses, "base_url": "https://relay.example",
			}}
			if mode == "chat-fallback" {
				account = forceChatResponsesFallbackAccount()
				account.Credentials["base_url"] = "https://relay.example"
				responseBody = `{"id":"chat_test","object":"chat.completion","model":"deepseek-v4","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(responseBody)),
			}}
			svc := openAIClientToolsTestService(upstream)
			var result *OpenAIForwardResult
			var err error
			if mode == "passthrough" {
				account.Platform = PlatformOpenAI
				result, err = svc.forwardOpenAIPassthrough(context.Background(), c, account, body, body, "deepseek-v4", false, nil, false, time.Now())
			} else {
				result, err = svc.Forward(context.Background(), c, account, body)
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, upstream.lastReq)
			require.NotContains(t, string(upstream.lastBody), "discarded")
			if mode == "chat-fallback" {
				require.Contains(t, upstream.lastReq.URL.Path, "/chat/completions")
				var calls, outputs []gjson.Result
				for _, message := range gjson.GetBytes(upstream.lastBody, "messages").Array() {
					calls = append(calls, message.Get("tool_calls").Array()...)
					if message.Get("role").String() == "tool" {
						outputs = append(outputs, message)
					}
				}
				require.Len(t, calls, 2)
				require.Equal(t, "call_a", calls[0].Get("id").String())
				require.Equal(t, `{"line":9007199254740993}`, calls[0].Get("function.arguments").String())
				require.Equal(t, "call_b", calls[1].Get("id").String())
				require.Len(t, outputs, 2)
				require.Equal(t, "{}", outputs[0].Get("content").String())
			} else {
				items := gjson.GetBytes(upstream.lastBody, "input").Array()
				require.Len(t, items, 4)
				require.Equal(t, "fc_a", items[0].Get("id").String())
				require.Equal(t, "call_a", items[0].Get("call_id").String())
				require.Equal(t, `{"line":9007199254740993}`, items[0].Get("arguments").String())
				require.Equal(t, "{}", items[1].Get("output").String())
				require.Equal(t, "call_b", items[2].Get("call_id").String())
			}
		})
	}
}

func TestDeepSeekDedupeStreamingPreservesLiveEventIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const arguments = `{"path":"src/main.go"}`
	events := []string{
		`{"type":"response.output_item.added","sequence_number":0,"output_index":0,"item":{"type":"function_call","id":"call_a","call_id":"call_a","name":"read","arguments":"","status":"in_progress"}}`,
		`{"type":"response.function_call_arguments.delta","sequence_number":1,"output_index":0,"item_id":"call_a","delta":"{\"path\":\"src/main.go\"}"}`,
		`{"type":"response.function_call_arguments.done","sequence_number":2,"output_index":0,"item_id":"call_a","arguments":"{\"path\":\"src/main.go\"}"}`,
		`{"type":"response.output_item.done","sequence_number":3,"output_index":0,"item":{"type":"function_call","id":"call_a","call_id":"call_a","name":"read","arguments":"{\"path\":\"src/main.go\"}","status":"completed"}}`,
		`{"type":"response.completed","sequence_number":4,"response":{"id":"resp_stream","status":"completed","output":[{"type":"function_call","id":"call_a","call_id":"call_a","name":"read","arguments":"{\"path\":\"src/main.go\"}"}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
	}
	var stream strings.Builder
	for _, event := range events {
		stream.WriteString("data: " + event + "\n\n")
	}
	for _, mode := range []string{"responses", "passthrough"} {
		t.Run(mode, func(t *testing.T) {
			body := []byte(`{"model":"deepseek-v4","stream":true,"input":"hello"}`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader(stream.String())),
			}}
			svc := openAIClientToolsTestService(upstream)
			account := &Account{ID: 7717, Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Credentials: map[string]any{
				"api_key": "test-key", "api_protocol": APIProtocolResponses, "base_url": "https://relay.example",
			}}
			var result *OpenAIForwardResult
			var err error
			if mode == "passthrough" {
				account.Platform = PlatformOpenAI
				result, err = svc.forwardOpenAIPassthrough(context.Background(), c, account, body, body, "deepseek-v4", false, nil, true, time.Now())
			} else {
				result, err = svc.Forward(context.Background(), c, account, body)
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			var forwarded []gjson.Result
			for _, line := range strings.Split(recorder.Body.String(), "\n") {
				if data, ok := strings.CutPrefix(line, "data:"); ok && gjson.Valid(strings.TrimSpace(data)) {
					forwarded = append(forwarded, gjson.Parse(strings.TrimSpace(data)))
				}
			}
			require.Len(t, forwarded, len(events))
			for i, event := range forwarded {
				require.Equal(t, gjson.Get(events[i], "type").String(), event.Get("type").String())
				require.Equal(t, int64(i), event.Get("sequence_number").Int())
			}
			require.Equal(t, "call_a", forwarded[0].Get("item.id").String())
			require.Equal(t, "call_a", forwarded[0].Get("item.call_id").String())
			require.Equal(t, "call_a", forwarded[1].Get("item_id").String())
			require.Equal(t, arguments, forwarded[1].Get("delta").String())
			require.Equal(t, "call_a", forwarded[2].Get("item_id").String())
			require.Equal(t, arguments, forwarded[2].Get("arguments").String())
			require.Equal(t, "call_a", forwarded[3].Get("item.id").String())
			require.Equal(t, "call_a", forwarded[4].Get("response.output.0.id").String())
		})
	}
}
