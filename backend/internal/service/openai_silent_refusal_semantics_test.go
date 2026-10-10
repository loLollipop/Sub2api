//go:build unit

package service

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSilentRefusalDetectorAcceptsExplicitRefusal(t *testing.T) {
	for _, payload := range []string{
		`{"choices":[{"delta":{"refusal":"cannot help"},"finish_reason":"stop"}]}`,
		`{"type":"response.refusal.delta","delta":"cannot help"}`,
		`{"type":"response.completed","response":{"output":[{"type":"message","content":[{"type":"refusal","refusal":"cannot help"}]}]}}`,
	} {
		t.Run(payload, func(t *testing.T) {
			d := newOpenAIChatSilentRefusalDetector(openAISilentRefusalMinRequestBodyBytes)
			d.ObservePayload([]byte(payload))
			require.True(t, d.ShouldReleaseClientOutput())
			require.False(t, d.IsSilentRefusal())
		})
	}
	d := newOpenAIChatSilentRefusalDetector(openAISilentRefusalMinRequestBodyBytes)
	refusal, stop := "cannot help", "stop"
	d.ObserveChatChunk(apicompat.ChatCompletionsChunk{Choices: []apicompat.ChatChunkChoice{{Delta: apicompat.ChatDelta{Refusal: &refusal}, FinishReason: &stop}}})
	require.True(t, d.ShouldReleaseClientOutput())
	require.False(t, d.IsSilentRefusal())
}

func TestChatTerminalOnlyRefusalStreamingAndBufferedParity(t *testing.T) {
	for _, stream := range []bool{true, false} {
		t.Run(fmt.Sprintf("stream=%v", stream), func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r1\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"refusal\",\"refusal\":\"cannot help\"}]}]}}\n\n"))}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
			var result *OpenAIForwardResult
			var err error
			if stream {
				result, err = svc.handleChatStreamingResponse(resp, c, rawChatCompletionsTestAccount(), "gpt-5.5", "gpt-5.5", "gpt-5.5", time.Now(), openAISilentRefusalMinRequestBodyBytes)
			} else {
				result, err = svc.handleChatBufferedStreamingResponse(resp, c, rawChatCompletionsTestAccount(), "gpt-5.5", "gpt-5.5", "gpt-5.5", time.Now())
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Contains(t, rec.Body.String(), `"refusal":"cannot help"`)
		})
	}
}
