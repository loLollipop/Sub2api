//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const mixedCCFailureUsage = `"usage":{"prompt_tokens":19,"completion_tokens":3}`

func TestMixedCCForwardSafeStreamEvidence(t *testing.T) {
	for _, ingress := range []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
		for _, rawLog := range []bool{false, true} {
			for _, failure := range []struct{ name, sse, diagnostic string }{
				{"named-error", "event: error\ndata: " + `{"model":"actual","error":{"type":"invalid_request_error","code":"unsupported_parameter","param":"reasoning_effort","message":"Unsupported parameter: reasoning_effort test-secret echo-private-prompt","request":{"messages":[{"content":"echo-private-prompt"}]}},` + mixedCCFailureUsage + "}\n\n", "code=unsupported_parameter param=reasoning_effort"},
				{"response-failed", "event: response.failed\ndata: " + `{"type":"response.failed","response":{"model":"actual","status":"failed","error":{"code":"context_length_exceeded","message":"Maximum context length exceeded test-secret echo-private-prompt"}},` + mixedCCFailureUsage + "}\n\n", "context window exceeded"},
				{"invalid-json", "data: {not-json test-secret echo-private-prompt}\n\n", "invalid_json"},
			} {
				t.Run(fmt.Sprintf("%s/raw=%t/%s", ingress, rawLog, failure.name), func(t *testing.T) {
					response := mixedHTTPResponse(200, failure.sse, true)
					response.Header.Set("x-request-id", "rid-cc-evidence")
					upstream := &httpUpstreamRecorder{responses: mixedResponsesForTarget(ingress, APIProtocolChatCompletions, response)}
					cfg := rawChatCompletionsTestConfig()
					cfg.Gateway.LogUpstreamErrorBody = rawLog
					svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
					result, rec, c, err := mixedForward(t, svc, mixedTestAccount(), ingress, mixedClientBody(ingress, true))
					require.Error(t, err)
					require.NotNil(t, result)
					require.Len(t, upstream.requests, mixedExpectedAttempts(ingress, APIProtocolChatCompletions))
					var replay *UpstreamFailoverError
					require.False(t, errors.As(err, &replay))
					for _, terminal := range []string{"response.completed", "message_stop", "[DONE]"} {
						require.NotContains(t, rec.Body.String(), terminal)
					}
					value, exists := c.Get(OpsUpstreamErrorsKey)
					require.True(t, exists, "stream failure must have an Ops event")
					events := value.([]*OpsUpstreamErrorEvent)
					require.Len(t, events, 1)
					require.Equal(t, PlatformMixed, events[0].Platform)
					require.Equal(t, int64(702), events[0].AccountID)
					require.Equal(t, "rid-cc-evidence", events[0].UpstreamRequestID)
					require.Contains(t, events[0].Message, failure.diagnostic)
					require.Contains(t, events[0].Message, "endpoint=/v1/chat/completions")
					require.Equal(t, events[0].Message, c.GetString(OpsUpstreamErrorMessageKey))
					encoded, marshalErr := json.Marshal(events)
					require.NoError(t, marshalErr)
					for _, unsafe := range []string{"test-secret", "echo-private-prompt", "request\"", "messages\""} {
						require.NotContains(t, string(encoded), unsafe)
						require.NotContains(t, rec.Body.String(), unsafe)
						require.NotContains(t, err.Error(), unsafe)
					}
					require.Empty(t, events[0].Detail)
					if failure.name != "invalid-json" {
						require.Equal(t, "actual", result.UpstreamResponseModel)
						require.Equal(t, 19, result.Usage.InputTokens)
						require.Equal(t, 3, result.Usage.OutputTokens)
					}
				})
			}
		}
	}
}

func TestMixedCCForwardEmptyAndToolOnly(t *testing.T) {
	for _, ingress := range []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
		for _, test := range []struct {
			name, payload string
			wantError     bool
		}{
			{"bare-done", "data: [DONE]\n\n", true},
			{"usage-only", "data: {\"model\":\"actual\",\"choices\":[]," + mixedCCFailureUsage + "}\n\ndata: [DONE]\n\n", true},
			{"empty-content", "data: {\"choices\":[{\"delta\":{\"content\":\"\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", true},
			{"tool-only", mixedChatToolSSE(), false},
			{"tool-without-done", strings.ReplaceAll(mixedChatToolSSE(), "data: [DONE]\n\n", ""), false},
		} {
			t.Run(ingress+"/"+test.name, func(t *testing.T) {
				upstream := &httpUpstreamRecorder{responses: mixedResponsesForTarget(ingress, APIProtocolChatCompletions, mixedHTTPResponse(200, test.payload, true))}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				result, rec, _, err := mixedForward(t, svc, mixedTestAccount(), ingress, mixedClientBody(ingress, true))
				require.Len(t, upstream.requests, mixedExpectedAttempts(ingress, APIProtocolChatCompletions))
				require.NotNil(t, result)
				if test.wantError {
					require.Error(t, err)
					for _, terminal := range []string{"response.completed", "message_stop", "[DONE]"} {
						require.NotContains(t, rec.Body.String(), terminal)
					}
					require.Nil(t, result.FirstTokenMs)
				} else {
					require.NoError(t, err)
					require.Contains(t, rec.Body.String(), "call_1")
					require.Equal(t, "actual", result.UpstreamResponseModel)
					require.Equal(t, 9, result.Usage.InputTokens)
					require.Equal(t, 4, result.Usage.OutputTokens)
				}
			})
		}
	}
}

func TestMixedCCForwardDeterministicFailureBilling(t *testing.T) {
	failure := "event: response.failed\ndata: " + `{"type":"response.failed","response":{"model":"actual","status":"failed","error":{"code":"context_length_exceeded","message":"Maximum context length exceeded"}},` + mixedCCFailureUsage + "}\n\n"
	for _, ingress := range []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
		for _, test := range []struct {
			name, prefix, errorSSE string
			nonBillable            bool
		}{
			{"before-output", "", failure, true},
			{"after-text", "data: " + `{"model":"first-model","choices":[{"delta":{"content":"hello"}}]}` + "\n\n", failure, false},
			{"after-reasoning", "data: " + `{"model":"first-model","choices":[{"delta":{"reasoning_content":"reason"}}]}` + "\n\n", failure, false},
			{"after-tool", strings.Split(mixedChatToolSSE(), "data: ")[1], failure, false},
			{"after-refusal", "data: " + `{"model":"first-model","choices":[{"delta":{"refusal":"policy"}}]}` + "\n\n", failure, false},
			{"cyber", "", strings.ReplaceAll(failure, `"context_length_exceeded"`, `"cyber_policy"`), false},
		} {
			if test.name == "after-refusal" && ingress != APIProtocolChatCompletions {
				// Unsupported converted output must stop before the later error.
				continue
			}
			t.Run(ingress+"/"+test.name, func(t *testing.T) {
				prefix := test.prefix
				if test.name == "after-tool" {
					prefix = "data: " + prefix
				}
				upstream := &httpUpstreamRecorder{responses: mixedResponsesForTarget(ingress, APIProtocolChatCompletions, mixedHTTPResponse(200, prefix+test.errorSSE, true))}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				result, rec, _, err := mixedForward(t, svc, mixedTestAccount(), ingress, mixedClientBody(ingress, true))
				require.Error(t, err)
				require.NotNil(t, result)
				require.Len(t, upstream.requests, mixedExpectedAttempts(ingress, APIProtocolChatCompletions))
				require.Equal(t, test.nonBillable, result.NonBillableUpstreamError)
				require.Equal(t, 19, result.Usage.InputTokens)
				require.Equal(t, 3, result.Usage.OutputTokens)
				require.Equal(t, "actual", result.UpstreamResponseModel)
				require.Equal(t, strings.HasPrefix(test.name, "after-") && test.name != "after-tool", result.UpstreamResponseModelConflict)
				require.NotContains(t, rec.Body.String(), "response.completed")
				require.NotContains(t, rec.Body.String(), "message_stop")
				require.NotContains(t, rec.Body.String(), "[DONE]")
			})
		}
	}
}

func TestMixedCCForwardBufferedAttributionAndFailure(t *testing.T) {
	for _, ingress := range []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
		for _, success := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/success=%t", ingress, success), func(t *testing.T) {
				payload := mixedChatToolJSON
				if !success {
					payload = `{"model":"actual","error":{"code":"context_length_exceeded","message":"Maximum context length exceeded test-secret echo-private-prompt"},` + mixedCCFailureUsage + `}`
				}
				upstream := &httpUpstreamRecorder{responses: mixedResponsesForTarget(ingress, APIProtocolChatCompletions, mixedHTTPResponse(200, payload, false))}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				result, rec, c, err := mixedForward(t, svc, mixedTestAccount(), ingress, mixedClientBody(ingress, false))
				require.NotNil(t, result)
				require.Len(t, upstream.requests, mixedExpectedAttempts(ingress, APIProtocolChatCompletions))
				require.Equal(t, "actual", result.UpstreamResponseModel)
				require.False(t, result.UpstreamResponseModelConflict)
				if success {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
					require.True(t, result.NonBillableUpstreamError)
					require.Equal(t, 19, result.Usage.InputTokens)
					require.Equal(t, 3, result.Usage.OutputTokens)
					require.NotEmpty(t, c.GetString(OpsUpstreamErrorMessageKey))
					require.NotContains(t, rec.Body.String(), "test-secret")
				}
			})
		}
	}
}

func TestMixedCCForwardReadFailureRetainsUsage(t *testing.T) {
	for _, ingress := range []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
		for _, cause := range []error{io.ErrUnexpectedEOF, context.Canceled} {
			t.Run(ingress+"/"+cause.Error(), func(t *testing.T) {
				payload := "data: {\"choices\":[]," + mixedCCFailureUsage + "}\n\n"
				response := mixedHTTPResponse(200, "", true)
				response.Body = &errTailReader{data: []byte(payload), err: cause}
				upstream := &httpUpstreamRecorder{responses: mixedResponsesForTarget(ingress, APIProtocolChatCompletions, response)}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				result, rec, _, err := mixedForward(t, svc, mixedTestAccount(), ingress, mixedClientBody(ingress, true))
				require.Error(t, err)
				require.True(t, errors.Is(err, cause))
				require.Len(t, upstream.requests, mixedExpectedAttempts(ingress, APIProtocolChatCompletions))
				require.NotNil(t, result)
				require.Equal(t, 19, result.Usage.InputTokens)
				require.Equal(t, 3, result.Usage.OutputTokens)
				require.False(t, result.NonBillableUpstreamError)
				require.NotContains(t, rec.Body.String(), "[DONE]")
			})
		}
	}
}

// Native raw Chat supports both the legacy function_call shape and refusals.
// Mixed aggregation must not turn these existing outputs into empty failures.
func TestMixedCCRawPreservesNativeOutputShapes(t *testing.T) {
	for _, test := range []struct{ name, delta, finish string }{
		{"legacy-function", `{"function_call":{"name":"lookup","arguments":"{}"}}`, "function_call"},
		{"refusal", `{"refusal":"Cannot assist with that request"}`, "stop"},
		{"reasoning-summary", `{"reasoning_summary":"Checking the request"}`, "stop"},
	} {
		for _, failAfterOutput := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/failure=%t", test.name, failAfterOutput), func(t *testing.T) {
				payload := fmt.Sprintf("data: {\"model\":\"actual\",\"choices\":[{\"delta\":%s}]}\n\n", test.delta)
				if failAfterOutput {
					payload += "event: error\ndata: " + `{"error":{"code":"context_length_exceeded","message":"Maximum context length exceeded"},` + mixedCCFailureUsage + "}\n\n"
				} else {
					payload += fmt.Sprintf("data: {\"model\":\"actual\",\"choices\":[{\"delta\":{},\"finish_reason\":%q}],%s}\n\ndata: [DONE]\n\n", test.finish, mixedCCFailureUsage)
				}
				upstream := &httpUpstreamRecorder{resp: mixedHTTPResponse(200, payload, true)}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				result, rec, _, err := mixedForward(t, svc, mixedTestAccount(), APIProtocolChatCompletions, mixedClientBody(APIProtocolChatCompletions, true))
				require.NotNil(t, result)
				require.Len(t, upstream.requests, 1)
				require.Contains(t, rec.Body.String(), test.delta)
				require.NotNil(t, result.FirstTokenMs)
				require.False(t, result.NonBillableUpstreamError, "already generated output must not be marked free")
				if failAfterOutput {
					require.Error(t, err)
					require.NotContains(t, rec.Body.String(), "[DONE]")
				} else {
					require.NoError(t, err)
					require.Contains(t, rec.Body.String(), "[DONE]")
				}
			})
		}
	}
}

func TestMixedCCConvertedUnsupportedOutputNeverCompletes(t *testing.T) {
	for _, ingress := range []string{APIProtocolResponses, APIProtocolAnthropic} {
		for _, output := range []string{
			`{"function_call":{"name":"lookup","arguments":"{}"}}`,
			`{"refusal":"Cannot assist with that request"}`,
			`{"reasoning_summary":"Checking the request"}`,
		} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", ingress, output, stream), func(t *testing.T) {
					payload := fmt.Sprintf(`{"model":"actual","choices":[{"message":%s,"finish_reason":"stop"}],%s}`, output, mixedCCFailureUsage)
					if stream {
						payload = fmt.Sprintf("data: {\"model\":\"actual\",\"choices\":[{\"delta\":%s}],%s}\n\ndata: [DONE]\n\n", output, mixedCCFailureUsage)
					}
					upstream := &httpUpstreamRecorder{responses: mixedResponsesForTarget(ingress, APIProtocolChatCompletions, mixedHTTPResponse(200, payload, stream))}
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
					result, rec, _, err := mixedForward(t, svc, mixedTestAccount(), ingress, mixedClientBody(ingress, stream))
					require.Error(t, err)
					require.Contains(t, err.Error(), "unsupported_conversion_output")
					require.NotNil(t, result)
					require.False(t, result.NonBillableUpstreamError)
					require.Equal(t, 19, result.Usage.InputTokens)
					require.Equal(t, 3, result.Usage.OutputTokens)
					require.Len(t, upstream.requests, mixedExpectedAttempts(ingress, APIProtocolChatCompletions))
					for _, terminal := range []string{"response.completed", "message_stop", "[DONE]"} {
						require.NotContains(t, rec.Body.String(), terminal)
					}
				})
			}
		}
	}
}

func TestMixedNativeResponsesFailureKeepsRequestErrorClassification(t *testing.T) {
	for _, test := range []struct {
		name, prefix, code string
		nonBillable        bool
	}{
		{"before-output", "", "context_length_exceeded", true},
		{"after-text", `{"type":"response.output_text.delta","delta":"hello"}`, "context_length_exceeded", false},
		{"after-tool", `{"type":"response.function_call_arguments.delta","delta":"{}"}`, "context_length_exceeded", false},
		{"after-refusal", `{"type":"response.refusal.delta","delta":"Cannot assist"}`, "context_length_exceeded", false},
		{"policy", "", "cyber_policy", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := ""
			if test.prefix != "" {
				payload += "data: " + test.prefix + "\n\n"
			}
			payload += fmt.Sprintf("event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"model\":\"actual\",\"error\":{\"code\":%q,\"message\":\"Maximum context length exceeded\"},\"usage\":{\"input_tokens\":19,\"output_tokens\":3}}}\n\n", test.code)
			upstream := &httpUpstreamRecorder{resp: mixedHTTPResponse(200, payload, true)}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			result, rec, _, err := mixedForward(t, svc, mixedTestAccount(), APIProtocolResponses, mixedClientBody(APIProtocolResponses, true))
			require.Error(t, err)
			require.NotNil(t, result)
			require.Equal(t, test.nonBillable, result.NonBillableUpstreamError)
			require.Equal(t, 19, result.Usage.InputTokens)
			require.Equal(t, 3, result.Usage.OutputTokens)
			require.Len(t, upstream.requests, 1)
			require.NotContains(t, rec.Body.String(), "response.completed")
		})
	}
}
