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

func TestGeminiBridgeStreamPreludeRecovery(t *testing.T) {
	prelude := `data: {"error":{"message":"temporary"}}` + "\n\n"
	answer := `data: {"choices":[{"index":0,"delta":{"content":"recovered answer"}}]}` + "\n\n"
	finish := `data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2}}` + "\n\n"
	done := "data: [DONE]\n\n"
	for _, tc := range []struct {
		name, body string
		wantError  bool
	}{
		{"recovers chat prelude", prelude + answer + finish + done, false},
		{"done alone cannot recover", prelude + done, true},
		{"empty finish cannot recover", prelude + `data: {"choices":[{"index":0,"delta":{},"finish_reason":""}]}` + "\n\n" + done, true},
		{"failed finish cannot recover", `data: {"choices":[{"index":0,"delta":{},"finish_reason":"error"}],"usage":{"prompt_tokens":10,"completion_tokens":2}}` + "\n\n" + done, true},
		{"named error overrides payload type", "event: error\ndata: {\"type\":\"chat.completion.chunk\",\"message\":\"failed\"}\n\n" + done, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini:streamGenerateContent", nil)
			result, err := (&GeminiMessagesCompatService{}).pipeOpenAIStreamAsGemini(context.Background(), c, strings.NewReader(tc.body), "gemini", APIProtocolChatCompletions, time.Now())
			require.NotNil(t, result)
			if tc.wantError {
				require.Error(t, err)
				require.Contains(t, w.Body.String(), `"error"`)
			} else {
				require.NoError(t, err)
				require.Contains(t, w.Body.String(), "recovered answer")
				require.Contains(t, w.Body.String(), `"finishReason":"STOP"`)
				require.NotContains(t, w.Body.String(), `"error"`)
				require.Equal(t, 10, result.Usage.InputTokens)
				require.Equal(t, 2, result.Usage.OutputTokens)
				require.False(t, result.NonBillableUpstreamError)
			}
		})
	}
}

func TestGeminiNativeEventFieldOrder(t *testing.T) {
	body := "event: error\nevent: message\ndata: " + `{"candidates":[{"content":{"parts":[{"text":"answer"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":2}}` + "\n\n"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
	result, err := (&GeminiMessagesCompatService{}).handleNativeStreamingResponse(c, resp, time.Now(), false, &Account{ID: 1, Platform: PlatformGemini}, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, body, w.Body.String())
	require.Equal(t, 10, result.usage.InputTokens)
}

func TestDetectGeminiResponseSignalNamedError(t *testing.T) {
	raw := []byte(`{"code":503,"status":"UNAVAILABLE","message":"retry later","candidates":[{"finishReason":"STOP"}]}`)
	sig, ok := detectGeminiResponseSignal(raw, "error")
	require.True(t, ok)
	require.Equal(t, geminiSignalError, sig.Kind)
	require.Equal(t, http.StatusServiceUnavailable, sig.Status)
	_, ok = detectGeminiResponseSignal(raw, "message")
	require.False(t, ok)
}
