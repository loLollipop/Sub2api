//go:build unit

package service

import (
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// The aggregation entry must use the same existing DeepSeek history cleanup
// as native forwarding. This is not evidence of any particular live rejection.
func TestMixedDeepSeekHistoryCleanupBeforeChatConversion(t *testing.T) {
	for _, model := range []string{"deepseek-v4-pro-0813", "glm-5.3"} {
		for _, stream := range []bool{false, true} {
			for _, distinctID := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stream=%t/distinct=%t", model, stream, distinctID), func(t *testing.T) {
					secondID := "call_first"
					if distinctID {
						secondID = "call_second"
					}
					body := []byte(fmt.Sprintf(`{"model":%q,"stream":%t,"input":[
					 {"type":"function_call","call_id":"call_first","name":"lookup","arguments":"{\"id\":1}"},
					 {"type":"function_call_output","call_id":"call_first","output":"first-result"},
					 {"type":"function_call","call_id":%q,"name":"lookup","arguments":"{\"id\":1}"},
					 {"type":"function_call_output","call_id":%q,"output":"later-result"}
					],"tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}`, model, stream, secondID, secondID))
					upstreamBody := mixedChatToolJSON
					if stream {
						upstreamBody = mixedChatToolSSE()
					}
					upstream := &httpUpstreamRecorder{responses: mixedResponsesForTarget(APIProtocolResponses, APIProtocolChatCompletions, mixedHTTPResponse(200, upstreamBody, stream))}
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
					account := mixedTestAccount()
					account.Credentials["model_mapping"] = map[string]any{model: model}
					_, _, _, err := mixedForward(t, svc, account, APIProtocolResponses, body)
					require.NoError(t, err)
					require.Len(t, upstream.requests, 2)
					require.Equal(t, "/v1/responses", upstream.requests[0].URL.Path)
					require.Equal(t, "/v1/chat/completions", upstream.requests[1].URL.Path)
					outbound, err := io.ReadAll(upstream.requests[1].Body)
					require.NoError(t, err)
					calls, outputs := 0, 0
					var resultTexts []string
					for _, message := range gjson.GetBytes(outbound, "messages").Array() {
						calls += len(message.Get("tool_calls").Array())
						if message.Get("role").String() == "tool" {
							outputs++
							resultTexts = append(resultTexts, message.Get("content").String())
						}
					}
					if model == "deepseek-v4-pro-0813" && !distinctID {
						require.Equal(t, 1, calls)
						require.Equal(t, 1, outputs)
						require.Equal(t, []string{"first-result"}, resultTexts)
					} else {
						require.Equal(t, 2, calls, "do not dedupe identical arguments with distinct IDs or change non-DeepSeek behavior")
						require.Equal(t, 2, outputs)
						if distinctID {
							require.Equal(t, []string{"first-result", "later-result"}, resultTexts)
						}
					}
				})
			}
		}
	}
}
