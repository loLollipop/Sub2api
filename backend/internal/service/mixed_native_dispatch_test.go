//go:build unit

package service

import (
	"bytes"
	"encoding/json"
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

func mixedPlainClientBody(protocol string, stream bool) []byte {
	fields := `"messages":[{"role":"user","content":"hello"}]`
	if protocol == APIProtocolResponses {
		fields = `"input":"hello"`
	} else if protocol == APIProtocolAnthropic {
		fields = `"max_tokens":128,` + fields
	}
	return []byte(fmt.Sprintf(`{"model":"alias","stream":%t,%s}`, stream, fields))
}

func TestMixedResponsesMatchesExistingOpenAINativeDeepSeek(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, history := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%t/history=%t", stream, history), func(t *testing.T) {
				body := mixedPlainClientBody(APIProtocolResponses, stream)
				if history {
					body = []byte(fmt.Sprintf(`{"model":"alias","stream":%t,"input":[{"type":"function_call","call_id":"prior","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"prior","output":"first-result"},{"type":"function_call","call_id":"prior","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"prior","output":"later-result"}],"tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}`, stream))
				}
				body = []byte(strings.TrimSuffix(string(body), "}") + `,"max_output_tokens":128}`)
				var upstreams []*httpUpstreamRecorder
				for _, platform := range []string{PlatformOpenAI, PlatformMixed} {
					account := mixedTestAccount()
					account.Platform = platform
					account.Extra["openai_passthrough"] = false
					account.Credentials["model_mapping"] = map[string]any{"alias": "deepseek-v4-pro-0813", "deepseek-v4-pro-0813": "wrong-double-mapping"}
					wire := `{"id":"resp_1","object":"response","model":"deepseek-v4-pro-0813","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":9,"output_tokens":4}}`
					if stream {
						wire = strings.ReplaceAll(mixedResponsesTextSSE(), `"actual"`, `"deepseek-v4-pro-0813"`)
					}
					upstream := &httpUpstreamRecorder{resp: mixedHTTPResponse(200, wire, stream)}
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
					_, _, _, err := mixedForward(t, svc, account, APIProtocolResponses, body)
					require.NoError(t, err)
					require.Len(t, upstream.requests, 1)
					require.Equal(t, "/v1/responses", upstream.lastReq.URL.Path)
					require.Equal(t, "deepseek-v4-pro-0813", gjson.GetBytes(upstream.lastBody, "model").String())
					require.Equal(t, int64(128), gjson.GetBytes(upstream.lastBody, "max_output_tokens").Int(), "native Responses must preserve the caller's output limit")
					if history {
						require.Len(t, gjson.GetBytes(upstream.lastBody, "input").Array(), 2)
						require.Contains(t, string(upstream.lastBody), "first-result")
						require.NotContains(t, string(upstream.lastBody), "later-result")
					}
					upstreams = append(upstreams, upstream)
				}
				require.Equal(t, upstreams[0].lastReq.URL.String(), upstreams[1].lastReq.URL.String())
				require.JSONEq(t, string(upstreams[0].lastBody), string(upstreams[1].lastBody))
			})
		}
	}
}

func TestMixedMessagesMatchesNativeHelperWithoutCrossProtocolPermission(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, history := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%t/history=%t", stream, history), func(t *testing.T) {
				messages := `[{"role":"user","content":"hello"}]`
				if history {
					messages = `[{"role":"assistant","content":[{"type":"thinking","thinking":"check","signature":"sig"},{"type":"tool_use","id":"prior","name":"lookup","input":{"id":1}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"prior","content":"result"}]}]`
				}
				body := []byte(fmt.Sprintf(`{"model":"alias","stream":%t,"max_tokens":2048,"system":[{"type":"text","text":"Keep native system","cache_control":{"type":"ephemeral"}}],"thinking":{"type":"enabled","budget_tokens":1024},"messages":%s,"tools":[{"name":"lookup","description":"find an id","input_schema":{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"]}}],"tool_choice":{"type":"auto"}}`, stream, messages))
				var upstreams []*httpUpstreamRecorder
				for _, directHelper := range []bool{true, false} {
					account := mixedTestAccount()
					account.Credentials["model_mapping"] = map[string]any{"alias": "qwen3.8-27b", "qwen3.8-27b": "wrong-double-mapping"}
					before, err := json.Marshal(account)
					require.NoError(t, err)
					wire := `{"id":"msg_1","type":"message","role":"assistant","model":"qwen3.8-27b","content":[{"type":"text","text":"OK"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`
					if stream {
						wire = strings.ReplaceAll(toolAnthropicSSEStream(), `"actual"`, `"qwen3.8-27b"`)
					}
					upstream := &httpUpstreamRecorder{resp: mixedHTTPResponse(200, wire, stream)}
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
					c.Set("api_key", &APIKey{Group: &Group{Platform: PlatformComposite, CrossProtocolConversionEnabled: false}})
					require.False(t, crossProtocolConversionAllowedFromContext(c))
					if directHelper {
						_, err = svc.forwardAnthropicViaNativeAnthropicEndpoint(c.Request.Context(), c, account, body, "")
					} else {
						_, err = svc.ForwardAsAnthropic(c.Request.Context(), c, account, body, "", "")
					}
					require.NoError(t, err)
					require.Len(t, upstream.requests, 1)
					require.Equal(t, "http://upstream.example/v1/messages", upstream.lastReq.URL.String())
					require.Equal(t, "qwen3.8-27b", gjson.GetBytes(upstream.lastBody, "model").String())
					for _, field := range []string{"system", "thinking", "tools", "tool_choice", "messages"} {
						require.JSONEq(t, gjson.GetBytes(body, field).Raw, gjson.GetBytes(upstream.lastBody, field).Raw, field)
					}
					require.False(t, gjson.GetBytes(upstream.lastBody, "input").Exists())
					after, err := json.Marshal(account)
					require.NoError(t, err)
					require.JSONEq(t, string(before), string(after))
					upstreams = append(upstreams, upstream)
				}
				require.JSONEq(t, string(upstreams[0].lastBody), string(upstreams[1].lastBody))
			})
		}
	}
}

func TestMixedResponsesShapedChatPreservesNativeProtocolAndToolsHTTP(t *testing.T) {
	for _, target := range []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", target, stream), func(t *testing.T) {
				type capturedRequest struct {
					path, authorization, apiKey string
					body                        []byte
				}
				requests := make(chan capturedRequest, 3)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					requests <- capturedRequest{r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("X-Api-Key"), body}
					protocol := APIProtocolResponses
					switch r.URL.Path {
					case "/v1/chat/completions":
						protocol = APIProtocolChatCompletions
					case "/v1/messages":
						protocol = APIProtocolAnthropic
					}
					if protocol != target {
						refusal := mixedRefusal(protocol)
						defer refusal.Body.Close()
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(refusal.StatusCode)
						_, _ = io.Copy(w, refusal.Body)
						return
					}
					if protocol != APIProtocolResponses && len(gjson.GetBytes(body, "messages").Array()) == 0 {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusBadRequest)
						_, _ = io.WriteString(w, `{"error":{"message":"messages is required","param":"messages"}}`)
						return
					}
					upstreamStream := gjson.GetBytes(body, "stream").Bool()
					wire, contentType := mixedResponsesToolSSE(), "text/event-stream"
					if protocol == APIProtocolResponses && !upstreamStream {
						wire, contentType = `{"id":"resp_1","object":"response","model":"actual","status":"completed","output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup","arguments":"{\"id\":1}"}],"usage":{"input_tokens":9,"output_tokens":4}}`, "application/json"
					}
					if protocol == APIProtocolChatCompletions {
						wire = mixedChatToolSSE()
						if !upstreamStream {
							wire, contentType = mixedChatToolJSON, "application/json"
						}
					} else if protocol == APIProtocolAnthropic {
						wire = toolAnthropicSSEStream()
					}
					w.Header().Set("Content-Type", contentType)
					_, _ = io.WriteString(w, wire)
				}))
				defer server.Close()
				account := mixedTestAccount()
				account.Credentials["base_url"] = server.URL + "/v1"
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: headerCancelUpstreamFunc(server.Client().Do)}
				body := mixedClientBody(APIProtocolResponses, stream)
				result, rec, _, err := mixedForward(t, svc, account, APIProtocolChatCompletions, body)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, "actual", result.UpstreamModel)
				require.Contains(t, rec.Body.String(), "lookup")
				if stream {
					require.Contains(t, rec.Body.String(), `"object":"chat.completion.chunk"`)
					require.Contains(t, rec.Body.String(), "data: [DONE]")
					require.NotContains(t, rec.Body.String(), "event: response.")
				} else {
					require.Equal(t, "chat.completion", gjson.Get(rec.Body.String(), "object").String())
					require.Equal(t, "lookup", gjson.Get(rec.Body.String(), "choices.0.message.tool_calls.0.function.name").String())
				}
				require.Len(t, requests, mixedExpectedAttempts(APIProtocolResponses, target))
				for i := 0; len(requests) > 0; i++ {
					req := <-requests
					require.Equal(t, protocolUpstreamEndpoint(mixedExpectedProtocolOrder(APIProtocolResponses)[i]), req.path)
					if req.path == "/v1/messages" {
						require.Equal(t, "test-secret", req.apiKey)
						require.Empty(t, req.authorization)
					} else {
						require.Equal(t, "Bearer test-secret", req.authorization)
						require.Empty(t, req.apiKey)
					}
					require.Equal(t, "actual", gjson.GetBytes(req.body, "model").String())
					require.Contains(t, string(req.body), "prior")
					require.Contains(t, string(req.body), "lookup")
					if req.path == "/v1/responses" {
						require.JSONEq(t, gjson.GetBytes(body, "input").Raw, gjson.GetBytes(req.body, "input").Raw)
					} else {
						require.NotEmpty(t, gjson.GetBytes(req.body, "messages").Array())
						require.False(t, gjson.GetBytes(req.body, "input").Exists())
					}
				}
			})
		}
	}
}
