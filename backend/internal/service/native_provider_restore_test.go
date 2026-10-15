//go:build unit

package service

import (
	"bytes"
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

const nativeProviderChatToolJSON = `{"id":"cc_1","object":"chat.completion","model":"actual","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"id\":1}"}}]},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13,"prompt_tokens_details":{"cached_tokens":3}}}`

func nativeProviderClientBody(protocol string, stream bool) []byte {
	var fields string
	switch protocol {
	case APIProtocolResponses:
		fields = `"input":[{"type":"function_call","call_id":"prior","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"prior","output":"result"}],"tools":[{"type":"function","name":"lookup","parameters":{"type":"object","properties":{"id":{"type":"integer"}}}}]`
	case APIProtocolChatCompletions:
		fields = `"messages":[{"role":"assistant","tool_calls":[{"id":"prior","type":"function","function":{"name":"lookup","arguments":"{}"}}]},{"role":"tool","tool_call_id":"prior","content":"result"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{"id":{"type":"integer"}}}}}]`
	case APIProtocolAnthropic:
		fields = `"max_tokens":128,"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"prior","name":"lookup","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"prior","content":"result"}]}],"tools":[{"name":"lookup","input_schema":{"type":"object","properties":{"id":{"type":"integer"}}}}]`
	}
	return []byte(fmt.Sprintf(`{"model":"alias","stream":%t,%s}`, stream, fields))
}

func nativeProviderChatToolSSE() string {
	return "data: " + `{"id":"cc_1","object":"chat.completion.chunk","model":"actual","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"id\":1}"}}]},"finish_reason":null}]}` + "\n\n" +
		"data: " + `{"id":"cc_1","object":"chat.completion.chunk","model":"actual","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13,"prompt_tokens_details":{"cached_tokens":3}}}` + "\n\ndata: [DONE]\n\n"
}

func TestNativeProviderMessagesKeyRoutesPreserveModel(t *testing.T) {
	const model = "claude-opus-4-6"
	for _, platform := range []string{PlatformCline, PlatformCommandCode} {
		t.Run(platform, func(t *testing.T) {
			ctx := WithOpenAIMessagesKeyRoute(context.Background())
			group := &Group{ID: 1, Platform: platform, Status: StatusActive, Hydrated: true}
			sanitizeGroupMessagesDispatchFields(group)
			require.False(t, group.AllowMessagesDispatch)
			require.True(t, openAIMessagesKeyRouteGroupAllowed(ctx, group))
			require.Empty(t, group.ResolveMessagesDispatchModel(model))
			require.Equal(t, model, openAIMessagesKeyRouteModel(ctx, group, model))
			for _, ids := range [][]int64{{1}, {1, 2}} {
				id := int64(1)
				key := &APIKey{GroupID: &id, Group: group, RouteGroupIDs: ids}
				svc := &OpenAIGatewayService{}
				routed, err := svc.hydrateAPIKeyGroup(ctx, key, id)
				require.NoError(t, err)
				require.Equal(t, platform, routed.Group.Platform)
			}
			composite := &Group{Platform: PlatformComposite}
			resolved := WithResolvedTargetPlatform(ctx, platform)
			require.True(t, openAIMessagesKeyRouteGroupAllowed(resolved, composite))
			require.Equal(t, model, openAIMessagesKeyRouteModel(resolved, composite, model))
		})
	}
	ctx := WithOpenAIMessagesKeyRoute(context.Background())
	require.False(t, openAIMessagesKeyRouteGroupAllowed(ctx, &Group{Platform: PlatformOpenAI}))
}

func TestNativeProviderMessagesRoutesEnforceCandidatePermission(t *testing.T) {
	const model = "claude-opus-4-6"
	for _, platform := range []string{PlatformCline, PlatformCommandCode} {
		t.Run(platform, func(t *testing.T) {
			primary := &Group{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, CrossProtocolConversionEnabled: true}
			fallback := &Group{ID: 2, Platform: platform, Status: StatusActive, Hydrated: true, CrossProtocolConversionEnabled: true}
			accounts := []*Account{
				{ID: 11, Platform: PlatformOpenAI, GroupIDs: []int64{1}, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4"}}},
				{ID: 12, Platform: platform, GroupIDs: []int64{2}, Credentials: map[string]any{"model_mapping": map[string]any{model: model}}},
			}
			snapshot, _ := newSmartRouteCoreSnapshot([]*Group{primary, fallback}, accounts...)
			svc := &OpenAIGatewayService{schedulerSnapshot: snapshot}
			key := &APIKey{User: &User{}, GroupID: &primary.ID, Group: primary, RouteGroupIDs: []int64{1, 2}}
			ctx := WithResolvedTargetPlatform(WithOpenAIMessagesKeyRoute(context.Background()), platform)
			require.False(t, openAIMessagesKeyRouteGroupAllowed(ctx, primary))
			var tried []int64
			_, _, routed, err := svc.selectAlongKeyRoutes(ctx, key, []string{platform}, model,
				func(_ context.Context, groupID *int64, _ []string, requestedModel string) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
					tried = append(tried, *groupID)
					require.Equal(t, model, requestedModel)
					return &AccountSelectionResult{Account: accounts[1]}, OpenAIAccountScheduleDecision{}, nil
				})
			require.NoError(t, err)
			require.Equal(t, []int64{2}, tried)
			require.Equal(t, int64(2), *routed.GroupID)
		})
	}
}

func nativeProviderResponsesToolSSE() string {
	return strings.Join([]string{
		`event: response.created`, `data: {"type":"response.created","response":{"id":"resp_1","model":"actual","status":"in_progress","output":[]}}`, "",
		`event: response.output_item.added`, `data: {"type":"response.output_item.added","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":"","status":"in_progress"}}`, "",
		`event: response.function_call_arguments.delta`, `data: {"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_1","delta":"{\"id\":1}"}`, "",
		`event: response.function_call_arguments.done`, `data: {"type":"response.function_call_arguments.done","output_index":0,"item_id":"fc_1","arguments":"{\"id\":1}"}`, "",
		`event: response.output_item.done`, `data: {"type":"response.output_item.done","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"id\":1}","status":"completed"}}`, "",
		`event: response.completed`, `data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"actual","status":"completed","output":[{"id":"fc_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"id\":1}","status":"completed"}],"usage":{"input_tokens":9,"output_tokens":4,"total_tokens":13,"input_tokens_details":{"cached_tokens":3}}}}`, "", "",
	}, "\n")
}

// Native providers must carry tool history through their configured protocol
// in one HTTP request, with no model-name/host inference or protocol probing.
func TestNativeProviderToolRoundtripHTTP(t *testing.T) {
	for _, platform := range []string{PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax} {
		for _, protocol := range []string{APIProtocolChatCompletions, APIProtocolResponses, APIProtocolAnthropic} {
			if platform == PlatformZhipu && protocol == APIProtocolResponses {
				continue
			}
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", platform, protocol, stream), func(t *testing.T) {
					type capture struct {
						path string
						body []byte
					}
					captures := make(chan capture, 4)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						body, err := io.ReadAll(r.Body)
						if err != nil {
							w.WriteHeader(400)
							return
						}
						captures <- capture{r.URL.Path, body}
						wire, contentType := nativeProviderChatToolJSON, "application/json"
						switch protocol {
						case APIProtocolResponses:
							wire = `{"id":"resp_1","object":"response","model":"actual","status":"completed","output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup","arguments":"{}"}],"usage":{"input_tokens":9,"output_tokens":4}}`
							if stream {
								wire, contentType = nativeProviderResponsesToolSSE(), "text/event-stream"
							}
						case APIProtocolAnthropic:
							wire = `{"id":"msg_1","type":"message","role":"assistant","model":"actual","content":[{"type":"tool_use","id":"call_1","name":"lookup","input":{}}],"stop_reason":"tool_use","usage":{"input_tokens":9,"output_tokens":4}}`
							if stream {
								wire, contentType = toolAnthropicSSEStream(), "text/event-stream"
							}
						default:
							if stream {
								wire, contentType = nativeProviderChatToolSSE(), "text/event-stream"
							}
						}
						w.Header().Set("Content-Type", contentType)
						_, _ = io.WriteString(w, wire)
					}))
					defer server.Close()
					account := &Account{ID: 900, Platform: platform, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
						Credentials: map[string]any{"api_key": "native-test-key", "api_protocol": APIProtocolAdaptive,
							"api_base_urls": map[string]any{APIProtocolChatCompletions: server.URL + "/v1", APIProtocolResponses: server.URL + "/v1", APIProtocolAnthropic: server.URL + "/v1"},
							"model_mapping": map[string]any{"alias": "actual", "actual": "must-not-map-twice"}}}
					body := nativeProviderClientBody(protocol, stream)
					path := "/v1/chat/completions"
					if protocol == APIProtocolResponses {
						path = "/v1/responses"
					}
					if protocol == APIProtocolAnthropic {
						path = "/v1/messages"
					}
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
					c.Set("api_key", &APIKey{Group: &Group{Platform: platform, CrossProtocolConversionEnabled: false}})
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: headerCancelUpstreamFunc(server.Client().Do)}
					var result *OpenAIForwardResult
					var err error
					switch protocol {
					case APIProtocolResponses:
						result, err = svc.Forward(c.Request.Context(), c, account, body)
					case APIProtocolAnthropic:
						result, err = svc.ForwardAsAnthropic(c.Request.Context(), c, account, body, "", "")
					default:
						result, err = svc.ForwardAsChatCompletions(c.Request.Context(), c, account, body, "", "")
					}
					require.NoError(t, err)
					require.NotNil(t, result)
					require.Len(t, captures, 1)
					request := <-captures
					require.Equal(t, path, request.path)
					require.Equal(t, "actual", gjson.GetBytes(request.body, "model").String())
					require.Contains(t, string(request.body), "prior")
					require.Contains(t, string(request.body), "lookup")
					require.Contains(t, rec.Body.String(), "lookup")
					if protocol == APIProtocolResponses {
						require.True(t, gjson.GetBytes(request.body, "input").Exists())
					} else {
						require.True(t, gjson.GetBytes(request.body, "messages").Exists())
					}
				})
			}
		}
	}
}
