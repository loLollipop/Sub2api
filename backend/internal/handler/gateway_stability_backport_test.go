package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGatewayStabilityExhaustedFailoverStatus(t *testing.T) {
	for _, handler := range []string{"anthropic", "openai", "anthropic_bridge", "gemini", "anthropic_simple", "openai_simple"} {
		for _, upstreamStatus := range []int{0, 400, 401, 403, 404, 429, 500, 502, 503, 504, 520, 529} {
			t.Run(fmt.Sprintf("%s/%d", handler, upstreamStatus), func(t *testing.T) {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				err := &service.UpstreamFailoverError{StatusCode: upstreamStatus, ResponseBody: []byte(`{"error":{"message":"provider rejected request"}}`)}
				gateway := &GatewayHandler{}
				openai := &OpenAIGatewayHandler{}
				switch handler {
				case "anthropic":
					gateway.handleFailoverExhausted(c, err, service.PlatformAnthropic, false)
				case "openai":
					openai.handleFailoverExhausted(c, err, false)
				case "anthropic_bridge":
					openai.handleAnthropicFailoverExhausted(c, err, false)
				case "gemini":
					gateway.handleGeminiFailoverExhausted(c, err)
				case "anthropic_simple":
					gateway.handleFailoverExhaustedSimple(c, upstreamStatus, false)
				case "openai_simple":
					openai.handleFailoverExhaustedSimple(c, upstreamStatus, false)
				}
				want := upstreamStatus
				if upstreamStatus == 401 || upstreamStatus == 403 {
					want = 502
				} else if upstreamStatus == 0 || upstreamStatus >= 500 && upstreamStatus != 503 {
					want = 529
				}
				require.Equal(t, want, recorder.Code)
				if want == 529 {
					require.Contains(t, recorder.Body.String(), "Upstream service temporarily unavailable")
					if handler != "gemini" {
						require.Equal(t, "overloaded_error", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
					}
				}
			})
		}
	}
}

func TestGatewayStabilityExhaustedFailoverFinancialMask(t *testing.T) {
	for _, handler := range []string{"anthropic", "openai", "anthropic_bridge", "gemini"} {
		t.Run(handler, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			err := &service.UpstreamFailoverError{StatusCode: 500, ResponseBody: []byte(`{"error":{"type":"overloaded_error","message":"insufficient balance"}}`)}
			switch handler {
			case "anthropic":
				(&GatewayHandler{}).handleFailoverExhausted(c, err, service.PlatformAnthropic, false)
			case "openai":
				(&OpenAIGatewayHandler{}).handleFailoverExhausted(c, err, false)
			case "anthropic_bridge":
				(&OpenAIGatewayHandler{}).handleAnthropicFailoverExhausted(c, err, false)
			case "gemini":
				(&GatewayHandler{}).handleGeminiFailoverExhausted(c, err)
			}
			require.Equal(t, 502, recorder.Code)
			require.NotContains(t, strings.ToLower(recorder.Body.String()), "balance")
			require.Contains(t, recorder.Body.String(), service.UpstreamUnavailableMessage)
		})
	}
}

func TestGatewayStabilityDisconnectedFallbackDoesNotWrite(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(fmt.Sprint(partial), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			ctx, cancel := context.WithCancel(context.Background())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
			if partial {
				_, _ = c.Writer.WriteString("partial")
			}
			before := recorder.Body.String()
			cancel()
			require.False(t, (&GatewayHandler{}).ensureForwardErrorResponse(c, partial))
			require.Equal(t, before, recorder.Body.String())
			if !partial {
				require.Equal(t, statusClientClosedRequest, c.Writer.Status())
				require.False(t, c.Writer.Written())
			}
		})
	}
}
