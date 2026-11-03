//go:build unit

package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDeepSeekBackportSummaryNeverPopulatesReplayCache(t *testing.T) {
	cache := &reasoningRecordingCache{}
	svc := &OpenAIGatewayService{cache: cache}
	summary := []apicompat.ResponsesSummary{{Type: "summary_text", Text: "display"}}
	svc.cacheReasoningItem(&apicompat.ResponsesOutput{Type: "reasoning", ID: "summary", Summary: summary})
	require.Empty(t, cache.snapshotSets())
	svc.cacheReasoningItem(&apicompat.ResponsesOutput{Type: "reasoning", ID: "genuine", Summary: summary, Content: []apicompat.ResponsesContentPart{{Type: "reasoning_text", Text: "  real  "}}})
	require.Equal(t, map[string]string{"genuine": "  real  "}, cache.snapshotSets())
}

func TestDeepSeekBackportNative400IsSingleTransparentRequest(t *testing.T) {
	for _, response := range []string{
		`{"error":{"code":"invalid_encrypted_content","message":"invalid encrypted content"}}`,
		`{"error":{"code":"unknown_parameter","message":"Unknown parameter: 'max_output_tokens'.","param":"max_output_tokens"}}`,
		`{"error":{"code":"invalid_request_error","message":"The reasoning_content in thinking mode must be passed back"}}`,
	} {
		for _, protocol := range []string{APIProtocolResponses, APIProtocolAdaptive} {
			t.Run(protocol+"/"+gjson.Get(response, "error.code").String(), func(t *testing.T) {
				body := []byte(`{"model":"deepseek-flash","stream":false,"store":true,"previous_response_id":"resp_previous","max_output_tokens":256,"text":{"verbosity":"high"},"reasoning":{"effort":"max","summary":"detailed","generate_summary":true},"input":[{"type":"reasoning","id":"rs_unknown","summary":[{"type":"summary_text","text":"display"}],"encrypted_content":"opaque"},{"type":"function_call","id":"fc_original","call_id":"call_a","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"call_a","output":"first"},{"type":"function_call","id":"fc_duplicate","call_id":"call_a","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"call_a","output":"second"}]}`)
				account := deepSeekNativeReplayAccount()
				account.Credentials["api_protocol"] = protocol
				upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(response))}}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), cache: &reasoningRecordingCache{}, httpUpstream: upstream}
				c, _ := deepSeekNativeReplayContext(21, "native-400")
				c.Request.Body = io.NopCloser(bytes.NewReader(body))
				_, err := svc.Forward(context.Background(), c, account, body)
				require.Error(t, err)
				require.Len(t, upstream.requests, 1, "native protocol errors must not cause a modified retry or Chat fallback")
				require.JSONEq(t, string(body), string(upstream.lastBody))
				require.Equal(t, body, RewriteInboundDeepSeekCallIDs(PlatformDeepseek, "alias", body))
			})
		}
	}
}

func TestDeepSeekBackportAccountGatedCrossProtocolRequest(t *testing.T) {
	for _, protocol := range []string{APIProtocolResponses, APIProtocolAnthropic, APIProtocolChatCompletions} {
		for _, disabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/disabled=%t", protocol, disabled), func(t *testing.T) {
				account := deepSeekNativeReplayAccount()
				account.Credentials["api_protocol"] = protocol
				account.Credentials["model_mapping"] = map[string]any{"public-alias": "deepseek-flash"}
				thinking := "enabled"
				if disabled {
					thinking = "disabled"
				}
				body := []byte(fmt.Sprintf(`{"model":"public-alias","stream":false,"max_tokens":17,"thinking":{"type":%q},"output_config":{"effort":"max"},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"genuine","signature":""},{"type":"text","text":"answer"}]},{"role":"user","content":"continue"}]}`, thinking))
				wire := `{"id":"resp_ok","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`
				contentType := "application/json"
				if protocol == APIProtocolAnthropic {
					wire = `{"id":"msg_ok","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
				} else if protocol == APIProtocolChatCompletions {
					wire = `{"id":"chat_ok","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`
				}
				upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(wire))}}
				cfg := rawChatCompletionsTestConfig()
				cfg.Gateway.MinOutputTokens = 512
				svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
				c, rec := deepSeekNativeReplayContext(21, "conversion")
				c.Request.Body = io.NopCloser(bytes.NewReader(body))
				_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
				require.NoError(t, err, rec.Body.String())
				require.Len(t, upstream.requests, 1)
				out := upstream.lastBody
				if protocol == APIProtocolResponses {
					want := "max"
					if disabled {
						want = "none"
					}
					require.Equal(t, want, gjson.GetBytes(out, "reasoning.effort").String())
					require.Equal(t, int64(512), gjson.GetBytes(out, "max_output_tokens").Int())
					require.Contains(t, string(out), `"reasoning_text"`)
				} else {
					require.Equal(t, thinking, gjson.GetBytes(out, "thinking.type").String())
					if protocol == APIProtocolChatCompletions {
						want := "max"
						if disabled {
							want = ""
						}
						require.Equal(t, want, gjson.GetBytes(out, "reasoning_effort").String())
					}
				}
			})
		}
	}
}
