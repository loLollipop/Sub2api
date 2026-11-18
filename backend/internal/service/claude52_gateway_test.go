//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestClaude52MappedMessagesValidationBeforeUpstream(t *testing.T) {
	for _, model := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		for _, route := range []string{"gateway", "passthrough", "native", "bedrock", "count_tokens"} {
			for _, invalid := range []string{`"temperature":0.5`, `"top_p":1`, `"top_k":0`, `"temperature":1,"top_p":0.99`, `"thinking":{"type":"enabled","budget_tokens":1024}`, `"thinking":{"type":"disabled"}`, `"thinking":{"type":"between_tools"}`, `"output_config":{"effort":"none"}`} {
				t.Run(model+"/"+route+"/"+invalid, func(t *testing.T) {
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
					body := []byte(`{"model":"public-alias","max_tokens":100,"messages":[{"role":"user","content":"hi"}],` + invalid + `}`)
					account := newAnthropicAPIKeyAccountForTest()
					account.Credentials["model_mapping"] = map[string]any{"public-alias": model}
					account.Extra["anthropic_passthrough"] = route == "passthrough"
					parsed := &ParsedRequest{Model: "public-alias", Body: NewRequestBodyRef(body)}
					var err error
					switch route {
					case "native":
						account.Platform = PlatformCommandCode
						_, err = (&OpenAIGatewayService{}).forwardAnthropicViaNativeAnthropicEndpoint(context.Background(), c, account, body, "")
					case "count_tokens":
						err = (&GatewayService{}).ForwardCountTokens(context.Background(), c, account, parsed)
					default:
						if route == "bedrock" {
							account.Type = AccountTypeBedrock
							account.Credentials["model_mapping"] = map[string]any{"public-alias": "anthropic." + model}
						}
						_, err = (&GatewayService{}).Forward(context.Background(), c, account, parsed)
					}
					// No transport is configured: validation must precede any upstream attempt.
					require.Error(t, err)
					require.Equal(t, http.StatusBadRequest, rec.Code)
					require.Contains(t, rec.Body.String(), "invalid_request_error")
				})
			}
		}
	}
}

func TestClaude52MappedBridgeValidationBeforeUpstream(t *testing.T) {
	for _, model := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		for _, route := range []string{"responses", "chat", "native_responses", "native_chat"} {
			for _, invalid := range []string{`"temperature":0.5`, `"top_p":1`, `"top_k":0`, `"temperature":1,"top_p":0.99`, `"tool_choice":"required"`} {
				t.Run(model+"/"+route+"/"+invalid, func(t *testing.T) {
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
					body := []byte(`{"model":"public-alias","input":"hi","messages":[{"role":"user","content":"hi"}],` + invalid + `}`)
					account := newAnthropicAPIKeyAccountForTest()
					account.Credentials["model_mapping"] = map[string]any{"public-alias": model}
					var err error
					switch route {
					case "responses":
						_, err = (&GatewayService{}).ForwardAsResponses(context.Background(), c, account, body, nil)
					case "chat":
						_, err = (&GatewayService{}).ForwardAsChatCompletions(context.Background(), c, account, body, nil)
					case "native_responses":
						account.Platform = PlatformCommandCode
						_, err = (&OpenAIGatewayService{}).forwardResponsesViaNativeAnthropic(context.Background(), c, account, body, "")
					case "native_chat":
						account.Platform = PlatformCommandCode
						_, err = (&OpenAIGatewayService{}).forwardChatCompletionsViaNativeAnthropic(context.Background(), c, account, body, "")
					}
					require.Error(t, err)
					require.Equal(t, http.StatusBadRequest, rec.Code)
					require.Contains(t, rec.Body.String(), "invalid_request_error")
				})
			}
		}
	}
}

func TestClaude52MappedBridgesForwardAdaptiveThinking(t *testing.T) {
	for _, model := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		for _, route := range []string{"responses", "chat", "native_responses", "native_chat"} {
			t.Run(model+"/"+route, func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
					StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
					Body: io.NopCloser(strings.NewReader(miniAnthropicSSEStream())),
				}}
				cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
				account := newAnthropicAPIKeyAccountForTest()
				account.Credentials["model_mapping"] = map[string]any{"public-alias": model}
				body := []byte(`{"model":"public-alias","input":"hi","messages":[{"role":"user","content":"hi"}]}`)
				var err error
				if strings.HasPrefix(route, "native_") {
					account.Platform = PlatformCommandCode
					account.Credentials["api_protocol"] = APIProtocolAnthropic
					svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
					if route == "native_responses" {
						_, err = svc.forwardResponsesViaNativeAnthropic(context.Background(), c, account, body, "")
					} else {
						_, err = svc.forwardChatCompletionsViaNativeAnthropic(context.Background(), c, account, body, "")
					}
				} else {
					svc := &GatewayService{cfg: cfg, httpUpstream: upstream, responseHeaderFilter: compileResponseHeaderFilter(cfg)}
					if route == "responses" {
						_, err = svc.ForwardAsResponses(context.Background(), c, account, body, nil)
					} else {
						_, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, nil)
					}
				}
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, rec.Code)
				require.Equal(t, model, gjson.GetBytes(upstream.lastBody, "model").String())
				require.Equal(t, "adaptive", gjson.GetBytes(upstream.lastBody, "thinking.type").String())
				require.Equal(t, "high", gjson.GetBytes(upstream.lastBody, "output_config.effort").String())
				require.False(t, gjson.GetBytes(upstream.lastBody, "thinking.budget_tokens").Exists())
			})
		}
	}
}

func TestClaude52SignedHistoryAtMappedResponseBoundary(t *testing.T) {
	const sse = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\",\"model\":\"upstream-alias\",\"content\":[],\"usage\":{\"input_tokens\":1}}}\n\n" +
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\",\"thinking\":\"\"}}\n\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"signature_delta\",\"signature\":\"signed\"}}\n\n" +
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	for _, model := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		for _, native := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/native=%v", model, native), func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				resp := &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(sse))}
				cfg := &config.Config{}
				if native {
					_, err := (&OpenAIGatewayService{cfg: cfg}).handleResponsesBufferedFromNativeAnthropic(resp, c, "alias", model, model, nil, time.Now(), apicompat.ResponsesClientToolMapping{})
					require.NoError(t, err)
				} else {
					_, err := (&GatewayService{cfg: cfg}).handleResponsesBufferedStreamingResponse(resp, c, "alias", model, nil, time.Now(), apicompat.ResponsesClientToolMapping{})
					require.NoError(t, err)
				}
				require.Contains(t, rec.Body.String(), "anthropic-thinking-v1:")
				require.Contains(t, rec.Body.String(), `"model":"alias"`)
			})
		}
	}
}

func TestClaude52ValidationKeepsOlderModelContracts(t *testing.T) {
	for _, model := range []string{"claude-sonnet-4-6", "claude-opus-4-8", "claude-fable-5-1", "claude-fable-5-20", "claude-mythos-5-2-preview"} {
		require.NoError(t, validateSelectedAnthropicMessagesRequest(nil, []byte(`{"temperature":0.5,"top_p":1,"top_k":40,"thinking":{"type":"enabled","budget_tokens":1024}}`), model))
	}
}
