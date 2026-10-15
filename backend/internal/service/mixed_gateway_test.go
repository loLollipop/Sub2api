//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

func mixedTestAccount() *Account {
	return &Account{ID: 702, Platform: PlatformMixed, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"base_url": "http://upstream.example/v1", "api_key": "test-secret", "model_mapping": map[string]any{"alias": "actual", "actual": "wrong-double-mapping"}},
		Extra:       map[string]any{"openai_responses_supported": true, "openai_passthrough": true}}
}

func mixedHTTPResponse(status int, body string, stream bool) *http.Response {
	ct := "application/json"
	if stream {
		ct = "text/event-stream"
	}
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{ct}}, Body: io.NopCloser(strings.NewReader(body))}
}

func mixedRefusal(protocol string) *http.Response {
	message := map[string]string{APIProtocolChatCompletions: "Chat Completions protocol is not supported for this model", APIProtocolResponses: "model does not support openai responses protocol", APIProtocolAnthropic: "Anthropic messages protocol is not supported for this model"}[protocol]
	b, _ := json.Marshal(map[string]any{"error": map[string]string{"code": "model_not_available", "message": message}})
	return mixedHTTPResponse(404, string(b), false)
}

// These expectations are deliberately independent of the production dispatcher.
func mixedExpectedProtocolOrder(ingress string) []string {
	switch ingress {
	case APIProtocolResponses:
		return []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic}
	case APIProtocolAnthropic:
		return []string{APIProtocolAnthropic, APIProtocolChatCompletions, APIProtocolResponses}
	default:
		return []string{APIProtocolChatCompletions, APIProtocolResponses, APIProtocolAnthropic}
	}
}

// Supply real HTTP refusals for every preceding protocol. mixedForward always
// enters the production dispatcher, including tests targeting a converted path.
func mixedResponsesForTarget(ingress, target string, response *http.Response) []*http.Response {
	responses := []*http.Response{}
	for _, protocol := range mixedExpectedProtocolOrder(ingress) {
		if protocol == target {
			return append(responses, response)
		}
		responses = append(responses, mixedRefusal(protocol))
	}
	panic("unknown Mixed test target")
}

func mixedExpectedAttempts(ingress, target string) int {
	for i, protocol := range mixedExpectedProtocolOrder(ingress) {
		if protocol == target {
			return i + 1
		}
	}
	panic("unknown Mixed test target")
}

const mixedChatToolJSON = `{"id":"cc_1","object":"chat.completion","model":"actual","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"id\":1}"}}]},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13,"prompt_tokens_details":{"cached_tokens":3}}}`

func mixedChatToolSSE() string {
	return "data: " + `{"id":"cc_1","object":"chat.completion.chunk","model":"actual","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"id\":1}"}}]},"finish_reason":null}]}` + "\n\n" +
		"data: " + `{"id":"cc_1","object":"chat.completion.chunk","model":"actual","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13,"prompt_tokens_details":{"cached_tokens":3}}}` + "\n\ndata: [DONE]\n\n"
}

func mixedClientBody(protocol string, stream bool) []byte {
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

func mixedForward(t *testing.T, svc *OpenAIGatewayService, account *Account, protocol string, body []byte) (*OpenAIForwardResult, *httptest.ResponseRecorder, *gin.Context, error) {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, protocolUpstreamEndpoint(protocol), bytes.NewReader(body))
	c.Set("api_key", &APIKey{Group: &Group{Platform: PlatformMixed}})
	var result *OpenAIForwardResult
	var err error
	switch protocol {
	case APIProtocolResponses:
		result, err = svc.Forward(c.Request.Context(), c, account, body)
	case APIProtocolChatCompletions:
		result, err = svc.ForwardAsChatCompletions(c.Request.Context(), c, account, body, "", "")
	case APIProtocolAnthropic:
		result, err = svc.ForwardAsAnthropic(c.Request.Context(), c, account, body, "", "")
	}
	return result, recorder, c, err
}

func TestMixedChatFallbackAllIngressToolsAndUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, protocol := range []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", protocol, stream), func(t *testing.T) {
				account := mixedTestAccount()
				before, _ := json.Marshal(account)
				upstreamBody := mixedChatToolJSON
				if stream {
					upstreamBody = mixedChatToolSSE()
				}
				upstream := &httpUpstreamRecorder{responses: mixedResponsesForTarget(protocol, APIProtocolChatCompletions, mixedHTTPResponse(200, upstreamBody, stream))}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				result, recorder, c, err := mixedForward(t, svc, account, protocol, mixedClientBody(protocol, stream))
				require.NoError(t, err)
				require.Len(t, upstream.requests, mixedExpectedAttempts(protocol, APIProtocolChatCompletions))
				require.Equal(t, "/v1/chat/completions", upstream.lastReq.URL.Path)
				require.Equal(t, "/v1/chat/completions", GetActualOpenAIUpstreamEndpoint(c))
				require.Equal(t, "Bearer test-secret", upstream.lastReq.Header.Get("Authorization"))
				require.Equal(t, "actual", gjson.GetBytes(upstream.lastBody, "model").String())
				require.Contains(t, string(upstream.lastBody), "prior")
				require.Contains(t, string(upstream.lastBody), "result")
				require.Contains(t, recorder.Body.String(), "call_1")
				require.Contains(t, recorder.Body.String(), "lookup")
				require.Equal(t, "actual", result.UpstreamModel)
				require.Equal(t, 9, result.Usage.InputTokens)
				require.Equal(t, 4, result.Usage.OutputTokens)
				require.Equal(t, 3, result.Usage.CacheReadInputTokens)
				if protocol == APIProtocolAnthropic {
					require.Contains(t, recorder.Body.String(), "tool_use")
				}
				if protocol == APIProtocolResponses {
					require.Contains(t, recorder.Body.String(), "function_call")
				}
				after, _ := json.Marshal(account)
				require.JSONEq(t, string(before), string(after))
			})
		}
	}
}

func TestMixedFallbackOrderBoundedAndMapsOriginalOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, protocol := range []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
		t.Run(protocol, func(t *testing.T) {
			order := mixedExpectedProtocolOrder(protocol)
			upstream := &httpUpstreamRecorder{responses: []*http.Response{mixedRefusal(order[0]), mixedRefusal(order[1]), mixedRefusal(order[2])}}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			account := mixedTestAccount()
			_, _, c, err := mixedForward(t, svc, account, protocol, mixedClientBody(protocol, false))
			require.Error(t, err)
			require.Len(t, upstream.requests, 3)
			for i, req := range upstream.requests {
				require.Equal(t, protocolUpstreamEndpoint(order[i]), req.URL.Path)
				require.Equal(t, "upstream.example", req.URL.Host)
				require.Equal(t, "actual", gjson.GetBytes(upstream.bodies[i], "model").String())
				require.Contains(t, string(upstream.bodies[i]), "prior")
				if order[i] == APIProtocolAnthropic {
					require.Equal(t, "test-secret", req.Header.Get("X-Api-Key"))
					require.Empty(t, req.Header.Get("Authorization"))
				}
			}
			require.Equal(t, protocolUpstreamEndpoint(order[2]), GetActualOpenAIUpstreamEndpoint(c))
		})
	}
}

func TestMixedExplicitProtocolRefusalStrict(t *testing.T) {
	positive := []struct{ protocol, message string }{
		{APIProtocolResponses, "model does not support openai responses protocol"},
		{APIProtocolResponses, "qwen3.8-27b does not support openai_responses protocol"},
		{APIProtocolChatCompletions, "qwen3.8-27b does not support openai_chat_completions protocol"},
		{APIProtocolChatCompletions, "Chat Completions protocol is not supported for this model"},
		{APIProtocolChatCompletions, "Cannot POST /v1/chat/completions"},
		{APIProtocolAnthropic, "Anthropic messages protocol is not supported for this model"},
		{APIProtocolAnthropic, "model does not support anthropic_messages protocol"},
	}
	for _, tt := range positive {
		body, _ := json.Marshal(map[string]any{"error": map[string]string{"message": tt.message}})
		for _, status := range []int{400, 404, 405} {
			require.True(t, isExplicitOpenAIProtocolRefusal(status, body, tt.protocol), tt.message)
		}
		for _, status := range []int{200, 401, 403, 429, 500, 502, 503} {
			require.False(t, isExplicitOpenAIProtocolRefusal(status, body, tt.protocol))
		}
	}
	negative := []string{
		`Not Found`, `{"error":{"message":"model not available","code":"model_not_available"}}`,
		`{"error":{"message":"Unsupported parameter for Responses protocol"}}`,
		`{"error":{"message":"Responses protocol is not supported","param":"tools"}}`,
		`{"error":{"message":"Responses protocol is not supported","code":"unsupported_parameter"}}`,
		`{"error":{"message":"Unsupported Responses tool schema"}}`,
		`{"error":{"message":"Unsupported tool for Chat Completions protocol"}}`,
		`{"error":{"message":"Cannot POST /v1/responses"}}`,
	}
	for _, body := range negative {
		require.False(t, isExplicitOpenAIProtocolRefusal(404, []byte(body), APIProtocolChatCompletions), body)
	}
	for _, body := range negative[0:7] {
		require.False(t, isExplicitOpenAIProtocolRefusal(404, []byte(body), APIProtocolResponses), body)
	}
}

func TestMixedNeverFallsBackOnOtherErrors(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 405, 429, 500, 502} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: mixedHTTPResponse(status, `{"error":{"message":"Unknown model or invalid parameter","code":"model_not_found"}}`, false)}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			_, _, _, err := mixedForward(t, svc, mixedTestAccount(), APIProtocolChatCompletions, mixedClientBody(APIProtocolChatCompletions, false))
			require.Error(t, err)
			require.Len(t, upstream.requests, 1)
		})
	}
	upstream := &httpUpstreamRecorder{err: errors.New("network unavailable")}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	_, _, _, err := mixedForward(t, svc, mixedTestAccount(), APIProtocolResponses, mixedClientBody(APIProtocolResponses, false))
	require.Error(t, err)
	require.Len(t, upstream.requests, 1)
}

func TestMixedRefusalDoesNotRetryCanceledOrCommitted(t *testing.T) {
	for _, committed := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		state := &openAIProtocolFallbackState{clientCtx: ctx, visited: map[string]bool{}, order: []string{APIProtocolChatCompletions, APIProtocolResponses}}
		attempt := context.WithValue(ctx, openAIProtocolFallbackKey{}, state)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
		if committed {
			c.String(200, "started")
		} else {
			cancel()
		}
		svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
		require.Nil(t, svc.protocolRefusalFallback(attempt, c, mixedTestAccount(), APIProtocolChatCompletions, 404, []byte(`{"error":{"message":"Chat Completions protocol is not supported"}}`), false))
		cancel()
	}
}

func TestMixedPlatformBoundariesAndValidation(t *testing.T) {
	account := mixedTestAccount()
	require.True(t, account.IsMixed())
	require.True(t, account.IsOpenAICompatible())
	require.False(t, account.IsOpenAI())
	require.False(t, account.IsOpenAIApiKey())
	require.False(t, account.IsCNProvider())
	require.False(t, account.IsPoolMode())
	require.Equal(t, PlatformMixed, NormalizeOpenAICompatiblePlatform(PlatformMixed))
	require.Contains(t, groupCatalogPlatforms(), PlatformMixed)
	require.Contains(t, schedulerSnapshotPlatforms(), PlatformMixed)
	require.True(t, IsAllowedQuotaPlatform(PlatformMixed))
	require.True(t, isConcreteRequestPlatform(PlatformMixed))
	require.True(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityChatCompletions))
	for _, cap := range []OpenAIEndpointCapability{OpenAIEndpointCapabilityEmbeddings, OpenAIEndpointCapabilityLive, OpenAIEndpointCapabilityAlphaSearch, OpenAIEndpointCapabilityGrokMediaGeneration} {
		require.False(t, account.SupportsOpenAIEndpointCapability(cap))
	}
	require.Equal(t, "platform_mismatch", openAICompatibleAccountEligibilityFailureReasonBeforeProfit(context.Background(), account, PlatformOpenAI, "alias", false, ""))
	require.True(t, CrossProtocolConversionAllowed(&APIKey{Group: &Group{Platform: PlatformMixed}}))
	require.False(t, CrossProtocolConversionAllowed(&APIKey{Group: &Group{Platform: PlatformOpenAI}}))
	require.False(t, CrossProtocolConversionAllowed(&APIKey{Group: &Group{Platform: PlatformComposite}}))
	for _, typ := range []string{AccountTypeOAuth, AccountTypeSetupToken, AccountTypeBedrock, AccountTypeCloudflare, AccountTypeUpstream} {
		account.Type = typ
		require.Error(t, validateMixedAccount(account))
	}
	account.Type = AccountTypeAPIKey
	for _, base := range []string{"", "api.example", "https://user:secret@api.example", "https://api.example?x=1", "ftp://api.example"} {
		account.Credentials["base_url"] = base
		require.Error(t, validateMixedAccount(account))
	}
	account.Credentials["base_url"] = "https://api.example"
	account.Credentials["api_key"] = ""
	require.Error(t, validateMixedAccount(account))
}

func TestMixedAnthropicSameOriginAndExplicitAuth(t *testing.T) {
	account := mixedTestAccount()
	account.Credentials["api_base_urls"] = map[string]any{APIProtocolAnthropic: "http://evil.example/anthropic"}
	_, err := genericOpenAIAnthropicBaseURL(account)
	require.Error(t, err)
	account.Credentials["api_base_urls"] = map[string]any{APIProtocolAnthropic: "http://upstream.example/anthropic"}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
	target, err := svc.nativeAnthropicTargetURL(account)
	require.NoError(t, err)
	require.Equal(t, "http://upstream.example/anthropic/v1/messages", target)
	account.Extra[anthropicAPIKeyAuthSchemeExtraKey] = AnthropicAPIKeyAuthSchemeAuthorizationBearer
	header := http.Header{}
	setAnthropicAPIKeyAuthHeader(header, account, "test-secret", "http://upstream.example/anthropic")
	require.Equal(t, "Bearer test-secret", header.Get("Authorization"))
	require.Empty(t, header.Get("X-Api-Key"))
	account.Credentials["base_url"] = ""
	_, err = svc.openAIChatCompletionsTargetURL(account)
	require.Error(t, err)
	_, err = buildOpenAIAPIKeyModelsRequest(context.Background(), account, func(s string) (string, error) { return s, nil })
	require.Error(t, err)
}

func mixedResponsesToolSSE() string {
	return strings.Join([]string{
		`event: response.created`, `data: {"type":"response.created","response":{"id":"resp_1","model":"actual","status":"in_progress","output":[]}}`, "",
		`event: response.output_item.added`, `data: {"type":"response.output_item.added","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":"","status":"in_progress"}}`, "",
		`event: response.function_call_arguments.delta`, `data: {"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_1","delta":"{\"id\":1}"}`, "",
		`event: response.function_call_arguments.done`, `data: {"type":"response.function_call_arguments.done","output_index":0,"item_id":"fc_1","arguments":"{\"id\":1}"}`, "",
		`event: response.output_item.done`, `data: {"type":"response.output_item.done","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"id\":1}","status":"completed"}}`, "",
		`event: response.completed`, `data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"actual","status":"completed","output":[{"id":"fc_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"id\":1}","status":"completed"}],"usage":{"input_tokens":9,"output_tokens":4,"total_tokens":13,"input_tokens_details":{"cached_tokens":3}}}}`, "", "",
	}, "\n")
}

func TestMixedFallbackSuccessPreservesClientFormatsAndTools(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, protocol := range []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
		for _, target := range []string{APIProtocolResponses, APIProtocolAnthropic} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/to=%s/stream=%t", protocol, target, stream), func(t *testing.T) {
					var upstreamResponse *http.Response
					if target == APIProtocolResponses {
						if protocol == APIProtocolResponses && !stream {
							upstreamResponse = mixedHTTPResponse(200, `{"id":"resp_1","object":"response","model":"actual","status":"completed","output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup","arguments":"{\"id\":1}"}],"usage":{"input_tokens":9,"output_tokens":4}}`, false)
						} else {
							upstreamResponse = mixedHTTPResponse(200, mixedResponsesToolSSE(), true)
						}
					} else {
						if protocol == APIProtocolAnthropic && !stream {
							upstreamResponse = mixedHTTPResponse(200, `{"id":"msg_1","type":"message","role":"assistant","model":"actual","content":[{"type":"tool_use","id":"toolu_1","name":"lookup","input":{"query":"status"}}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`, false)
						} else {
							// Real compatibility providers can emit tool_use with end_turn.
							upstreamResponse = mixedHTTPResponse(200, strings.ReplaceAll(toolAnthropicSSEStream(), `"stop_reason":"tool_use"`, `"stop_reason":"end_turn"`), true)
						}
					}
					responses := mixedResponsesForTarget(protocol, target, upstreamResponse)
					attempts := len(responses)
					upstream := &httpUpstreamRecorder{responses: responses}
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
					result, rec, c, err := mixedForward(t, svc, mixedTestAccount(), protocol, mixedClientBody(protocol, stream))
					require.NoError(t, err)
					require.NotNil(t, result)
					require.Len(t, upstream.requests, attempts)
					require.Contains(t, rec.Body.String(), "lookup")
					if protocol == APIProtocolAnthropic && target == APIProtocolAnthropic {
						require.Contains(t, rec.Body.String(), `"stop_reason":"tool_use"`)
						require.NotContains(t, rec.Body.String(), `"stop_reason":"end_turn"`)
					}
					require.Equal(t, protocolUpstreamEndpoint(target), GetActualOpenAIUpstreamEndpoint(c))
					require.Equal(t, "actual", result.UpstreamModel)
					if target == APIProtocolResponses {
						require.Equal(t, 9, result.Usage.InputTokens)
						require.Equal(t, 4, result.Usage.OutputTokens)
					} else {
						require.Equal(t, 10, result.Usage.InputTokens)
						require.Equal(t, 5, result.Usage.OutputTokens)
					}
					for _, body := range upstream.bodies {
						require.Equal(t, "actual", gjson.GetBytes(body, "model").String())
						require.Contains(t, string(body), "prior")
					}
				})
			}
		}
	}
}

func TestMixed200SSEErrorNeverReplays(t *testing.T) {
	for _, protocol := range []string{APIProtocolResponses, APIProtocolAnthropic, APIProtocolChatCompletions} {
		t.Run(protocol, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: mixedHTTPResponse(200, "data: "+`{"error":{"message":"Chat Completions protocol is not supported"}}`+"\n\n", true)}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			_, rec, _, err := mixedForward(t, svc, mixedTestAccount(), protocol, mixedClientBody(protocol, true))
			require.Error(t, err)
			require.Contains(t, rec.Body.String(), `"error"`)
			require.NotContains(t, rec.Body.String(), "response.completed")
			require.NotContains(t, rec.Body.String(), "message_stop")
			require.NotContains(t, rec.Body.String(), "[DONE]")
			require.Len(t, upstream.requests, 1)
		})
	}
}

type mixedGroupRepo struct {
	GroupRepository
	group *Group
}

func (r *mixedGroupRepo) GetByID(context.Context, int64) (*Group, error) { return r.group, nil }

func TestMixedAccountGroupBindings(t *testing.T) {
	for _, platform := range []string{PlatformMixed, PlatformComposite, PlatformOpenAI, PlatformAnthropic} {
		repo := &mixedGroupRepo{group: &Group{Platform: platform}}
		account := mixedTestAccount()
		err := validateMixedGroupBindings(context.Background(), repo, account, []int64{1})
		if platform == PlatformMixed || platform == PlatformComposite {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
		account.Platform = PlatformOpenAI
		err = validateMixedGroupBindings(context.Background(), repo, account, []int64{1})
		if platform == PlatformMixed {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
	}
	account := mixedTestAccount()
	_, err := buildAccountForCreate(&CreateAccountInput{Platform: PlatformMixed, Type: AccountTypeOAuth, Credentials: account.Credentials}, nil)
	require.Error(t, err)
	_, err = buildAccountForCreate(&CreateAccountInput{Platform: PlatformMixed, Type: AccountTypeAPIKey, Credentials: account.Credentials}, nil)
	require.NoError(t, err)
	groupRepo := &groupPlatformRepoStub{group: &Group{ID: 7, Platform: PlatformMixed, RequireOAuthOnly: true, RequirePrivacySet: true}}
	admin := &adminServiceImpl{groupRepo: groupRepo}
	group, err := admin.UpdateGroup(context.Background(), 7, &UpdateGroupInput{})
	require.NoError(t, err)
	require.False(t, group.RequireOAuthOnly)
	require.False(t, group.RequirePrivacySet)
}

func TestMixedConnectivityTestUsesSameDispatcher(t *testing.T) {
	upstream := &httpUpstreamRecorder{responses: []*http.Response{mixedRefusal(APIProtocolChatCompletions), mixedHTTPResponse(200, mixedResponsesTextSSE(), true)}}
	gateway := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	svc := &AccountTestService{openaiGatewayService: gateway}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/test", nil)
	require.NoError(t, svc.testMixedAccountConnection(c, mixedTestAccount(), "alias", "Hi"))
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "/v1/responses", upstream.lastReq.URL.Path)
	require.Contains(t, rec.Body.String(), "Hello")
	require.NotContains(t, rec.Body.String(), "/v1/")
	require.Contains(t, rec.Body.String(), `"success":true`)
}

func TestMixedTextOnlyAndExactModelBoundary(t *testing.T) {
	account := mixedTestAccount()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	for _, body := range []string{
		`{"model":"alias","audio":{"voice":"alloy"}}`,
		`{"model":"alias","modalities":["text","audio"]}`,
		`{"model":"alias","tools":[{"type":"image_generation"}]}`,
		`{"model":"alias","tools":[{"type":"web_search"}]}`,
		`{"model":"alias","context_management":[]}`,
	} {
		require.False(t, openAIProtocolFallbackEligible(c, account, []byte(body)))
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	require.False(t, openAIProtocolFallbackEligible(c, account, []byte(`{"model":"alias"}`)))
	ctx := WithOpenAIMessagesKeyRoute(context.Background())
	require.Equal(t, "vendor/gpt-6.1-sol-high", openAIMessagesKeyRouteModel(ctx, &Group{Platform: PlatformMixed}, "vendor/gpt-6.1-sol-high"))
	ctx = WithResolvedTargetPlatform(ctx, PlatformMixed)
	require.False(t, openAIMessagesKeyRouteGroupAllowed(ctx, &Group{Platform: PlatformComposite}))
	require.Equal(t, "claude-sonnet-4-5", openAIMessagesKeyRouteModel(ctx, &Group{Platform: PlatformComposite}, "claude-sonnet-4-5"))
	require.Empty(t, (&Group{Platform: PlatformMixed, Name: "Grok mixed"}).ResolveMessagesDispatchModel("claude-sonnet-4-5"))
}
