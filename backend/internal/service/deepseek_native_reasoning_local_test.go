//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func TestDeepSeekNativeReasoningReplayResponseHandlersAcrossAccounts(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, mode := range []string{"json", "stream", "sse-to-json"} {
			t.Run(fmt.Sprintf("passthrough=%t/%s", passthrough, mode), func(t *testing.T) {
				cache := &reasoningRecordingCache{}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), cache: cache}
				account := deepSeekNativeReplayAccount()
				c, downstream := deepSeekNativeReplayContext(21, "handler-thread")
				svc.prepareDeepSeekNativeReasoningReplay(c, account, []byte(`{"model":"deepseek-v4.1-flash","input":"question"}`))
				response := deepSeekNativeReplayResponse("resp_handlers", 10, 3,
					map[string]any{"type": "reasoning", "id": "rs_plain", "content": []any{map[string]string{"type": "reasoning_text", "text": "Full upstream reasoning."}}},
					deepSeekNativeReplayAnswer())
				body, contentType := string(response), "application/json"
				if mode != "json" {
					body, contentType = deepSeekNativeReplaySSE(response), "text/event-stream"
				}
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body))}
				var usage *OpenAIUsage
				var err error
				switch {
				case passthrough && mode == "stream":
					var result *openaiStreamingResultPassthrough
					result, err = svc.handleStreamingResponsePassthrough(context.Background(), resp, c, account, time.Now(), "deepseek-v4.1-flash", "deepseek-v4.1-flash")
					if result != nil {
						usage = result.usage
					}
				case passthrough:
					var result *openaiNonStreamingResultPassthrough
					result, err = svc.handleNonStreamingResponsePassthrough(context.Background(), resp, c, account, "deepseek-v4.1-flash", "deepseek-v4.1-flash")
					if result != nil {
						usage = result.usage
					}
				case mode == "stream":
					var result *openaiStreamingResult
					result, err = svc.handleStreamingResponse(context.Background(), resp, c, account, time.Now(), "deepseek-v4.1-flash", "deepseek-v4.1-flash")
					if result != nil {
						usage = result.usage
					}
				default:
					var result *openaiNonStreamingResult
					result, err = svc.handleNonStreamingResponse(context.Background(), resp, c, account, "deepseek-v4.1-flash", "deepseek-v4.1-flash")
					if result != nil {
						usage = result.usage
					}
				}
				require.NoError(t, err, downstream.Body.String())
				require.NotNil(t, usage)
				require.Equal(t, 10, usage.InputTokens)
				require.Equal(t, 3, usage.OutputTokens)
				require.NotEmpty(t, cache.snapshotSets())
				next := &OpenAIGatewayService{cache: &reasoningRecordingCache{getResp: cache.snapshotSets()}}
				nextAccount := deepSeekNativeReplayAccount()
				nextAccount.ID = account.ID + 1
				replay := []byte(`{"model":"deepseek-v4.1-flash","input":[{"type":"reasoning","id":"rs_plain","summary":[]}]}`)
				for _, keyID := range []int64{21, 22} {
					nextContext, _ := deepSeekNativeReplayContext(keyID, "handler-thread")
					out := next.prepareDeepSeekNativeReasoningReplay(nextContext, nextAccount, replay)
					if keyID == 21 {
						require.Equal(t, "Full upstream reasoning.", gjson.GetBytes(out, "input.0.content.0.text").String())
					} else {
						require.Equal(t, replay, out, "changing upstream accounts must not bypass API-key isolation")
					}
				}
			})
		}
	}
}

func TestDeepSeekNativeReasoningReplayFailedPlainAttemptDoesNotOverwrite(t *testing.T) {
	for _, terminal := range []string{"", "response.failed", "response.incomplete", "response.completed"} {
		t.Run(terminal, func(t *testing.T) {
			cache := &reasoningRecordingCache{}
			svc := &OpenAIGatewayService{cache: cache}
			account := deepSeekNativeReplayAccount()
			c, _ := deepSeekNativeReplayContext(21, "retry-thread")
			svc.prepareDeepSeekNativeReasoningReplay(c, account, []byte(`{"model":"deepseek-v4.1-flash","input":"question"}`))
			svc.cacheDeepSeekNativeReasoningBody(c, account, []byte(`{"status":"completed","output":[{"type":"reasoning","id":"rs_fixed","content":[{"type":"reasoning_text","text":"prior success"}]}]}`))
			recorder := svc.newDeepSeekNativeReasoningRecorder(c, account)
			recorder.ObserveFrame(parseOpenAISSEDataFrame([]byte(`{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"rs_fixed","content":[{"type":"reasoning_text","text":"retry reasoning"}]}}`), ""))
			if terminal != "" {
				recorder.ObserveFrame(parseOpenAISSEDataFrame([]byte(fmt.Sprintf(`{"type":%q,"response":{"output":[]}}`, terminal)), ""))
			}
			svc.cache = &reasoningRecordingCache{getResp: cache.snapshotSets()}
			out := svc.prepareDeepSeekNativeReasoningReplay(c, account, []byte(`{"model":"deepseek-v4.1-flash","input":[{"type":"reasoning","id":"rs_fixed"}]}`))
			want := "prior success"
			if terminal == "response.completed" {
				want = "retry reasoning"
			}
			require.Equal(t, want, gjson.GetBytes(out, "input.0.content.0.text").String())
		})
	}
}

func TestDeepSeekNativeReasoningReplayRejectsFailureEnvelopes(t *testing.T) {
	const output = `[{"type":"reasoning","id":"rs_fixed","content":[{"type":"reasoning_text","text":"failed attempt"}]}]`
	for _, tc := range []struct{ name, body string }{
		{"json-error", `{"error":{"message":"failed"},"output":` + output + `}`},
		{"json-hidden-error", `{"error":null,"error":{"message":"failed"},"output":` + output + `}`},
		{"json-incomplete", `{"status":"incomplete","output":` + output + `}`},
		{"json-hidden-incomplete", `{"status":"completed","status":"incomplete","output":` + output + `}`},
		{"sse-done-failed", "data: " + `{"type":"response.done","response":{"status":"failed","output":` + output + `}}` + "\n\n"},
		{"sse-done-incomplete", "data: " + `{"type":"response.done","response":{"status":"incomplete","output":` + output + `}}` + "\n\n"},
		{"sse-completed-error", "data: " + `{"type":"response.completed","response":{"status":"completed","error":{"message":"failed"},"output":` + output + `}}` + "\n\n"},
		{"sse-completed-top-error", "data: " + `{"type":"response.completed","error":{"message":"failed"},"response":{"output":` + output + `}}` + "\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := &reasoningRecordingCache{}
			svc := &OpenAIGatewayService{cache: cache}
			account := deepSeekNativeReplayAccount()
			c, _ := deepSeekNativeReplayContext(21, "error-thread")
			svc.prepareDeepSeekNativeReasoningReplay(c, account, []byte(`{"model":"deepseek-v4.1-flash","input":"question"}`))
			svc.cacheDeepSeekNativeReasoningBody(c, account, []byte(`{"status":"completed","output":[{"type":"reasoning","id":"rs_fixed","content":[{"type":"reasoning_text","text":"prior success"}]}]}`))
			before := cache.snapshotSets()
			svc.cacheDeepSeekNativeReasoningBody(c, account, []byte(tc.body))
			require.Equal(t, before, cache.snapshotSets(), "failure envelopes must not replace a successful plain item")
		})
	}
}

func TestDeepSeekNativeReasoningReplayNonStreamingSSEFraming(t *testing.T) {
	for _, data := range []string{
		`data: {"response":{"status":"completed","output":[{"type":"reasoning","id":"rs_named","content":[{"type":"reasoning_text","text":"actual"}]}]}}`,
		"data: {\"response\":{\"status\":\"completed\",\n" + `data: "output":[{"type":"reasoning","id":"rs_named","content":[{"type":"reasoning_text","text":"actual"}]}]}}`,
	} {
		t.Run(fmt.Sprint(strings.Contains(data, "\n")), func(t *testing.T) {
			cache := &reasoningRecordingCache{}
			svc := &OpenAIGatewayService{cache: cache}
			account := deepSeekNativeReplayAccount()
			c, _ := deepSeekNativeReplayContext(21, "named-event-thread")
			svc.prepareDeepSeekNativeReasoningReplay(c, account, []byte(`{"model":"deepseek-v4.1-flash","input":"question"}`))
			svc.cacheDeepSeekNativeReasoningBody(c, account, []byte("event: response.completed\n"+data+"\n\n"))
			svc.cache = &reasoningRecordingCache{getResp: cache.snapshotSets()}
			out := svc.prepareDeepSeekNativeReasoningReplay(c, account, []byte(`{"model":"deepseek-v4.1-flash","input":[{"type":"reasoning","id":"rs_named"}]}`))
			require.Equal(t, "actual", gjson.GetBytes(out, "input.0.content.0.text").String())
		})
	}
}

func TestDeepSeekNativeReasoningReplayAnonymousInputShapeContinuation(t *testing.T) {
	for _, content := range []string{`"question"`, `[{"type":"input_text","text":"question"}]`} {
		t.Run(content, func(t *testing.T) {
			cache := &reasoningRecordingCache{}
			svc := &OpenAIGatewayService{cache: cache}
			account := deepSeekNativeReplayAccount()
			c, _ := deepSeekNativeReplayContext(21, "")
			svc.prepareDeepSeekNativeReasoningReplay(c, account, []byte(`{"model":"deepseek-v4.1-flash","input":"question"}`))
			svc.cacheDeepSeekNativeReasoningBody(c, account, []byte(`{"output":[{"type":"reasoning","id":"rs_anon","content":[{"type":"reasoning_text","text":"anonymous actual"}]}]}`))
			svc.cache = &reasoningRecordingCache{getResp: cache.snapshotSets()}
			replay := []byte(`{"model":"deepseek-v4.1-flash","input":[{"role":"user","content":` + content + `},{"type":"reasoning","id":"rs_anon"}]}`)
			next, _ := deepSeekNativeReplayContext(21, "")
			out := svc.prepareDeepSeekNativeReasoningReplay(next, account, replay)
			require.Equal(t, "anonymous actual", gjson.GetBytes(out, "input.1.content.0.text").String())
			otherKey, _ := deepSeekNativeReplayContext(22, "")
			require.Equal(t, replay, svc.prepareDeepSeekNativeReasoningReplay(otherKey, account, replay))
			otherQuestion, err := sjson.SetBytes(replay, "input.0.content", "another question")
			require.NoError(t, err)
			require.Equal(t, otherQuestion, svc.prepareDeepSeekNativeReasoningReplay(next, account, otherQuestion))
		})
	}
}
