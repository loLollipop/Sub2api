//go:build unit

package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func mixedChatTextSSE(text string) string {
	chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": text}, "finish_reason": "stop"}}})
	return "data: " + string(chunk) + "\n\ndata: [DONE]\n\n"
}

func mixedResponsesTextSSE() string {
	return strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_text","model":"actual","status":"in_progress","output":[]}}`, "",
		`data: {"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"Hello"}`, "",
		`data: {"type":"response.completed","response":{"id":"resp_text","object":"response","model":"actual","status":"completed","output":[{"type":"message","id":"msg_text","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Hello"}]}],"usage":{"input_tokens":9,"output_tokens":4}}}`, "", "",
	}, "\n")
}

func runMixedAccountProbe(t *testing.T, upstream *httpUpstreamRecorder, account *Account, model, prompt string) (string, error) {
	t.Helper()
	gateway := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	svc := &AccountTestService{openaiGatewayService: gateway}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/test", nil)
	err := svc.testMixedAccountConnection(c, account, model, prompt)
	return rec.Body.String(), err
}

func TestMixedConnectivityRequiresReplyText(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		wantText   string
	}{
		{"normal", mixedChatTextSSE("Hello"), "Hello"},
		{"finish without done", strings.ReplaceAll(mixedChatTextSSE("Hello"), "data: [DONE]\n\n", ""), "Hello"},
		{"truncated text", "data: " + `{"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}` + "\n\n", ""},
		{"empty", mixedChatTextSSE(""), ""},
		{"null content", "data: " + `{"choices":[{"delta":{"content":null},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n", ""},
		{"whitespace", mixedChatTextSSE(" \t\n"), ""},
		{"reasoning only", strings.ReplaceAll(mixedChatTextSSE("thinking"), `"content"`, `"reasoning_content"`), ""},
		{"tool only", mixedChatToolSSE(), ""},
		{"no body", "", ""},
		{"done only", "data: [DONE]\n\n", ""},
		{"array content", "data: " + `{"choices":[{"delta":{"content":[{"type":"text","text":"Hello"}]},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n", ""},
		{"number content", "data: " + `{"choices":[{"delta":{"content":42},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n", ""},
		{"invalid choices", "data: " + `{"choices":{"delta":{"content":"Hello"}}}` + "\n\ndata: [DONE]\n\n", ""},
		{"invalid message", "data: " + `{"choices":[{"delta":["Hello"],"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: mixedHTTPResponse(http.StatusOK, tt.body, true)}
			body, err := runMixedAccountProbe(t, upstream, mixedTestAccount(), "alias", "Hi")
			require.Len(t, upstream.requests, 1)
			require.NotContains(t, body, "Upstream:")
			text, probeErr := parseTestSSEOutput(body)
			if tt.wantText != "" {
				require.NoError(t, err)
				require.Empty(t, probeErr)
				require.Equal(t, tt.wantText, text)
				require.Contains(t, body, `"success":true`)
				require.NotContains(t, body, "/v1/")
			} else {
				require.Error(t, err)
				require.NotEmpty(t, probeErr)
				require.NotContains(t, body, `"success":true`)
			}
		})
	}
}

func TestMixedConnectivityReusesPayloadAndMapsModelOnce(t *testing.T) {
	for _, prompt := range []string{"  Reply normally  ", " \t "} {
		upstream := &httpUpstreamRecorder{resp: mixedHTTPResponse(http.StatusOK, mixedChatTextSSE("Hello"), true)}
		account := mixedTestAccount()
		before, err := json.Marshal(account)
		require.NoError(t, err)
		body, err := runMixedAccountProbe(t, upstream, account, " alias ", prompt)
		require.NoError(t, err)
		require.Contains(t, body, `"model":"alias"`)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(upstream.lastBody, &payload))
		require.Equal(t, "actual", payload["model"])
		require.Equal(t, true, payload["stream"])
		require.NotContains(t, payload, "max_tokens")
		require.NotContains(t, payload, "max_completion_tokens")
		require.NotContains(t, payload, "enable_thinking")
		require.NotContains(t, payload, "thinking")
		require.NotContains(t, payload, "reasoning_effort")
		payload["model"] = "alias"
		delete(payload, "stream_options") // The shared raw dispatcher adds usage reporting.
		expected, err := json.Marshal(createOpenAIChatCompletionsTestPayload("alias", prompt))
		require.NoError(t, err)
		actual, err := json.Marshal(payload)
		require.NoError(t, err)
		require.JSONEq(t, string(expected), string(actual))
		require.Equal(t, "upstream.example", upstream.lastReq.URL.Host)
		require.Equal(t, "Bearer test-secret", upstream.lastReq.Header.Get("Authorization"))
		after, err := json.Marshal(account)
		require.NoError(t, err)
		require.JSONEq(t, string(before), string(after))
	}
}

func TestMixedConnectivityFallbackRequiresReplyText(t *testing.T) {
	for _, tt := range []struct {
		name, target, reply string
		wantSuccess         bool
	}{
		{"responses text", APIProtocolResponses, mixedResponsesTextSSE(), true},
		{"anthropic text", APIProtocolAnthropic, strings.ReplaceAll(miniAnthropicSSEStream(), "glm-4.7", "actual"), true},
		{"responses tools", APIProtocolResponses, mixedResponsesToolSSE(), false},
		{"anthropic tools", APIProtocolAnthropic, toolAnthropicSSEStream(), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			responses := []*http.Response{mixedRefusal(APIProtocolChatCompletions)}
			if tt.target == APIProtocolAnthropic {
				responses = append(responses, mixedRefusal(APIProtocolResponses))
			}
			responses = append(responses, mixedHTTPResponse(http.StatusOK, tt.reply, true))
			attempts := len(responses)
			upstream := &httpUpstreamRecorder{responses: responses}
			body, err := runMixedAccountProbe(t, upstream, mixedTestAccount(), "alias", "Hi")
			require.Len(t, upstream.requests, attempts)
			require.Equal(t, protocolUpstreamEndpoint(tt.target), upstream.lastReq.URL.Path)
			text, probeErr := parseTestSSEOutput(body)
			if tt.wantSuccess {
				require.NoError(t, err)
				require.Empty(t, probeErr)
				require.Equal(t, "Hello", text)
				require.Contains(t, body, `"success":true`)
				require.NotContains(t, body, "/v1/")
			} else {
				require.Error(t, err)
				require.Empty(t, text)
				require.NotEmpty(t, probeErr)
				require.NotContains(t, body, `"success":true`)
			}
			for _, request := range upstream.requests {
				require.Equal(t, "upstream.example", request.URL.Host)
			}
		})
	}
}

func TestMixedTextRequirementKeepsLegacyParserBehavior(t *testing.T) {
	svc := &AccountTestService{}
	for _, requireText := range []bool{false, true} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		var err error
		if requireText {
			err = svc.processOpenAIChatCompletionsStreamWithTextRequirement(c, strings.NewReader(mixedChatToolSSE()), true)
		} else {
			err = svc.processOpenAIChatCompletionsStream(c, strings.NewReader(mixedChatToolSSE()))
		}
		if requireText {
			require.Error(t, err)
			require.NotContains(t, rec.Body.String(), `"success":true`)
		} else {
			require.NoError(t, err)
			require.Contains(t, rec.Body.String(), `"success":true`)
			require.Contains(t, rec.Body.String(), "/v1/chat/completions")
		}
	}
}
