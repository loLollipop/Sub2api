//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func chatRoleRepairStreamFixture() string {
	return strings.Join([]string{
		": relay preamble", "",
		`data:{"id":"chatcmpl-role","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{"role":""},"finish_reason":null}]}`, "",
		`data: {"id":"chatcmpl-role","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{"role":"","reasoning_content":"thinking <think>intact</think>"},"finish_reason":null}]}`, "",
		`data: {"id":"chatcmpl-role","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{"role":"","content":"hello"},"finish_reason":null}]}`, "",
		`data: {"id":"chatcmpl-role","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{"role":"","tool_calls":[{"index":0,"id":"call_00_keep","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":null}]}`, "",
		`data: {"id":"chatcmpl-role","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`, "",
		`data: {"id":"chatcmpl-role","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{"role":""},"finish_reason":"stop"}]}`, "",
		`data: {"id":"chatcmpl-role","object":"chat.completion.chunk","model":"deepseek-chat","choices":[],"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13,"prompt_tokens_details":{"cached_tokens":3}}}`, "",
		"data: [DONE]", "", "",
	}, "\n")
}

func chatRoleRepairJSONFixture(role string) string {
	return ` {"id":"chatcmpl-role","object":"chat.completion","model":"deepseek-chat","choices":[{"index":0,"message":{` + role + `"content":"hello <think>keep</think>","reasoning_content":"thinking","tool_calls":[{"id":"call_00_keep","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13,"prompt_tokens_details":{"cached_tokens":3}}} `
}

func TestForwardAsRawChatCompletionsDeepSeekRepairsRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, upstream, want string
		stream               bool
	}{
		{"stream", chatRoleRepairStreamFixture(), strings.Replace(chatRoleRepairStreamFixture(), `"delta":{"role":""}`, `"delta":{"role":"assistant"}`, 1), true},
		{"empty role", chatRoleRepairJSONFixture(`"role":"",`), chatRoleRepairJSONFixture(`"role":"assistant",`), false},
		{"missing role", chatRoleRepairJSONFixture(""), strings.Replace(chatRoleRepairJSONFixture(""), `"arguments":"{}"}}]}`, `"arguments":"{}"}}],"role":"assistant"}`, 1), false},
		{"valid role", chatRoleRepairJSONFixture(`"role":"assistant",`), chatRoleRepairJSONFixture(`"role":"assistant",`), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requestBody := `{"model":"deepseek-chat","messages":[{"role":"user","content":"hello"}],"stream":false}`
			contentType := "application/json"
			if tc.stream {
				requestBody = strings.Replace(requestBody, `"stream":false`, `"stream":true`, 1)
				contentType = "text/event-stream"
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{contentType}},
				Body:       io.NopCloser(strings.NewReader(tc.upstream)),
			}}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(requestBody))
			result, err := svc.forwardAsRawChatCompletions(context.Background(), c, deepSeekToolsTestAccount(), []byte(requestBody), "")
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, tc.want, rec.Body.String())
			require.Equal(t, "https://api.deepseek.com/v1/chat/completions", upstream.lastReq.URL.String())
			require.Equal(t, tc.stream, result.Stream)
			require.Equal(t, "deepseek-chat", result.Model)
			require.Equal(t, "deepseek-chat", result.UpstreamModel)
			require.Equal(t, 9, result.Usage.InputTokens)
			require.Equal(t, 4, result.Usage.OutputTokens)
			require.Equal(t, 3, result.Usage.CacheReadInputTokens)
		})
	}
}

func TestHandleResponsePassthroughRepairsChatRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		name := "json"
		upstreamBody := chatRoleRepairJSONFixture(`"role":"",`)
		want := chatRoleRepairJSONFixture(`"role":"assistant",`)
		contentType := "application/json"
		if stream {
			name = "stream"
			upstreamBody = chatRoleRepairStreamFixture()
			want = strings.Replace(upstreamBody, `"delta":{"role":""}`, `"delta":{"role":"assistant"}`, 1)
			contentType = "text/event-stream"
		}
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(upstreamBody))}
			svc := &OpenAIGatewayService{}
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
			if stream {
				result, err := svc.handleStreamingResponsePassthrough(context.Background(), resp, c, account, time.Now(), "deepseek-chat", "deepseek-chat")
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, 9, result.usage.InputTokens)
				require.Equal(t, 4, result.usage.OutputTokens)
				require.Equal(t, 3, result.usage.CacheReadInputTokens)
			} else {
				result, err := svc.handleNonStreamingResponsePassthrough(context.Background(), resp, c, account, "deepseek-chat", "deepseek-chat")
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, 9, result.usage.InputTokens)
				require.Equal(t, 4, result.usage.OutputTokens)
				require.Equal(t, 3, result.usage.CacheReadInputTokens)
			}
			require.Equal(t, want, rec.Body.String())
		})
	}
}

func TestHandleNonStreamingResponsePassthroughPreservesNativeResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := ` {"id":"resp-native","object":"response","model":"deepseek-chat","metadata":{"example":"chat.completion"},"choices":[{"message":{"role":""}}],"output":[{"id":"rs-native","type":"reasoning","encrypted_content":"keep-encrypted","summary":[]},{"id":"c0ffee00-1234-5678-90ab-abcdef123456","type":"function_call","call_id":"call_00_keep","name":"lookup","arguments":"{}"}],"usage":{"input_tokens":9,"output_tokens":4,"total_tokens":13}} `
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
	svc := &OpenAIGatewayService{}
	result, err := svc.handleNonStreamingResponsePassthrough(context.Background(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, "deepseek-chat", "deepseek-chat")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, body, rec.Body.String())
	require.Equal(t, 9, result.usage.InputTokens)
	require.Equal(t, 4, result.usage.OutputTokens)
}
