//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesMissingToolOutputsForward(t *testing.T) {
	for _, platform := range []string{PlatformOpenAI, PlatformDeepseek} {
		t.Run(platform, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"id":"resp_test","model":"actual","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)),
			}}
			svc := newOpenAIImageGenerationControlTestService(upstream)
			c, _ := newOpenAIImageGenerationControlTestContext(true, "codex_cli_rs/0.144.1")
			account := newOpenAIImageGenerationControlTestAccount()
			account.Platform = platform
			account.Credentials["api_protocol"] = APIProtocolResponses
			account.Extra = map[string]any{"openai_passthrough": true}
			body := []byte(`{"model":"actual","stream":false,"input":[{"role":"user","content":"hello"},{"type":"function_call","call_id":"interrupted","name":"lookup","arguments":"{}"},{"role":"user","content":"continue"}]}`)
			if platform == PlatformDeepseek {
				// Native DeepSeek discards this legacy field before sending upstream.
				body = []byte(strings.Replace(string(body), `"stream":false`, `"stream":false,"previous_response_id":"stale"`, 1))
			}
			result, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, upstream.lastReq)
			require.Len(t, upstream.requests, 1, "repair must not execute tools or replay the request")
			require.Equal(t, "function_call_output", gjson.GetBytes(upstream.lastBody, "input.2.type").String())
			require.Equal(t, "interrupted", gjson.GetBytes(upstream.lastBody, "input.2.call_id").String())
			require.Contains(t, gjson.GetBytes(upstream.lastBody, "input.2.output").String(), "tool_output_missing")
			require.Equal(t, "user", gjson.GetBytes(upstream.lastBody, "input.3.role").String())
			if platform == PlatformDeepseek {
				require.False(t, gjson.GetBytes(upstream.lastBody, "previous_response_id").Exists())
			}
		})
	}
}

func TestResponsesMissingToolOutputsPairing(t *testing.T) {
	for _, pair := range [][2]string{{"function_call", "function_call_output"}, {"custom_tool_call", "custom_tool_call_output"}, {"local_shell_call", "local_shell_call_output"}} {
		t.Run(pair[0], func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"input":[{"type":%q,"call_id":"missing","name":"lookup"},{"role":"user","content":"continue"}]}`, pair[0]))
			next, changed, err := repairOpenAIResponsesMissingToolOutputs(body, true)
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, pair[1], gjson.GetBytes(next, "input.1.type").String())
			if pair[0] == "local_shell_call" {
				require.Equal(t, "missing", gjson.GetBytes(next, "input.1.id").String())
				require.False(t, gjson.GetBytes(next, "input.1.call_id").Exists())
			} else {
				require.Equal(t, "missing", gjson.GetBytes(next, "input.1.call_id").String())
			}
			require.Equal(t, responsesMissingToolOutput, gjson.GetBytes(next, "input.1.output").String())
			require.Equal(t, "user", gjson.GetBytes(next, "input.2.role").String())
			again, changed, err := repairOpenAIResponsesMissingToolOutputs(next, true)
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, next, again)
		})
	}
	for _, tc := range []struct {
		name, input string
		missing     bool
	}{
		{"parallel partial", `[{"type":"function_call","call_id":"a"},{"type":"function_call","call_id":"b"},{"type":"function_call_output","call_id":"a","output":"real"}]`, true},
		{"complete", `[{"type":"function_call","call_id":"a"},{"type":"function_call_output","call_id":"a","output":"real"}]`, false},
		{"different tool kind", `[{"type":"custom_tool_call","call_id":"a"},{"type":"function_call_output","call_id":"a","output":"real"}]`, true},
		{"orphan output precedes call", `[{"type":"function_call_output","call_id":"a","output":"old"},{"type":"function_call","call_id":"a"}]`, true},
		{"old answer cannot satisfy reused ID", `[{"type":"function_call","call_id":"a"},{"type":"function_call_output","call_id":"a","output":"old"},{"type":"function_call","call_id":"a"}]`, true},
		{"numeric ID", `[{"type":"function_call","call_id":42}]`, false},
		{"missing ID", `[{"type":"function_call"}]`, false},
		{"empty ID", `[{"type":"function_call","call_id":""}]`, false},
		{"hosted tool is not a client call", `[{"type":"web_search_call","id":"ws_1"}]`, false},
		{"native shell output uses id", `[{"type":"local_shell_call","id":"fc_shell","call_id":"call_shell","status":"completed","action":{"type":"exec","command":["pwd"],"env":{}}},{"type":"local_shell_call_output","id":"call_shell","output":"{\"stdout\":\"/tmp\"}"}]`, false},
		{"missing tool search is not a string output", `[{"type":"tool_search_call","call_id":"search_1","execution":"client","arguments":{"query":"docs"}}]`, false},
		{"complete tool search", `[{"type":"tool_search_call","call_id":"search_1","execution":"client","arguments":{"query":"docs"}},{"type":"tool_search_output","call_id":"search_1","execution":"client","status":"completed","tools":[]}]`, false},
		{"message roles untouched", `[{"role":"assistant","content":"[Previous chapter] text"},{"role":"user","content":"next"}]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"input":` + tc.input + `}`)
			next, changed, err := repairOpenAIResponsesMissingToolOutputs(body, true)
			require.NoError(t, err)
			require.Equal(t, tc.missing, changed)
			if !changed {
				require.Equal(t, body, next)
			} else {
				require.Len(t, gjson.GetBytes(next, "input").Array(), len(gjson.GetBytes(body, "input").Array())+1)
			}
		})
	}
}

func TestResponsesMissingToolOutputsPreserveRawHistory(t *testing.T) {
	call := `{"type":"function_call", "call_id":"a", "namespace":"client", "arguments":"{ \"n\": 9007199254740993 }", "unknown":9007199254740993}`
	output := `{"type":"function_call_output", "call_id":"a", "output":[{"type":"input_image","image_url":"https://example.invalid/image"},{"type":"input_file","file_id":"file_1"}],"unknown":1e100}`
	assistant := `{"role":"assistant", "content":"[Previous chapter] original answer"}`
	missing := `{"type":"custom_tool_call", "call_id":"b", "input":"patch", "namespace":"tools"}`
	body := []byte(`{"model":"alias","opaque":9007199254740993,"input":[` + call + `,` + output + `,` + assistant + `,` + missing + `]}`)
	next, changed, err := repairOpenAIResponsesMissingToolOutputs(body, false)
	require.NoError(t, err)
	require.True(t, changed)
	items := gjson.GetBytes(next, "input").Array()
	for i, raw := range []string{call, output, assistant, missing} {
		require.Equal(t, raw, items[i].Raw)
	}
	require.Equal(t, "9007199254740993", gjson.GetBytes(next, "opaque").Raw)
	for _, body := range []string{`{"input":"hello"}`, `{"input":null}`, `{"input":[`, `{}`} {
		next, changed, err := repairOpenAIResponsesMissingToolOutputs([]byte(body), true)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, body, string(next))
	}
}

func TestResponsesMissingToolOutputsForwardBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, platform, protocol, path, previous string }{
		{"stored OpenAI continuation", PlatformOpenAI, APIProtocolResponses, "/v1/responses", "resp_previous"},
		{"compact", PlatformOpenAI, APIProtocolResponses, "/v1/responses/compact", ""},
		{"DeepSeek Chat", PlatformDeepseek, APIProtocolChatCompletions, "/v1/responses", ""},
		{"OpenAI forced Chat", PlatformOpenAI, APIProtocolChatCompletions, "/v1/responses", ""},
		{"DeepSeek Anthropic", PlatformDeepseek, APIProtocolAnthropic, "/v1/responses", ""},
		{"other native provider", PlatformKimi, APIProtocolResponses, "/v1/responses", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := `{"id":"resp_test","object":"response","model":"actual","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`
			if tc.protocol == APIProtocolChatCompletions {
				response = `{"id":"cc_1","object":"chat.completion","model":"actual","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`
			} else if tc.protocol == APIProtocolAnthropic {
				response = toolAnthropicSSEStream()
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(response))}}
			if tc.protocol == APIProtocolAnthropic {
				upstream.resp.Header.Set("Content-Type", "text/event-stream")
			}
			svc := newOpenAIImageGenerationControlTestService(upstream)
			c, _ := newOpenAIImageGenerationControlTestContext(true, "codex_cli_rs/0.144.1")
			c.Request.URL.Path = tc.path
			key, _ := c.Get("api_key")
			key.(*APIKey).Group.CrossProtocolConversionEnabled = true
			account := newOpenAIImageGenerationControlTestAccount()
			account.Platform = tc.platform
			account.Credentials["api_protocol"] = tc.protocol
			account.Extra = map[string]any{"openai_passthrough": true}
			if tc.platform == PlatformOpenAI && tc.protocol == APIProtocolChatCompletions {
				account.Extra[openai_compat.ExtraKeyResponsesMode] = string(openai_compat.ResponsesSupportModeForceChatCompletions)
			}
			body := []byte(fmt.Sprintf(`{"model":"actual","stream":false,"previous_response_id":%q,"input":[{"role":"user","content":"hello"},{"type":"function_call","call_id":"missing","name":"lookup","arguments":"{}"}]}`, tc.previous))
			_, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.Len(t, upstream.requests, 1)
			require.NotContains(t, string(upstream.lastBody), "tool_output_missing")
		})
	}
}

func TestResponsesMissingToolOutputsOAuthContinuation(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		t.Run(fmt.Sprintf("passthrough=%t", passthrough), func(t *testing.T) {
			response := "data: " + `{"type":"response.completed","response":{"id":"resp_test","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\ndata: [DONE]\n\n"
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(response))}}
			svc := newOpenAIImageGenerationControlTestService(upstream)
			c, _ := newOpenAIImageGenerationControlTestContext(true, "codex_cli_rs/0.144.1")
			account := newOpenAIImageGenerationControlTestAccount()
			account.Type = AccountTypeOAuth
			account.Credentials = map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account"}
			account.Extra = map[string]any{"openai_passthrough": passthrough}
			body := []byte(`{"model":"gpt-5.6-sol","stream":true,"instructions":"test","previous_response_id":"resp_previous","input":[{"type":"function_call","call_id":"missing","name":"lookup","arguments":"{}"}]}`)
			_, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.Len(t, upstream.requests, 1)
			require.Equal(t, passthrough, gjson.GetBytes(upstream.lastBody, "previous_response_id").Exists())
			require.Equal(t, !passthrough, strings.Contains(string(upstream.lastBody), "tool_output_missing"))
		})
	}
}

func TestResponsesMissingToolOutputsNativeShapesForward(t *testing.T) {
	shellCall := `{"type":"local_shell_call","id":"fc_shell","call_id":"call_shell","status":"completed","action":{"type":"exec","command":["pwd"],"env":{}}}`
	shellOutput := `{"type":"local_shell_call_output","id":"call_shell","output":"{\"stdout\":\"/tmp\"}"}`
	searchCall := `{"type":"tool_search_call","call_id":"search_1","execution":"client","arguments":{"query":"docs"}}`
	searchOutput := `{"type":"tool_search_output","call_id":"search_1","execution":"client","status":"completed","tools":[]}`
	for _, tc := range []struct {
		name, input, tools string
		repaired           bool
	}{
		{"missing shell", shellCall, `[]`, true},
		{"complete shell", shellCall + "," + shellOutput, `[]`, false},
		{"missing native tool search", searchCall, `[]`, false},
		{"complete native tool search", searchCall + "," + searchOutput, `[]`, false},
		{"lowered missing shell", shellCall, `[{"type":"local_shell"},{"type":"tool_search"}]`, true},
		{"lowered complete shell", shellCall + "," + shellOutput, `[{"type":"local_shell"},{"type":"tool_search"}]`, false},
		{"lowered missing search", searchCall, `[{"type":"tool_search"}]`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"resp_test","model":"actual","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`))}}
			svc := newOpenAIImageGenerationControlTestService(upstream)
			c, _ := newOpenAIImageGenerationControlTestContext(true, "codex_cli_rs/0.144.1")
			account := newOpenAIImageGenerationControlTestAccount()
			account.Extra = map[string]any{"openai_passthrough": true}
			_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"actual","tools":`+tc.tools+`,"input":[`+tc.input+`]}`))
			require.NoError(t, err)
			require.Len(t, upstream.requests, 1)
			require.Equal(t, tc.repaired, strings.Contains(string(upstream.lastBody), "tool_output_missing"))
			if tc.tools != `[]` {
				require.Equal(t, "function_call", gjson.GetBytes(upstream.lastBody, "input.0.type").String())
				require.Equal(t, "function_call_output", gjson.GetBytes(upstream.lastBody, "input.1.type").String())
				require.Equal(t, gjson.GetBytes(upstream.lastBody, "input.0.call_id").String(), gjson.GetBytes(upstream.lastBody, "input.1.call_id").String())
				if !tc.repaired {
					require.Equal(t, `{"stdout":"/tmp"}`, gjson.GetBytes(upstream.lastBody, "input.1.output").String())
				}
			} else if tc.repaired {
				require.Equal(t, "local_shell_call_output", gjson.GetBytes(upstream.lastBody, "input.1.type").String())
				require.Equal(t, "call_shell", gjson.GetBytes(upstream.lastBody, "input.1.id").String())
				require.False(t, gjson.GetBytes(upstream.lastBody, "input.1.call_id").Exists())
			} else {
				require.JSONEq(t, "["+tc.input+"]", gjson.GetBytes(upstream.lastBody, "input").Raw)
			}
		})
	}
}

func TestResponsesMissingToolOutputsOAuthNativeShellForward(t *testing.T) {
	shellCall := `{"type":"local_shell_call","id":"fc_shell_item","call_id":"call_shell","status":"completed","action":{"type":"exec","command":["pwd"],"env":{}},"unknown":{"keep":true,"namespace":"client.shell"}}`
	shellOutput := `{"type":"local_shell_call_output","id":"call_shell","output":"{\"stdout\":\"/tmp\",\"exit_code\":0}","unknown":{"keep":"real","namespace":"client.shell"}}`
	for _, complete := range []bool{false, true} {
		t.Run(fmt.Sprintf("complete=%t", complete), func(t *testing.T) {
			response := "data: " + `{"type":"response.completed","response":{"id":"resp_test","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\ndata: [DONE]\n\n"
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(response))}}
			svc := newOpenAIImageGenerationControlTestService(upstream)
			c, _ := newOpenAIImageGenerationControlTestContext(true, "codex_cli_rs/0.144.1")
			account := newOpenAIImageGenerationControlTestAccount()
			account.Type = AccountTypeOAuth
			account.Credentials = map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account"}
			account.Extra = map[string]any{"openai_passthrough": false}
			input := `{"role":"assistant","content":"[Previous chapter] original answer"},` + shellCall
			if complete {
				input += "," + shellOutput
			}
			input += `,{"role":"user","content":"continue"}`
			body := []byte(`{"model":"gpt-5.6-sol","stream":true,"instructions":"test","tools":[],"input":[` + input + `]}`)
			result, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, upstream.requests, 1, "native shell history must be forwarded once without executing or replaying tools")
			items := gjson.GetBytes(upstream.lastBody, "input").Array()
			require.Len(t, items, 4)
			t.Logf("forwarded native shell input: %s", gjson.GetBytes(upstream.lastBody, "input").Raw)
			require.Equal(t, items[1].Get("call_id").String(), items[2].Get("id").String(), "native shell output.id must follow the transformed call.call_id")
			require.JSONEq(t, strings.Replace(shellCall, `"call_id":"call_shell"`, `"call_id":"fc_shell"`, 1), items[1].Raw)
			require.Equal(t, "local_shell_call_output", items[2].Get("type").String())
			require.False(t, items[2].Get("call_id").Exists(), "native shell output correlates through id")
			if complete {
				require.JSONEq(t, strings.Replace(shellOutput, `"id":"call_shell"`, `"id":"fc_shell"`, 1), items[2].Raw)
			} else {
				require.Equal(t, responsesMissingToolOutput, items[2].Get("output").String())
			}
			require.JSONEq(t, `{"role":"assistant","content":"[Previous chapter] original answer"}`, items[0].Raw)
			require.JSONEq(t, `{"role":"user","content":"continue"}`, items[3].Raw)
		})
	}
}
