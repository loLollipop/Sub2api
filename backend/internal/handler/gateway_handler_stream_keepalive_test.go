package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGatewayConvertedToolFileErrorAfterPreHeaderKeepalive(t *testing.T) {
	for _, protocol := range []string{"chat", "responses"} {
		t.Run(protocol, func(t *testing.T) {
			path := EndpointChatCompletions
			payload := `{"model":"public-alias","stream":true,"messages":[{"role":"assistant","tool_calls":[{"id":"call_file","type":"function","function":{"name":"read","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_file","content":[{"type":"file","file":{"file_id":"file_synthetic"}}]}]}`
			if protocol == "responses" {
				path = EndpointResponses
				payload = `{"model":"public-alias","stream":true,"input":[{"type":"function_call","call_id":"call_file","name":"read","arguments":"{}"},{"type":"function_call_output","call_id":"call_file","output":[{"type":"input_file","file_id":"file_synthetic"}]}]}`
			}
			c, recorder, stop := committedGatewayKeepaliveContext(t, path)
			defer stop()
			c.Set("api_key", &service.APIKey{Group: &service.Group{CrossProtocolConversionEnabled: true}})
			account := &service.Account{ID: 1, Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
				Credentials: map[string]any{"model_mapping": map[string]any{"public-alias": "claude-sonnet-4-6"}}}
			svc := &service.GatewayService{}
			var result *service.ForwardResult
			var err error
			if protocol == "chat" {
				result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, []byte(payload), nil)
			} else {
				result, err = svc.ForwardAsResponses(context.Background(), c, account, []byte(payload), nil)
			}
			require.Error(t, err)
			require.Nil(t, result)
			require.True(t, service.IsResponseCommitted(c))
			beforeFallback := recorder.Body.String()
			require.False(t, (&GatewayHandler{}).ensureForwardErrorResponse(c, false))
			require.Equal(t, beforeFallback, recorder.Body.String())
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
			errorFrames, doneFrames := 0, 0
			for _, block := range strings.Split(strings.TrimSpace(recorder.Body.String()), "\n\n") {
				for _, line := range strings.Split(block, "\n") {
					if strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") {
						continue
					}
					require.True(t, strings.HasPrefix(line, "data: "), "unexpected unframed output: %s", line)
					data := strings.TrimPrefix(line, "data: ")
					if data == "[DONE]" {
						doneFrames++
						continue
					}
					require.True(t, json.Valid([]byte(data)))
					errorPath := "error.code"
					if protocol == "responses" {
						require.Equal(t, "response.failed", gjson.Get(data, "type").String())
						errorPath = "response.error.code"
					}
					require.Equal(t, "invalid_request_error", gjson.Get(data, errorPath).String())
					require.Contains(t, data, "provider file IDs cannot be shared")
					errorFrames++
				}
			}
			require.Equal(t, 1, errorFrames)
			if protocol == "chat" {
				require.Equal(t, 1, doneFrames)
			} else {
				require.Zero(t, doneFrames)
			}
		})
	}
}

// committedGatewayKeepaliveContext forces the pre-header heartbeat through the
// service manager instead of waiting on a timer, keeping these protocol tests
// deterministic while still exercising the same writer/lifecycle path.
func committedGatewayKeepaliveContext(t *testing.T, path string) (*gin.Context, *httptest.ResponseRecorder, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	stop := service.StartOpenAIStreamSSEKeepalive(c, time.Hour)
	written, err, handled := service.WriteOpenAIStreamSSEHeartbeat(c, []byte(": keepalive\n\n"))
	require.True(t, handled)
	require.NoError(t, err)
	require.Positive(t, written)
	// The handler's accounting marker is normally updated by the slot-wait
	// helper immediately after this service call.
	recordGatewayStreamHeartbeat(c, written)
	return c, recorder, stop
}

func TestGatewayStreamingAwareErrorAfterPreHeaderKeepalive(t *testing.T) {
	c, recorder, stop := committedGatewayKeepaliveContext(t, EndpointMessages)
	defer stop()

	(&GatewayHandler{}).handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "provider stalled", false)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	require.Contains(t, body, ": keepalive\n\n")
	require.Contains(t, body, `data: {"type":"error"`)
	require.Contains(t, body, "provider stalled")
	require.NotContains(t, body, `{"type":"error","error":{"code"`)
}

func TestGatewayResponsesErrorAfterPreHeaderKeepaliveUsesResponseFailed(t *testing.T) {
	c, recorder, stop := committedGatewayKeepaliveContext(t, EndpointResponses)
	defer stop()

	(&GatewayHandler{}).responsesErrorResponse(c, http.StatusBadGateway, "upstream_error", "provider stalled")

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	require.Contains(t, body, "event: response.failed\n")
	require.Contains(t, body, `"type":"response.failed"`)
	require.NotContains(t, body, `{"error":{"code"`)
}

func TestGatewayChatCompletionsFailoverAfterPreHeaderKeepaliveEmitsOneError(t *testing.T) {
	c, recorder, stop := committedGatewayKeepaliveContext(t, EndpointChatCompletions)
	defer stop()

	(&GatewayHandler{}).chatCompletionsErrorResponse(c, http.StatusBadGateway, "upstream_error", "provider stalled")

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	require.Equal(t, 1, strings.Count(body, `data: {"type":"error"`))
	require.NotContains(t, body, `{"error":{"type"`)
}

func TestGatewayStreamingErrorsBeforeFirstKeepaliveRemainJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, EndpointResponses, nil)
	stop := service.StartOpenAIStreamSSEKeepalive(c, time.Hour)
	defer stop()

	(&GatewayHandler{}).responsesErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "bad request")

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), `{"error":{"code":"invalid_request_error"`)
	require.NotContains(t, recorder.Body.String(), "event: response.failed")
}

func TestGatewayStreamingAwareErrorAppendsAfterSemanticBytes(t *testing.T) {
	c, recorder, stop := committedGatewayKeepaliveContext(t, EndpointMessages)
	defer stop()

	_, err := c.Writer.Write([]byte("event: message_start\ndata: {}\n\n"))
	require.NoError(t, err)
	(&GatewayHandler{}).handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "late failure", false)

	require.Contains(t, recorder.Body.String(), "late failure")
}
