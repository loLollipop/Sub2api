//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMixedProtocolRefusalAfterHeartbeat(t *testing.T) {
	for _, protocol := range []string{APIProtocolChatCompletions, APIProtocolResponses, APIProtocolAnthropic} {
		for _, boundary := range []string{"heartbeat", "output", "committed", "cancelled", "write failed", "headers only"} {
			t.Run(protocol+"/"+boundary, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				state := &openAIProtocolFallbackState{clientCtx: ctx, visited: map[string]bool{}, order: []string{APIProtocolResponses, APIProtocolAnthropic, APIProtocolChatCompletions}}
				ctx = context.WithValue(ctx, openAIProtocolFallbackKey{}, state)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, protocolUpstreamEndpoint(protocol), nil).WithContext(ctx)
				if boundary == "headers only" {
					c.Writer.WriteHeaderNow()
				} else {
					stop := StartOpenAIStreamSSEKeepalive(c, time.Hour)
					defer stop()
					value, _ := c.Get(openAICompactSSEKeepaliveKey)
					keepalive := value.(*openAICompactSSEKeepalive)
					require.True(t, keepalive.beat())
					require.True(t, c.Writer.Written())
					require.False(t, openAIStreamClientOutputStarted(c, false))
					switch boundary {
					case "output":
						_, err := c.Writer.Write([]byte("data: real output\n\n"))
						require.NoError(t, err)
					case "committed":
						MarkResponseCommitted(c)
					case "cancelled":
						cancel()
					case "write failed":
						keepalive.mu.Lock()
						keepalive.writeFailed = true
						keepalive.mu.Unlock()
					}
				}
				response := mixedRefusal(protocol)
				var errorBody bytes.Buffer
				_, err := errorBody.ReadFrom(response.Body)
				require.NoError(t, err)
				_ = response.Body.Close()
				err = (&OpenAIGatewayService{}).protocolRefusalFallback(ctx, c, mixedTestAccount(), protocol, response.StatusCode, errorBody.Bytes(), false)
				if boundary == "heartbeat" {
					var refusal *openAIProtocolRefusal
					require.ErrorAs(t, err, &refusal)
					require.NotEqual(t, protocol, refusal.next)
				} else {
					require.NoError(t, err, "real output, committed errors and failed/cancelled clients must not replay")
				}
			})
		}
	}
}

func TestMixedGLMParameterAdaptationAcrossIngress(t *testing.T) {
	for _, upstreamModel := range []string{"glm-5.3", "z-ai/glm-5.3", "zhipuai/glm-5.3"} {
		for _, protocol := range []string{APIProtocolChatCompletions, APIProtocolResponses, APIProtocolAnthropic} {
			for _, effort := range []string{"", "medium", "xhigh", "low"} {
				t.Run(upstreamModel+"/"+protocol+"/"+effort, func(t *testing.T) {
					account := mixedTestAccount()
					account.Credentials["model_mapping"] = map[string]any{"glm-client": upstreamModel}
					body := map[string]any{"model": "glm-client", "stream": false}
					switch protocol {
					case APIProtocolResponses:
						body["input"] = "hello"
						if effort != "" {
							body["reasoning"] = map[string]string{"effort": effort}
						}
					case APIProtocolAnthropic:
						body["max_tokens"] = 1024
						body["messages"] = []map[string]string{{"role": "user", "content": "hello"}}
						if effort != "" {
							body["output_config"] = map[string]string{"effort": effort}
						}
					default:
						body["messages"] = []map[string]string{{"role": "user", "content": "hello"}}
						if effort != "" {
							body["reasoning_effort"] = effort
						}
					}
					encoded, err := json.Marshal(body)
					require.NoError(t, err)
					upstream := &httpUpstreamRecorder{responses: mixedResponsesForTarget(protocol, APIProtocolChatCompletions, mixedHTTPResponse(200, strings.ReplaceAll(mixedChatToolJSON, `"actual"`, `"`+upstreamModel+`"`), false))}
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
					result, _, _, err := mixedForward(t, svc, account, protocol, encoded)
					require.NoError(t, err)
					require.Equal(t, upstreamModel, result.UpstreamModel)
					require.Equal(t, upstreamModel, gjson.GetBytes(upstream.lastBody, "model").String())
					got := gjson.GetBytes(upstream.lastBody, "reasoning_effort")
					if effort == "" {
						if protocol == APIProtocolAnthropic {
							// Preserve the existing Anthropic bridge's default medium,
							// then apply the same GLM adaptation as explicit medium.
							require.Equal(t, "high", got.String())
						} else {
							require.False(t, got.Exists())
						}
						return
					}
					want := map[string]string{"medium": "high", "xhigh": "max", "low": "low"}[effort]
					require.Equal(t, want, got.String())
				})
			}
		}
	}
}

func TestMixedDispatcherFallbackAfterHeartbeat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, ingress := range []string{APIProtocolChatCompletions, APIProtocolResponses, APIProtocolAnthropic} {
		for _, outcome := range []string{APIProtocolResponses, APIProtocolAnthropic, "refused"} {
			t.Run(ingress+"/"+outcome, func(t *testing.T) {
				order := mixedExpectedProtocolOrder(ingress)
				responses := []*http.Response{}
				for _, protocol := range order {
					if protocol != outcome {
						responses = append(responses, mixedRefusal(protocol))
						continue
					}
					wire := mixedResponsesToolSSE()
					if protocol == APIProtocolAnthropic {
						wire = toolAnthropicSSEStream()
					}
					responses = append(responses, mixedHTTPResponse(200, wire, true))
					break
				}
				upstream := &httpUpstreamRecorder{responses: responses}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				body := mixedClientBody(ingress, true)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, protocolUpstreamEndpoint(ingress), bytes.NewReader(body))
				c.Set("api_key", &APIKey{Group: &Group{Platform: PlatformMixed}})
				stop := StartOpenAIStreamSSEKeepalive(c, time.Hour)
				defer stop()
				value, _ := c.Get(openAICompactSSEKeepaliveKey)
				require.True(t, value.(*openAICompactSSEKeepalive).beat())
				var err error
				switch ingress {
				case APIProtocolChatCompletions:
					_, err = svc.ForwardAsChatCompletions(c.Request.Context(), c, mixedTestAccount(), body, "", "")
				case APIProtocolResponses:
					_, err = svc.Forward(c.Request.Context(), c, mixedTestAccount(), body)
				case APIProtocolAnthropic:
					_, err = svc.ForwardAsAnthropic(c.Request.Context(), c, mixedTestAccount(), body, "", "")
				}
				require.Len(t, upstream.requests, len(responses))
				for i, req := range upstream.requests {
					require.Equal(t, protocolUpstreamEndpoint(order[i]), req.URL.Path)
				}
				require.Equal(t, 1, strings.Count(rec.Body.String(), ": keepalive\n\n"))
				if outcome == "refused" {
					require.Error(t, err)
					require.NotContains(t, rec.Body.String(), "lookup")
					return
				}
				require.NoError(t, err)
				require.Contains(t, rec.Body.String(), "lookup")
				require.NotContains(t, rec.Body.String(), "does not support")
				switch ingress {
				case APIProtocolChatCompletions:
					require.Contains(t, rec.Body.String(), "chat.completion.chunk")
					require.NotContains(t, rec.Body.String(), "event: response.")
					require.NotContains(t, rec.Body.String(), "event: message_")
				case APIProtocolResponses:
					require.Contains(t, rec.Body.String(), "event: response.completed")
					require.NotContains(t, rec.Body.String(), "chat.completion.chunk")
					require.NotContains(t, rec.Body.String(), "event: message_")
				case APIProtocolAnthropic:
					require.Contains(t, rec.Body.String(), "event: message_stop")
					require.NotContains(t, rec.Body.String(), "chat.completion.chunk")
					require.NotContains(t, rec.Body.String(), "event: response.")
				}
			})
		}
	}
}

func TestMixedOneRelayAccountSupportsModelSpecificProtocols(t *testing.T) {
	account := mixedTestAccount()
	account.Credentials["model_mapping"] = map[string]any{
		"qwen/qwen3.8-27b": "qwen/qwen3.8-27b", "z-ai/glm-5.3": "z-ai/glm-5.3", "deepseek-ai/deepseek-v4-pro": "deepseek-ai/deepseek-v4-pro",
	}
	before, err := json.Marshal(account)
	require.NoError(t, err)
	for _, model := range []struct{ public, target, protocol string }{
		{"qwen3.8-27b", "qwen/qwen3.8-27b", APIProtocolAnthropic},
		{"glm-5.3", "z-ai/glm-5.3", APIProtocolChatCompletions},
		{"deepseek-v4-pro", "deepseek-ai/deepseek-v4-pro", APIProtocolResponses},
	} {
		for _, ingress := range []string{APIProtocolChatCompletions, APIProtocolResponses, APIProtocolAnthropic} {
			for _, stream := range []bool{false, true} {
				for _, history := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/stream=%t/history=%t", model.public, ingress, stream, history), func(t *testing.T) {
						wire := mixedChatToolJSON
						upstreamStream := stream
						if stream {
							wire = mixedChatToolSSE()
						}
						if model.protocol == APIProtocolResponses {
							wire = mixedResponsesToolSSE()
							upstreamStream = true
							if ingress == APIProtocolResponses && !stream {
								wire = `{"id":"resp_1","object":"response","model":"actual","status":"completed","output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup","arguments":"{}"}],"usage":{"input_tokens":9,"output_tokens":4}}`
								upstreamStream = false
							}
						}
						if model.protocol == APIProtocolAnthropic {
							wire = toolAnthropicSSEStream()
							upstreamStream = true
							if ingress == APIProtocolAnthropic && !stream {
								wire = `{"id":"msg_1","type":"message","role":"assistant","model":"actual","content":[{"type":"tool_use","id":"toolu_1","name":"lookup","input":{}}],"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}}`
								upstreamStream = false
							}
						}
						wire = strings.ReplaceAll(wire, `"actual"`, `"`+model.target+`"`)
						responses := mixedResponsesForTarget(ingress, model.protocol, mixedHTTPResponse(200, wire, upstreamStream))
						upstream := &httpUpstreamRecorder{responses: responses}
						svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
						clientBody := mixedPlainClientBody(ingress, stream)
						if history {
							clientBody = mixedClientBody(ingress, stream)
						}
						body := []byte(strings.ReplaceAll(string(clientBody), `"alias"`, `"`+model.public+`"`))
						result, rec, _, err := mixedForward(t, svc, account, ingress, body)
						require.NoError(t, err)
						require.Equal(t, model.target, result.UpstreamModel)
						require.Len(t, upstream.requests, mixedExpectedAttempts(ingress, model.protocol))
						require.Equal(t, protocolUpstreamEndpoint(model.protocol), upstream.lastReq.URL.Path)
						require.Contains(t, rec.Body.String(), "lookup")
						for i, req := range upstream.requests {
							require.Equal(t, protocolUpstreamEndpoint(mixedExpectedProtocolOrder(ingress)[i]), req.URL.Path)
							require.Equal(t, "upstream.example", req.URL.Host)
							require.Equal(t, model.target, gjson.GetBytes(upstream.bodies[i], "model").String())
							if history {
								require.Contains(t, string(upstream.bodies[i]), "prior")
							} else {
								require.Contains(t, string(upstream.bodies[i]), "hello")
							}
						}
					})
				}
			}
		}
	}
	after, err := json.Marshal(account)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after), "model routing must not persist guesses or create duplicate accounts")
}

func TestMixedGLMProviderPrefixDoesNotAlterModelOrDefaults(t *testing.T) {
	for _, model := range []string{"z-ai/glm-5.3", "zhipuai/glm-5.3"} {
		body := []byte(`{"model":"` + model + `","output_config":{"effort":"xhigh"}}`)
		got, changed := NormalizeGLM53AnthropicThinking(body, model)
		require.True(t, changed)
		require.Equal(t, model, gjson.GetBytes(got, "model").String())
		require.Equal(t, "max", gjson.GetBytes(got, "output_config.effort").String())
		body = []byte(`{"model":"` + model + `","messages":[]}`)
		for _, normalizer := range []func([]byte, string) ([]byte, bool){NormalizeGLMOpenAIReasoningEffort, NormalizeGLM53AnthropicThinking} {
			got, changed = normalizer(body, model)
			require.False(t, changed)
			require.Equal(t, body, got)
		}
	}
	for _, model := range []string{"openai/glm-5.3", "unknown/glm-5.3", "z-ai/nested/glm-5.3", "qwen/qwen3.8-27b"} {
		body := []byte(`{"reasoning_effort":"xhigh","output_config":{"effort":"xhigh"}}`)
		for _, normalizer := range []func([]byte, string) ([]byte, bool){NormalizeGLMOpenAIReasoningEffort, NormalizeGLM53AnthropicThinking} {
			got, changed := normalizer(body, model)
			require.False(t, changed)
			require.Equal(t, body, got)
		}
	}
}
