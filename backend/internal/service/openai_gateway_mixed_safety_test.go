//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type mixedFailedReadBody struct{ body io.Reader }

// Exercise each actual upstream, including fallback attempts. An HTTP 200
// stream failure must not become account failover or a fabricated completion.
func TestMixedAllUpstreamStreamFailures(t *testing.T) {
	for _, ingress := range []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
		for _, upstreamProtocol := range []string{APIProtocolChatCompletions, APIProtocolResponses, APIProtocolAnthropic} {
			for _, stream := range []bool{false, true} {
				for _, failure := range []string{"error", "first_error", "failed", "empty", "truncated", "read_error"} {
					t.Run(fmt.Sprintf("%s/to=%s/stream=%t/%s", ingress, upstreamProtocol, stream, failure), func(t *testing.T) {
						// Chat and native Messages honor stream=false; the two
						// Anthropic conversion paths and Responses bridges force SSE.
						sse := stream || upstreamProtocol == APIProtocolResponses || upstreamProtocol == APIProtocolAnthropic && ingress != APIProtocolAnthropic
						payload, input, output := mixedFailurePayload(upstreamProtocol, sse, failure)
						responses := []*http.Response{}
						order := mixedExpectedProtocolOrder(ingress)
						for _, protocol := range order {
							if protocol == upstreamProtocol {
								break
							}
							responses = append(responses, mixedRefusal(protocol))
						}
						response := mixedHTTPResponse(http.StatusOK, payload, sse)
						if failure == "read_error" {
							response.Body = &mixedFailedReadBody{body: strings.NewReader(payload)}
						}
						responses = append(responses, response)
						attempts := len(responses)
						upstream := &httpUpstreamRecorder{responses: responses}
						svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
						result, rec, _, err := mixedForward(t, svc, mixedTestAccount(), ingress, mixedClientBody(ingress, stream))
						require.Error(t, err)
						var replay *UpstreamFailoverError
						require.False(t, errors.As(err, &replay), "HTTP 200 must not replay: %v", err)
						require.Len(t, upstream.requests, attempts)
						require.Contains(t, rec.Body.String(), `"error"`)
						require.NotContains(t, rec.Body.String(), "response.completed")
						require.NotContains(t, rec.Body.String(), "message_stop")
						require.NotContains(t, rec.Body.String(), "[DONE]")
						if !stream {
							require.Equal(t, http.StatusBadGateway, rec.Code)
							require.NotContains(t, rec.Body.String(), `"object":"chat.completion"`)
							require.NotContains(t, rec.Body.String(), `"status":"completed"`)
						}
						require.NotNil(t, result, "partial usage result must survive errors")
						require.Equal(t, input, result.Usage.InputTokens)
						require.Equal(t, output, result.Usage.OutputTokens)
					})
				}
			}
		}
	}
}

func mixedFailurePayload(protocol string, sse bool, failure string) (string, int, int) {
	if failure == "empty" {
		return "", 0, 0
	}
	if !sse {
		usage := `"usage":{"prompt_tokens":9,"completion_tokens":4}`
		if protocol == APIProtocolAnthropic {
			usage = `"usage":{"input_tokens":9,"output_tokens":4}`
		}
		if failure == "error" || failure == "first_error" || failure == "failed" {
			return `{` + usage + `,"error":{"type":"api_error","message":"Upstream overloaded"}}`, 9, 4
		}
		if protocol == APIProtocolAnthropic {
			return `{` + usage + `,"id":"msg_1","type":"message","model":"actual","content":[]}`, 9, 4
		}
		return `{` + usage + `,"id":"cc_1","choices":[{"index":0,"message":{"role":"assistant","content":"partial"},"finish_reason":null}]}`, 9, 4
	}
	prefix := ""
	switch protocol {
	case APIProtocolChatCompletions:
		prefix = "data: " + `{"id":"cc_1","choices":[],"usage":{"prompt_tokens":9,"completion_tokens":4}}` + "\n\n"
	case APIProtocolResponses:
		prefix = "event: response.created\ndata: " + `{"type":"response.created","response":{"id":"resp_1","model":"actual","status":"in_progress","output":[],"usage":{"input_tokens":9,"output_tokens":4}}}` + "\n\n"
	case APIProtocolAnthropic:
		prefix = "event: message_start\ndata: " + `{"type":"message_start","message":{"id":"msg_1","type":"message","model":"actual","content":[],"usage":{"input_tokens":9,"output_tokens":0}}}` + "\n\nevent: message_delta\ndata: " + `{"type":"message_delta","usage":{"output_tokens":4}}` + "\n\n"
	}
	if failure == "first_error" {
		usage := `"usage":{"input_tokens":9,"output_tokens":4}`
		if protocol == APIProtocolChatCompletions {
			usage = `"usage":{"prompt_tokens":9,"completion_tokens":4}`
		}
		return "event: error\ndata: {\"type\":\"error\"," + usage + `,"error":{"type":"api_error","code":"server_error","message":"Upstream overloaded"}}` + "\n\n", 9, 4
	}
	if failure == "error" || failure == "failed" {
		if protocol == APIProtocolResponses && failure == "failed" {
			prefix += "event: response.failed\ndata: " + `{"type":"response.failed","response":{"id":"resp_1","status":"failed","error":{"code":"server_error","message":"Upstream overloaded"},"usage":{"input_tokens":9,"output_tokens":4}}}` + "\n\n"
		} else {
			prefix += "event: error\ndata: " + `{"type":"error","error":{"type":"api_error","code":"server_error","message":"Upstream overloaded"}}` + "\n\n"
		}
	}
	return prefix, 9, 4
}

func TestMixedEmptyChatCompletionNeverAccountFailsOver(t *testing.T) {
	for _, upstreamProtocol := range []string{APIProtocolChatCompletions, APIProtocolResponses} {
		t.Run(upstreamProtocol, func(t *testing.T) {
			responses := []*http.Response{}
			payload := "data: " + `{"id":"cc_empty","choices":[{"index":0,"delta":{"content":""},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n"
			if upstreamProtocol == APIProtocolResponses {
				responses = append(responses, mixedRefusal(APIProtocolChatCompletions))
				payload = "event: response.completed\ndata: " + `{"type":"response.completed","response":{"id":"resp_empty","status":"completed","output":[]}}` + "\n\n"
			}
			responses = append(responses, mixedHTTPResponse(http.StatusOK, payload, true))
			attempts := len(responses)
			upstream := &httpUpstreamRecorder{responses: responses}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			result, rec, _, err := mixedForward(t, svc, mixedTestAccount(), APIProtocolChatCompletions, largeRawChatCompletionsBody())
			require.Error(t, err)
			var replay *UpstreamFailoverError
			require.False(t, errors.As(err, &replay))
			require.Len(t, upstream.requests, attempts)
			require.NotNil(t, result)
			require.Contains(t, rec.Body.String(), `"error"`)
			require.NotContains(t, rec.Body.String(), "[DONE]")
		})
	}
}

func TestMixedHTTPFailuresStillAllowAccountFailover(t *testing.T) {
	for _, ingress := range []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
		for _, target := range []string{APIProtocolChatCompletions, APIProtocolResponses, APIProtocolAnthropic} {
			for _, status := range []int{http.StatusUnauthorized, http.StatusTooManyRequests} {
				t.Run(fmt.Sprintf("%s/to=%s/status=%d", ingress, target, status), func(t *testing.T) {
					order := mixedExpectedProtocolOrder(ingress)
					responses := []*http.Response{}
					for _, protocol := range order {
						if protocol == target {
							break
						}
						responses = append(responses, mixedRefusal(protocol))
					}
					responses = append(responses, mixedHTTPResponse(status, `{"error":{"type":"api_error","message":"Upstream request denied"}}`, false))
					attempts := len(responses)
					upstream := &httpUpstreamRecorder{responses: responses}
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
					_, _, _, err := mixedForward(t, svc, mixedTestAccount(), ingress, mixedClientBody(ingress, true))
					var failover *UpstreamFailoverError
					require.ErrorAs(t, err, &failover)
					require.Equal(t, status, failover.StatusCode)
					require.Len(t, upstream.requests, attempts)
				})
			}
		}
	}
}

func TestMixedResponsesDoneWithoutTerminalNeverSucceeds(t *testing.T) {
	for _, ingress := range []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", ingress, stream), func(t *testing.T) {
				payload, _, _ := mixedFailurePayload(APIProtocolResponses, true, "truncated")
				responses := mixedResponsesForTarget(ingress, APIProtocolResponses, mixedHTTPResponse(200, payload+"data: [DONE]\n\n", true))
				attempts := len(responses)
				upstream := &httpUpstreamRecorder{responses: responses}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				result, rec, _, err := mixedForward(t, svc, mixedTestAccount(), ingress, mixedClientBody(ingress, stream))
				require.Error(t, err)
				var replay *UpstreamFailoverError
				require.False(t, errors.As(err, &replay))
				require.Len(t, upstream.requests, attempts)
				require.NotNil(t, result)
				require.Equal(t, 9, result.Usage.InputTokens)
				require.Contains(t, rec.Body.String(), `"error"`)
				require.NotContains(t, rec.Body.String(), "response.completed")
				require.NotContains(t, rec.Body.String(), "message_stop")
				require.NotContains(t, rec.Body.String(), "[DONE]")
			})
		}
	}
}

func (b *mixedFailedReadBody) Read(p []byte) (int, error) {
	n, err := b.body.Read(p)
	if err == io.EOF {
		return n, io.ErrUnexpectedEOF
	}
	return n, err
}

func TestMixedPartialStreamNeverFinalizesAndCompleteWithoutDoneWorks(t *testing.T) {
	for _, protocol := range []string{APIProtocolResponses, APIProtocolAnthropic, APIProtocolChatCompletions} {
		for _, complete := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/complete=%t", protocol, complete), func(t *testing.T) {
				payload := "data: " + `{"id":"cc_1","model":"actual","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}]}` + "\n\n"
				if complete {
					// A finish_reason is terminal evidence even without a DONE sentinel.
					payload = strings.ReplaceAll(mixedChatToolSSE(), "data: [DONE]\n\n", "")
				} else {
					// Usage alone is not completion evidence for Mixed.
					payload += "data: " + `{"choices":[],"usage":{"prompt_tokens":9,"completion_tokens":4}}` + "\n\n"
				}
				upstream := &httpUpstreamRecorder{responses: mixedResponsesForTarget(protocol, APIProtocolChatCompletions, mixedHTTPResponse(200, payload, true))}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				result, rec, _, err := mixedForward(t, svc, mixedTestAccount(), protocol, mixedClientBody(protocol, true))
				require.Len(t, upstream.requests, mixedExpectedAttempts(protocol, APIProtocolChatCompletions))
				if complete {
					require.NoError(t, err)
					require.NotNil(t, result)
					return
				}
				require.ErrorIs(t, err, ErrOpenAIUpstreamStreamTruncated)
				require.Contains(t, rec.Body.String(), `"error"`)
				require.NotContains(t, rec.Body.String(), "response.completed")
				require.NotContains(t, rec.Body.String(), "message_stop")
				require.NotContains(t, rec.Body.String(), "[DONE]")
				require.NotNil(t, result)
				require.Equal(t, 9, result.Usage.InputTokens)
			})
		}
	}
}

func TestMixedCancellationClosesAllProtocolAttempts(t *testing.T) {
	for _, protocol := range []string{APIProtocolResponses, APIProtocolAnthropic, APIProtocolChatCompletions} {
		for _, target := range []string{APIProtocolChatCompletions, APIProtocolResponses, APIProtocolAnthropic} {
			for _, mode := range []string{"header", "error_body", "json_body", "sse_body"} {
				t.Run(fmt.Sprintf("%s/to=%s/%s", protocol, target, mode), func(t *testing.T) {
					arrived, serverDone := make(chan struct{}), make(chan struct{})
					var calls atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						_, _ = io.Copy(io.Discard, r.Body)
						if r.URL.Path != protocolUpstreamEndpoint(target) {
							refused := APIProtocolChatCompletions
							if r.URL.Path == "/v1/responses" {
								refused = APIProtocolResponses
							}
							if r.URL.Path == "/v1/messages" {
								refused = APIProtocolAnthropic
							}
							resp := mixedRefusal(refused)
							w.WriteHeader(resp.StatusCode)
							_, _ = io.Copy(w, resp.Body)
							return
						}
						if mode != "header" {
							w.Header().Set("Content-Type", "application/json")
							status := http.StatusOK
							if mode == "sse_body" {
								w.Header().Set("Content-Type", "text/event-stream")
							}
							if mode == "error_body" {
								status = http.StatusNotFound
							}
							w.WriteHeader(status)
							w.(http.Flusher).Flush()
						}
						close(arrived)
						<-r.Context().Done()
						close(serverDone)
					}))
					t.Cleanup(func() { server.CloseClientConnections(); server.Close() })
					targetURL, err := url.Parse(server.URL)
					require.NoError(t, err)
					upstream := &headerCancelHTTPUpstream{client: server.Client(), target: targetURL}
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					body := mixedClientBody(protocol, mode == "sse_body")
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, protocolUpstreamEndpoint(protocol), strings.NewReader(string(body))).WithContext(ctx)
					c.Set("api_key", &APIKey{Group: &Group{Platform: PlatformMixed}})
					done := make(chan error, 1)
					go func() {
						var forwardErr error
						detached := context.WithoutCancel(ctx)
						switch protocol {
						case APIProtocolResponses:
							_, forwardErr = svc.Forward(detached, c, mixedTestAccount(), body)
						case APIProtocolChatCompletions:
							_, forwardErr = svc.ForwardAsChatCompletions(detached, c, mixedTestAccount(), body, "", "")
						case APIProtocolAnthropic:
							_, forwardErr = svc.ForwardAsAnthropic(detached, c, mixedTestAccount(), body, "", "")
						}
						done <- forwardErr
					}()
					select {
					case <-arrived:
					case err := <-done:
						t.Fatalf("did not reach blocking attempt: %v", err)
					case <-time.After(2 * time.Second):
						t.Fatal("upstream did not arrive")
					}
					before := calls.Load()
					cancel()
					select {
					case err := <-done:
						require.ErrorIs(t, err, context.Canceled)
					case <-time.After(2 * time.Second):
						t.Fatal("canceled forwarding did not terminate")
					}
					select {
					case <-serverDone:
					case <-time.After(time.Second):
						t.Fatal("upstream body/request was not released")
					}
					require.Equal(t, before, calls.Load(), "cancellation must not replay")
				})
			}
		}
	}
}
func (*mixedFailedReadBody) Close() error { return nil }

func TestMixedIncompleteRefusalNeverReplays(t *testing.T) {
	for _, protocol := range []string{APIProtocolResponses, APIProtocolAnthropic, APIProtocolChatCompletions} {
		t.Run(protocol, func(t *testing.T) {
			resp := mixedRefusal(protocol)
			data, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			resp.Body = &mixedFailedReadBody{body: strings.NewReader(string(data))}
			upstream := &httpUpstreamRecorder{responses: []*http.Response{resp, mixedHTTPResponse(200, mixedChatToolJSON, false)}}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			_, _, _, err = mixedForward(t, svc, mixedTestAccount(), protocol, mixedClientBody(protocol, false))
			require.ErrorIs(t, err, io.ErrUnexpectedEOF)
			require.Len(t, upstream.requests, 1)
		})
	}
}

func TestMixedFullEndpointBasesSwitchPaths(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
	for _, prefix := range []string{"", "/vendor", "/vendor/v4"} {
		for _, source := range []string{"/v1/chat/completions", "/v1/responses", "/v1/messages"} {
			t.Run(prefix+source, func(t *testing.T) {
				account := mixedTestAccount()
				// A versioned vendor base has its own version, without an extra v1.
				configured := prefix + source
				expectedPrefix := prefix + "/v1"
				if strings.HasSuffix(prefix, "/v4") {
					configured = prefix + strings.TrimPrefix(source, "/v1")
					expectedPrefix = prefix
				}
				account.Credentials["base_url"] = "http://upstream.example" + configured
				chat, err := svc.openAIChatCompletionsTargetURL(account)
				require.NoError(t, err)
				require.Equal(t, "http://upstream.example"+expectedPrefix+"/chat/completions", chat)
				require.Equal(t, "http://upstream.example"+expectedPrefix+"/responses", buildOpenAIResponsesURL(account.GetOpenAIBaseURL()))
				messages, err := svc.nativeAnthropicTargetURL(account)
				require.NoError(t, err)
				require.Equal(t, "http://upstream.example"+expectedPrefix+"/messages", messages)
			})
		}
	}
}

func TestMixedUnsupportedRequestsReturnClientError(t *testing.T) {
	for _, protocol := range []string{APIProtocolResponses, APIProtocolAnthropic, APIProtocolChatCompletions} {
		for _, field := range []string{`"audio":{}`, `"modalities":["audio"]`, `"context_management":[]`, `"web_search_options":{}`, `"tools":[{"type":"web_search_20250305"}]`, `"tools":[{"type":"web_search"}]`, `"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://image.example/a"}}]}]`, `"input":[{"role":"user","content":[{"type":"input_audio","data":"AA"}]}]`} {
			t.Run(fmt.Sprintf("%s/%s", protocol, field), func(t *testing.T) {
				upstream := &httpUpstreamRecorder{err: errors.New("must not send")}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				_, rec, _, err := mixedForward(t, svc, mixedTestAccount(), protocol, []byte(`{"model":"alias",`+field+`}`))
				require.Error(t, err)
				require.Equal(t, http.StatusBadRequest, rec.Code)
				require.Contains(t, rec.Body.String(), `"invalid_request_error"`)
				require.Empty(t, upstream.requests)
			})
		}
	}
}

func TestMixedForwardDisablesRedirectsAllProtocols(t *testing.T) {
	for _, protocol := range []string{APIProtocolResponses, APIProtocolAnthropic, APIProtocolChatCompletions} {
		order := mixedExpectedProtocolOrder(protocol)
		upstream := &httpUpstreamRecorder{responses: []*http.Response{mixedRefusal(order[0]), mixedRefusal(order[1]), mixedRefusal(order[2])}}
		svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
		_, _, _, _ = mixedForward(t, svc, mixedTestAccount(), protocol, mixedClientBody(protocol, false))
		require.Len(t, upstream.requests, 3)
		for _, req := range upstream.requests {
			require.True(t, HTTPUpstreamRedirectsDisabled(req.Context()), req.URL.String())
		}
	}
	account := mixedTestAccount()
	req, err := buildOpenAIAPIKeyModelsRequest(context.Background(), account, func(s string) (string, error) { return s, nil })
	require.NoError(t, err)
	require.True(t, HTTPUpstreamRedirectsDisabled(req.Context()))
}
