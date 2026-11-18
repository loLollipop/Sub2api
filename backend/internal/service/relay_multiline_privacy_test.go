package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRelayPrivacyMultilineErrorsAtLocalStreamBoundaries(t *testing.T) {
	const failure = `{"type":"error","error":{"type":"new_api_error","message":"bad input (request id: Synthetic123)"}}`
	const successMarker = "new_api_error (request id: Content123)"
	for _, protocol := range []string{"antigravity", "native", "images", "images-timed", "gemini"} {
		for _, header := range []string{"", "event: error\n"} {
			t.Run(protocol+map[bool]string{false: "/data", true: "/named"}[header != ""], func(t *testing.T) {
				prefix := "data: " + `{"candidates":[{"content":{"parts":[{"text":"` + successMarker + `"}]}}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":3}}` + "\n\n"
				if protocol == "native" {
					prefix = "event: message_start\ndata: " + `{"type":"message_start","message":{"usage":{"input_tokens":11,"cache_read_input_tokens":7}}}` + "\n\nevent: content_block_delta\ndata: " + `{"type":"content_block_delta","delta":{"text":"` + successMarker + `"}}` + "\n\n"
				} else if strings.HasPrefix(protocol, "images") {
					prefix = "data: " + `{"type":"image_generation.partial_image","partial_image_b64":"` + successMarker + `","usage":{"input_tokens":11,"output_tokens":3}}` + "\n\n"
				}
				frame := header + "data: {\"type\":\"error\",\ndata: \"error\":{\"type\":\"new_api_error\",\"message\":\"bad input (request id: Synthetic123)\"}}\n\n"
				response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(prefix + frame))}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				cfg := &config.Config{}
				cfg.Gateway.LogUpstreamErrorBody = true
				if protocol == "images-timed" {
					cfg.Gateway.ImageStreamKeepaliveInterval = 1
				}
				switch protocol {
				case "antigravity":
					svc := &AntigravityGatewayService{settingService: &SettingService{cfg: cfg}}
					result, err := svc.handleGeminiStreamingResponse(c, response, time.Now())
					require.NoError(t, err)
					require.Equal(t, 11, result.usage.InputTokens)
					require.Equal(t, 3, result.usage.OutputTokens)
				case "native":
					svc := &OpenAIGatewayService{cfg: cfg}
					result, err := svc.handleNativeAnthropicStreamingResponse(t.Context(), response, c, nativeAnthropicTestAccount(), "k3", "k3", "k3", nil, time.Now())
					require.NoError(t, err)
					require.Equal(t, 18, result.Usage.InputTokens)
					require.Equal(t, 7, result.Usage.CacheReadInputTokens)
				case "images", "images-timed":
					svc := &OpenAIGatewayService{cfg: cfg}
					usage, _, _, _, err := svc.handleOpenAIImagesStreamingResponse(response, c, time.Now())
					require.NoError(t, err)
					require.Equal(t, 11, usage.InputTokens)
					require.Equal(t, 3, usage.OutputTokens)
				case "gemini":
					svc := &GeminiMessagesCompatService{cfg: cfg}
					result, err := svc.handleNativeStreamingResponse(c, response, time.Now(), false, &Account{ID: 1, Platform: PlatformGemini}, "fixture-request")
					require.NoError(t, err)
					require.Equal(t, 11, result.usage.InputTokens)
					require.Equal(t, 3, result.usage.OutputTokens)
				}
				output := rec.Body.String()
				require.Contains(t, output, successMarker, "successful content must remain unchanged")
				if protocol != "antigravity" {
					require.True(t, strings.HasPrefix(output, prefix), "successful SSE bytes must remain unchanged")
				}
				require.NotContains(t, output, `"type":"new_api_error"`)
				require.NotContains(t, output, "request id: Synthetic123")
				var parser openAICompatSSEFrameParser
				var errorFrame openAICompatSSEFrame
				for _, line := range strings.Split(output, "\n") {
					if parsed, ok := parser.AddLine(line); ok && gjson.Get(parsed.Data, "type").String() == "error" {
						errorFrame = parsed
					}
				}
				require.True(t, gjson.Valid(errorFrame.Data), "terminal error must be one valid JSON document: %s", output)
				require.Equal(t, "upstream_error", gjson.Get(errorFrame.Data, "error.type").String())
				require.Equal(t, "bad input", gjson.Get(errorFrame.Data, "error.message").String())
				if header != "" {
					require.Equal(t, "error", errorFrame.EventType)
				}
				if !strings.HasPrefix(protocol, "images") {
					require.Equal(t, "bad input (request id: Synthetic123)", c.GetString(OpsUpstreamErrorMessageKey))
					require.JSONEq(t, failure, c.GetString(OpsUpstreamErrorDetailKey))
				}
			})
		}
	}
}
