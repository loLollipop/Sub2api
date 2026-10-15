//go:build unit

package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMixedScopedKeyAllIngressForwarding(t *testing.T) {
	for scope, model := range map[string]string{"openai": "gpt-5", "anthropic": "claude-4", "cn": "qwen3", "other": "gemini-3"} {
		for _, protocol := range []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
			for _, deny := range []bool{false, true} {
				t.Run(scope+"/"+protocol+"/deny="+map[bool]string{false: "false", true: "true"}[deny], func(t *testing.T) {
					account := mixedTestAccount()
					target := model
					if deny {
						target = "gpt-5"
						if scope == "openai" {
							target = "qwen3"
						}
					}
					account.Credentials["model_mapping"] = map[string]any{model: target}
					before, err := json.Marshal(account)
					require.NoError(t, err)
					upstream := &httpUpstreamRecorder{responses: mixedResponsesForTarget(protocol, APIProtocolChatCompletions, mixedHTTPResponse(200, strings.ReplaceAll(mixedChatToolJSON, "actual", target), false))}
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: withModelScopeUpstream(upstream)}
					body := bytes.ReplaceAll(mixedClientBody(protocol, false), []byte(`"alias"`), []byte(`"`+model+`"`))
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, protocolUpstreamEndpoint(protocol), bytes.NewReader(body))
					c.Request = c.Request.WithContext(ContextWithModelScope(c.Request.Context(), scope))
					c.Set("api_key", &APIKey{ModelScope: scope, Group: &Group{Platform: PlatformMixed}})
					var result *OpenAIForwardResult
					switch protocol {
					case APIProtocolResponses:
						result, err = svc.Forward(c.Request.Context(), c, account, body)
					case APIProtocolChatCompletions:
						result, err = svc.ForwardAsChatCompletions(c.Request.Context(), c, account, body, "", "")
					case APIProtocolAnthropic:
						result, err = svc.ForwardAsAnthropic(c.Request.Context(), c, account, body, "", "")
					}
					if deny {
						require.ErrorIs(t, err, ErrModelScopeDenied)
						require.Empty(t, upstream.requests, "no cross-scope network or protocol retry")
					} else {
						require.NoError(t, err)
						require.Len(t, upstream.requests, mixedExpectedAttempts(protocol, APIProtocolChatCompletions))
						require.Equal(t, target, result.UpstreamModel)
						require.Equal(t, 9, result.Usage.InputTokens)
						require.Equal(t, 4, result.Usage.OutputTokens)
					}
					after, err := json.Marshal(account)
					require.NoError(t, err)
					require.JSONEq(t, string(before), string(after), "scope does not change credentials, mapping, rate or platform")
				})
			}
		}
	}
}
