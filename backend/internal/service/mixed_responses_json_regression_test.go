//go:build unit

package service

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMixedResponsesJSONCompletionIndependentOfUsage(t *testing.T) {
	for _, status := range []string{"completed", "incomplete", "in_progress", ""} {
		for _, usage := range []bool{false, true} {
			t.Run(fmt.Sprintf("status=%s/usage=%t", status, usage), func(t *testing.T) {
				body := fmt.Sprintf(`{"id":"resp_1","object":"response","status":%q,"model":"actual","output":[{"type":"message","content":[{"type":"output_text","text":"OK"}]}]`, status)
				if usage {
					body += `,"usage":{"input_tokens":9,"output_tokens":4}`
				}
				body += "}"
				upstream := &httpUpstreamRecorder{responses: []*http.Response{mixedRefusal(APIProtocolChatCompletions), mixedHTTPResponse(200, body, false)}}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				result, rec, _, err := mixedForward(t, svc, mixedTestAccount(), APIProtocolResponses, mixedClientBody(APIProtocolResponses, false))
				require.Len(t, upstream.requests, 2)
				require.NotNil(t, result)
				if usage {
					require.Equal(t, 9, result.Usage.InputTokens)
					require.Equal(t, 4, result.Usage.OutputTokens)
				}
				if status == "completed" || status == "incomplete" {
					require.NoError(t, err)
					require.Equal(t, 200, rec.Code)
					require.Contains(t, rec.Body.String(), "OK")
				} else {
					require.Error(t, err)
					require.Equal(t, 502, rec.Code)
					require.Contains(t, rec.Body.String(), `"error"`)
				}
			})
		}
	}
}

func TestMixedMalformedResponsesTerminalNeverSucceeds(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			prefix, _, _ := mixedFailurePayload(APIProtocolResponses, true, "truncated")
			body := prefix + "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":\n\n"
			upstream := &httpUpstreamRecorder{responses: []*http.Response{mixedRefusal(APIProtocolChatCompletions), mixedHTTPResponse(200, body, true)}}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			result, rec, _, err := mixedForward(t, svc, mixedTestAccount(), APIProtocolResponses, mixedClientBody(APIProtocolResponses, stream))
			require.Error(t, err)
			require.NotNil(t, result)
			require.Len(t, upstream.requests, 2)
			require.Equal(t, 9, result.Usage.InputTokens)
			require.Equal(t, 4, result.Usage.OutputTokens)
			require.Contains(t, rec.Body.String(), `"error"`)
			require.NotContains(t, rec.Body.String(), `data: {"type":"response.completed"`)
		})
	}
}

func TestMixedNamedChatErrorNeverFinalizes(t *testing.T) {
	for _, ingress := range []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
		for _, named := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/named=%t", ingress, named), func(t *testing.T) {
				body := ""
				if named {
					body = "event: error\ndata: " + `{"message":"overloaded","usage":{"prompt_tokens":9,"completion_tokens":4}}` + "\n\n"
				} else {
					body = "data: " + `{"type":"error","message":"overloaded","usage":{"prompt_tokens":9,"completion_tokens":4}}` + "\n\n"
				}
				body += "data: [DONE]\n\n"
				upstream := &httpUpstreamRecorder{resp: mixedHTTPResponse(200, body, true)}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				result, rec, _, err := mixedForward(t, svc, mixedTestAccount(), ingress, mixedClientBody(ingress, true))
				require.Error(t, err)
				require.NotNil(t, result)
				require.Len(t, upstream.requests, 1)
				require.Equal(t, 9, result.Usage.InputTokens)
				require.Equal(t, 4, result.Usage.OutputTokens)
				require.Contains(t, rec.Body.String(), `"error"`)
				for _, terminal := range []string{"response.completed", "message_stop", "[DONE]"} {
					require.NotContains(t, rec.Body.String(), terminal)
				}
			})
		}
	}
}

func TestMixedReversedNamedChatErrorNeverFinalizes(t *testing.T) {
	for _, ingress := range []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
		for _, priorOutput := range []bool{false, true} {
			for _, finalBlank := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/output=%t/blank=%t", ingress, priorOutput, finalBlank), func(t *testing.T) {
					body := ""
					if priorOutput {
						body = "data: " + `{"id":"cc_1","model":"actual","choices":[{"index":0,"delta":{"role":"assistant","content":"partial"},"finish_reason":null}]}` + "\n\n"
					}
					body += "data: " + `{"message":"overloaded","usage":{"prompt_tokens":9,"completion_tokens":4}}` + "\nevent: error\n"
					if finalBlank {
						body += "\ndata: [DONE]\n\n"
					}
					upstream := &httpUpstreamRecorder{resp: mixedHTTPResponse(200, body, true)}
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
					result, rec, _, err := mixedForward(t, svc, mixedTestAccount(), ingress, mixedClientBody(ingress, true))
					require.Error(t, err)
					var replay *UpstreamFailoverError
					require.False(t, errors.As(err, &replay))
					require.NotNil(t, result)
					require.Len(t, upstream.requests, 1)
					require.Equal(t, 9, result.Usage.InputTokens)
					require.Equal(t, 4, result.Usage.OutputTokens)
					require.Contains(t, rec.Body.String(), `"error"`)
					if priorOutput {
						require.Contains(t, rec.Body.String(), "partial")
					}
					for _, terminal := range []string{"response.completed", "message_stop", "[DONE]"} {
						require.NotContains(t, rec.Body.String(), terminal)
					}
				})
			}
		}
	}
}

func TestMixedBufferedResponsesUsesValidatedTerminal(t *testing.T) {
	for _, first := range []string{"in_progress", "malformed", "completed"} {
		t.Run(first, func(t *testing.T) {
			body := "event: response.completed\ndata: "
			if first == "malformed" {
				body += `{"type":"response.completed","response":`
			} else {
				body += fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_first","status":%q,"output":[{"type":"message","content":[{"type":"output_text","text":"FIRST"}]}],"usage":{"input_tokens":9,"output_tokens":4}}}`, first)
			}
			body += "\n\nevent: response.completed\ndata: " + `{"type":"response.completed","response":{"id":"resp_last","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"LAST"}]}],"usage":{"input_tokens":12,"output_tokens":6}}}` + "\n\n"
			upstream := &httpUpstreamRecorder{responses: []*http.Response{mixedRefusal(APIProtocolChatCompletions), mixedHTTPResponse(200, body, true)}}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			result, rec, _, err := mixedForward(t, svc, mixedTestAccount(), APIProtocolResponses, mixedClientBody(APIProtocolResponses, false))
			require.Len(t, upstream.requests, 2)
			require.NotNil(t, result)
			if first == "completed" {
				require.NoError(t, err)
				require.Equal(t, 200, rec.Code)
				require.Contains(t, rec.Body.String(), "resp_last")
				require.Contains(t, rec.Body.String(), "LAST")
				require.NotContains(t, rec.Body.String(), "FIRST")
				require.Equal(t, 12, result.Usage.InputTokens)
				require.Equal(t, 6, result.Usage.OutputTokens)
			} else {
				require.Error(t, err)
				var replay *UpstreamFailoverError
				require.False(t, errors.As(err, &replay))
				require.Equal(t, 502, rec.Code)
				require.Contains(t, rec.Body.String(), `"error"`)
				require.NotContains(t, rec.Body.String(), "FIRST")
			}
		})
	}
}

func TestMixedPendingChatUsageSurvivesReadFailure(t *testing.T) {
	for _, ingress := range []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
		t.Run(ingress, func(t *testing.T) {
			body := "data: " + `{"choices":[],"usage":{"prompt_tokens":9,"completion_tokens":4}}` + "\n"
			response := mixedHTTPResponse(200, body, true)
			response.Body = &mixedFailedReadBody{body: strings.NewReader(body)}
			upstream := &httpUpstreamRecorder{resp: response}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			result, rec, _, err := mixedForward(t, svc, mixedTestAccount(), ingress, mixedClientBody(ingress, true))
			require.Error(t, err)
			var replay *UpstreamFailoverError
			require.False(t, errors.As(err, &replay))
			require.Len(t, upstream.requests, 1)
			require.NotNil(t, result)
			require.Equal(t, 9, result.Usage.InputTokens)
			require.Equal(t, 4, result.Usage.OutputTokens)
			require.Contains(t, rec.Body.String(), `"error"`)
			for _, terminal := range []string{"response.completed", "message_stop", "[DONE]"} {
				require.NotContains(t, rec.Body.String(), terminal)
			}
		})
	}
}

func TestMixedBufferedResponsesNamedErrorCannotBeMasked(t *testing.T) {
	for _, reversed := range []bool{false, true} {
		t.Run(fmt.Sprint(reversed), func(t *testing.T) {
			event := "event: error\n"
			data := "data: " + `{"type":"server_error","message":"overloaded","usage":{"input_tokens":9,"output_tokens":4}}` + "\n"
			body := event + data
			if reversed {
				body = data + event
			}
			body += "\nevent: response.completed\ndata: " + `{"type":"response.completed","response":{"id":"resp_last","status":"completed","output":[],"usage":{"input_tokens":9,"output_tokens":4}}}` + "\n\n"
			upstream := &httpUpstreamRecorder{responses: []*http.Response{mixedRefusal(APIProtocolChatCompletions), mixedHTTPResponse(200, body, true)}}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			result, rec, _, err := mixedForward(t, svc, mixedTestAccount(), APIProtocolResponses, mixedClientBody(APIProtocolResponses, false))
			require.Error(t, err)
			var replay *UpstreamFailoverError
			require.False(t, errors.As(err, &replay))
			require.Len(t, upstream.requests, 2)
			require.NotNil(t, result)
			require.Equal(t, 9, result.Usage.InputTokens)
			require.Equal(t, 4, result.Usage.OutputTokens)
			require.Equal(t, 502, rec.Code)
			require.Contains(t, rec.Body.String(), `"error"`)
			require.NotContains(t, rec.Body.String(), "resp_last")
		})
	}
}
