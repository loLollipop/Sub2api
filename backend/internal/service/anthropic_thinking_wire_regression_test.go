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

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAnthropicThinkingMappedMessagesWire(t *testing.T) {
	for _, route := range []string{"gateway", "passthrough", "native"} {
		for _, tc := range []struct{ model, mode, want string }{
			{"claude-opus-5-5", "enabled", "adaptive"},
			{"claude-opus-5-5", "between_tools", "adaptive"},
			{"claude-fable-5-1", "between_tools", "adaptive"},
			{"claude-sonnet-5-5", "between_tools", "between_tools"},
			{"claude-haiku-5-5", "disabled", "disabled"},
			{"claude-opus-5", "between_tools", "between_tools"},
			{"deepseek-v4", "between_tools", "between_tools"},
		} {
			// Native Messages already shares validation/sanitization; one new-mode
			// regression is sufficient alongside its existing native request tests.
			if route == "native" && tc.model != "claude-opus-5-5" {
				continue
			}
			t.Run(route+"/"+tc.model+"/"+tc.mode, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				body := []byte(fmt.Sprintf(`{"model":"public-alias","max_tokens":100,"thinking":{"type":%q},"messages":[{"role":"user","content":"hi"}]}`, tc.mode))
				upstream := &anthropicHTTPUpstreamRecorder{resp: nativeAnthropicBufferedResponse()}
				account := newAnthropicAPIKeyAccountForTest()
				account.Credentials["model_mapping"] = map[string]any{"public-alias": tc.model}
				account.Extra["anthropic_passthrough"] = route == "passthrough"
				if route == "native" {
					account.Platform = PlatformCommandCode
					account.Credentials["api_protocol"] = APIProtocolAnthropic
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
					_, err := svc.forwardAnthropicViaNativeAnthropicEndpoint(context.Background(), c, account, body, "")
					require.NoError(t, err)
				} else {
					svc := newForwardPartialUsageServiceForTest(upstream)
					_, err := svc.Forward(context.Background(), c, account, &ParsedRequest{Model: "public-alias", Body: NewRequestBodyRef(body)})
					require.NoError(t, err)
				}
				require.NotEmpty(t, upstream.lastBody)
				require.Equal(t, tc.model, gjson.GetBytes(upstream.lastBody, "model").String())
				require.Equal(t, tc.want, gjson.GetBytes(upstream.lastBody, "thinking.type").String())
			})
		}
	}
}

func TestAnthropicThinkingCountBuildersWire(t *testing.T) {
	for _, pass := range []bool{false, true} {
		for _, tc := range []struct{ model, mode, want string }{
			{"claude-opus-5-5", "enabled", "adaptive"},
			{"claude-opus-5-5", "between_tools", "adaptive"},
			{"claude-fable-5-1", "between_tools", "adaptive"},
			{"claude-sonnet-5-5", "between_tools", "between_tools"},
			{"claude-opus-5-5", "disabled", "disabled"},
			{"claude-haiku-5-5", "enabled", "enabled"},
			{"claude-opus-5", "between_tools", "between_tools"},
			{"deepseek-v4", "enabled", "enabled"},
		} {
			t.Run(fmt.Sprintf("pass=%v/%s/%s", pass, tc.model, tc.mode), func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)
				body := []byte(fmt.Sprintf(`{"model":%q,"thinking":{"type":%q,"budget_tokens":1024},"messages":[{"role":"user","content":"hi"}]}`, tc.model, tc.mode))
				svc := &GatewayService{cfg: rawChatCompletionsTestConfig()}
				account := newAnthropicAPIKeyAccountForTest()
				var req *http.Request
				var err error
				if pass {
					req, err = svc.buildCountTokensRequestAnthropicAPIKeyPassthrough(context.Background(), c, account, body, "test-only")
				} else {
					req, _, err = svc.buildCountTokensRequest(context.Background(), c, account, body, "test-only", "apikey", tc.model, false)
				}
				require.NoError(t, err)
				require.NotNil(t, req)
				wire, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.NoError(t, req.Body.Close())
				require.Equal(t, tc.want, gjson.GetBytes(wire, "thinking.type").String())
				require.Equal(t, tc.want != "adaptive", gjson.GetBytes(wire, "thinking.budget_tokens").Exists())
				require.Equal(t, tc.model, gjson.GetBytes(wire, "model").String())
			})
		}
	}
}

func TestAnthropicThinkingBridgesMappedWire(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, chat := range []bool{false, true} {
			for _, tc := range []struct{ model, effort, want string }{
				{"claude-fable-5-1", "high", "adaptive"},
				{"claude-opus-5-5", "high", "adaptive"},
				{"claude-sonnet-5-5", "none", "between_tools"},
				{"claude-haiku-5-5", "none", "disabled"},
				{"claude-opus-5", "high", "enabled"},
				{"deepseek-v4", "high", "enabled"},
			} {
				t.Run(fmt.Sprintf("native=%v/chat=%v/%s", native, chat, tc.model), func(t *testing.T) {
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					path := "/v1/responses"
					body := []byte(fmt.Sprintf(`{"model":"public-alias","input":"hi","reasoning":{"effort":%q}}`, tc.effort))
					if chat {
						path = "/v1/chat/completions"
						body = []byte(fmt.Sprintf(`{"model":"public-alias","messages":[{"role":"user","content":"hi"}],"reasoning_effort":%q}`, tc.effort))
					}
					c.Request = httptest.NewRequest(http.MethodPost, path, nil)
					c.Set("api_key", &APIKey{Group: &Group{CrossProtocolConversionEnabled: true}})
					upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK,
						Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(miniAnthropicSSEStream()))}}
					account := newAnthropicAPIKeyAccountForTest()
					account.Credentials["model_mapping"] = map[string]any{"public-alias": tc.model}
					var err error
					if native {
						account.Platform = PlatformCommandCode
						account.Credentials["api_protocol"] = APIProtocolAnthropic
						svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
						if chat {
							_, err = svc.forwardChatCompletionsViaNativeAnthropic(context.Background(), c, account, body, "")
						} else {
							_, err = svc.forwardResponsesViaNativeAnthropic(context.Background(), c, account, body, "")
						}
					} else {
						svc := newForwardPartialUsageServiceForTest(upstream)
						if chat {
							_, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, nil)
						} else {
							_, err = svc.ForwardAsResponses(context.Background(), c, account, body, nil)
						}
					}
					require.NoError(t, err)
					require.NotEmpty(t, upstream.lastBody)
					require.Equal(t, tc.model, gjson.GetBytes(upstream.lastBody, "model").String())
					require.Equal(t, tc.want, gjson.GetBytes(upstream.lastBody, "thinking.type").String())
					if tc.want == "adaptive" {
						require.False(t, gjson.GetBytes(upstream.lastBody, "thinking.budget_tokens").Exists())
					}
				})
			}
		}
	}
}

func TestAnthropicThinkingBridgesStillRequireConversionOptIn(t *testing.T) {
	for _, chat := range []bool{false, true} {
		t.Run(fmt.Sprintf("chat=%v", chat), func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Set("api_key", &APIKey{Group: &Group{CrossProtocolConversionEnabled: false}})
			var err error
			if chat {
				_, err = (&GatewayService{}).ForwardAsChatCompletions(context.Background(), c, newAnthropicAPIKeyAccountForTest(), []byte(`{}`), nil)
			} else {
				_, err = (&GatewayService{}).ForwardAsResponses(context.Background(), c, newAnthropicAPIKeyAccountForTest(), []byte(`{}`), nil)
			}
			require.Error(t, err)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Contains(t, rec.Body.String(), crossProtocolDisabledCode)
		})
	}
}

func TestAnthropicThinkingInvalidOriginalMessagesStillRejected(t *testing.T) {
	for _, route := range []string{"gateway", "passthrough", "native"} {
		for _, tc := range []struct{ name, fields, want string }{
			{"disabled", `"max_tokens":100,"messages":[{"role":"user","content":"hi"}],"thinking":{"type":"disabled"}`, "thinking"},
			{"missing max", `"messages":[{"role":"user","content":"hi"}],"thinking":{"type":"between_tools"}`, "max_tokens"},
			{"missing messages", `"max_tokens":100,"thinking":{"type":"between_tools"}`, "messages"},
			{"nondefault sampling", `"max_tokens":100,"messages":[{"role":"user","content":"hi"}],"thinking":{"type":"between_tools"},"temperature":0.5`, "temperature"},
		} {
			t.Run(route+"/"+tc.name, func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				body := []byte(`{"model":"public-alias",` + tc.fields + `}`)
				CaptureMessagesValidationFacts(c, body)
				account := newAnthropicAPIKeyAccountForTest()
				account.Credentials["model_mapping"] = map[string]any{"public-alias": "claude-opus-5-5"}
				account.Extra["anthropic_passthrough"] = route == "passthrough"
				var err error
				// No transport configured: every error must be returned locally.
				if route == "native" {
					account.Platform = PlatformCommandCode
					_, err = (&OpenAIGatewayService{}).forwardAnthropicViaNativeAnthropicEndpoint(context.Background(), c, account, body, "")
				} else {
					_, err = (&GatewayService{}).Forward(context.Background(), c, account, &ParsedRequest{Model: "public-alias", Body: NewRequestBodyRef(body)})
				}
				require.ErrorContains(t, err, tc.want)
				require.Equal(t, http.StatusBadRequest, rec.Code)
				require.Contains(t, rec.Body.String(), "invalid_request_error")
			})
		}
	}
}
