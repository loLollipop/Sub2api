//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMixedOfficialImportsReuseOriginalProviderForwarding(t *testing.T) {
	for _, provider := range []struct{ platform, base, model, responsesPath string }{
		{PlatformDeepseek, DefaultDeepseekBaseURL, "deepseek-v4-pro-0813", "/responses"},
		{PlatformKimi, DefaultKimiPayGBaseURL, "kimi-k3", "/v1/responses"},
		{PlatformZhipu, DefaultZhipuPayGBaseURL, "glm-5.3", ""},
		{PlatformMiniMax, DefaultMiniMaxBaseURL, "MiniMax-M3", "/v1/responses"},
		{PlatformKimi, DefaultKimiCodingBaseURL, "kimi-k3", "/coding/v1/responses"},
		{PlatformZhipu, DefaultZhipuCodingBaseURL, "glm-5.3", ""},
		{PlatformMiniMax, mixedMiniMaxInternationalBaseURL, "MiniMax-M3", "/v1/responses"},
	} {
		for _, ingress := range []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
			for _, stream := range []bool{false, true} {
				for _, history := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/stream=%t/history=%t", provider.platform, ingress, stream, history), func(t *testing.T) {
						body := mixedPlainClientBody(ingress, stream)
						if history {
							body = mixedClientBody(ingress, stream)
						}
						if ingress == APIProtocolResponses {
							body = []byte(strings.TrimSuffix(string(body), "}") + `,"store":true,"previous_response_id":"old-response","max_output_tokens":128}`)
						}
						var upstreams []*httpUpstreamRecorder
						for _, platform := range []string{provider.platform, PlatformMixed} {
							account := mixedTestAccount()
							account.Platform = platform
							account.Credentials["base_url"] = provider.base
							account.Credentials["model_mapping"] = map[string]any{"alias": provider.model}
							account.Extra["openai_passthrough"] = false
							if platform != PlatformMixed {
								account.Credentials["api_protocol"] = APIProtocolAdaptive
								if provider.base == DefaultKimiCodingBaseURL || provider.base == DefaultZhipuCodingBaseURL {
									account.Credentials["account_mode"] = AccountModeCoding
								}
								if provider.base == mixedMiniMaxInternationalBaseURL {
									account.Credentials["api_base_urls"] = map[string]any{
										APIProtocolChatCompletions: provider.base,
										APIProtocolResponses:       provider.base,
										APIProtocolAnthropic:       mixedMiniMaxInternationalAnthropicBaseURL,
									}
								}
							}
							before, err := json.Marshal(account)
							require.NoError(t, err)
							wire := mixedChatToolJSON
							if stream {
								wire = mixedChatToolSSE()
							}
							if ingress == APIProtocolResponses && provider.responsesPath != "" {
								wire = `{"id":"resp_1","object":"response","model":"actual","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":9,"output_tokens":4}}`
								if stream {
									wire = mixedResponsesTextSSE()
								}
							} else if ingress == APIProtocolAnthropic {
								wire = `{"id":"msg_1","type":"message","role":"assistant","model":"actual","content":[{"type":"text","text":"OK"}],"stop_reason":"end_turn","usage":{"input_tokens":9,"output_tokens":4}}`
								if stream {
									wire = toolAnthropicSSEStream()
								}
							}
							wire = strings.ReplaceAll(wire, `"actual"`, `"`+provider.model+`"`)
							upstream := &httpUpstreamRecorder{resp: mixedHTTPResponse(200, wire, stream)}
							svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
							result, _, _, err := mixedForward(t, svc, account, ingress, body)
							require.NoError(t, err)
							require.NotNil(t, result)
							require.Len(t, upstream.requests, 1)
							require.Equal(t, provider.model, result.UpstreamModel)
							if ingress == APIProtocolResponses && provider.responsesPath != "" {
								require.Equal(t, provider.responsesPath, upstream.lastReq.URL.Path)
								require.False(t, gjson.GetBytes(upstream.lastBody, "store").Bool())
								require.False(t, gjson.GetBytes(upstream.lastBody, "previous_response_id").Exists())
							}
							after, err := json.Marshal(account)
							require.NoError(t, err)
							require.JSONEq(t, string(before), string(after), "request-only provider details must not mutate the imported account")
							upstreams = append(upstreams, upstream)
						}
						require.Equal(t, upstreams[0].lastReq.URL.String(), upstreams[1].lastReq.URL.String())
						require.JSONEq(t, string(upstreams[0].lastBody), string(upstreams[1].lastBody))
						require.Equal(t, upstreams[0].lastReq.Header.Get("Authorization"), upstreams[1].lastReq.Header.Get("Authorization"))
						require.Equal(t, upstreams[0].lastReq.Header.Get("X-Api-Key"), upstreams[1].lastReq.Header.Get("X-Api-Key"))
					})
				}
			}
		}
	}
}

func TestMixedOfficialProviderRecognitionDoesNotGuessFromModels(t *testing.T) {
	for _, base := range []string{
		"https://relay.example/v1", "https://api.deepseek.com.evil.example", "https://deepseek.example",
		"https://api.deepseek.com:8443", "http://api.deepseek.com", "https://api.deepseek.com/custom",
		"https://api.deepseek.com/%61nthropic", "https://api.deepseek.com?key=value", "https://secret@api.deepseek.com",
		"https://api.minimax.io.evil.example/v1", "https://api.minimax.io:8443/v1",
	} {
		t.Run(base, func(t *testing.T) {
			account := mixedTestAccount()
			account.Credentials["base_url"] = base
			account.Credentials["model_mapping"] = map[string]any{"deepseek-v4-pro-0813": "deepseek-v4-pro-0813"}
			require.Nil(t, mixedOfficialProviderView(account))
			require.Equal(t, PlatformMixed, account.openAIWirePlatform())
		})
	}
	account := mixedTestAccount()
	account.Credentials["base_url"] = DefaultDeepseekBaseURL
	account.Credentials["api_base_urls"] = map[string]any{APIProtocolAnthropic: "https://other.example/anthropic"}
	require.Nil(t, mixedOfficialProviderView(account), "do not reuse vendor defaults across credential origins")
}

func TestMixedOfficialInternationalExplicitAnthropicPreservesOrigin(t *testing.T) {
	for _, ingress := range []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", ingress, stream), func(t *testing.T) {
				var upstreams []*httpUpstreamRecorder
				for _, platform := range []string{PlatformMiniMax, PlatformMixed} {
					account := mixedTestAccount()
					account.Platform = platform
					account.Credentials["base_url"] = mixedMiniMaxInternationalAnthropicBaseURL
					account.Credentials["api_protocol"] = APIProtocolAnthropic
					account.Credentials["model_mapping"] = map[string]any{"alias": "MiniMax-M3"}
					wire := `{"id":"msg_1","type":"message","role":"assistant","model":"MiniMax-M3","content":[{"type":"text","text":"OK"}],"stop_reason":"end_turn","usage":{"input_tokens":9,"output_tokens":4}}`
					// Existing Responses/Chat -> Anthropic forwarding always reads
					// upstream SSE, buffering it when the client is non-streaming.
					upstreamStream := stream || ingress != APIProtocolAnthropic
					if upstreamStream {
						wire = toolAnthropicSSEStream()
					}
					upstream := &httpUpstreamRecorder{resp: mixedHTTPResponse(200, wire, upstreamStream)}
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
					_, _, _, err := mixedForward(t, svc, account, ingress, mixedClientBody(ingress, stream))
					require.NoError(t, err)
					require.Len(t, upstream.requests, 1)
					require.Equal(t, "https://api.minimax.io/anthropic/v1/messages", upstream.lastReq.URL.String())
					if platform == PlatformMixed {
						require.Equal(t, mixedMiniMaxInternationalBaseURL, account.GetOpenAIFormatBaseURL())
					}
					upstreams = append(upstreams, upstream)
				}
				require.JSONEq(t, string(upstreams[0].lastBody), string(upstreams[1].lastBody))
			})
		}
	}
}

func TestMixedOfficialCanceledRequestDoesNotReachUpstream(t *testing.T) {
	for _, ingress := range []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
		t.Run(ingress, func(t *testing.T) {
			account := mixedTestAccount()
			account.Credentials["base_url"] = DefaultDeepseekBaseURL
			account.Credentials["model_mapping"] = map[string]any{"alias": "deepseek-v4-pro-0813"}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			upstream := headerCancelUpstreamFunc(func(req *http.Request) (*http.Response, error) {
				require.ErrorIs(t, req.Context().Err(), context.Canceled)
				return nil, req.Context().Err()
			})
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			body := mixedPlainClientBody(ingress, true)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, protocolUpstreamEndpoint(ingress), strings.NewReader(string(body))).WithContext(ctx)
			c.Set("api_key", &APIKey{Group: &Group{Platform: PlatformMixed}})
			done := make(chan error, 1)
			go func() {
				var err error
				switch ingress {
				case APIProtocolResponses:
					_, err = svc.Forward(context.WithoutCancel(ctx), c, account, body)
				case APIProtocolChatCompletions:
					_, err = svc.ForwardAsChatCompletions(context.WithoutCancel(ctx), c, account, body, "", "")
				case APIProtocolAnthropic:
					_, err = svc.ForwardAsAnthropic(context.WithoutCancel(ctx), c, account, body, "", "")
				}
				done <- err
			}()
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(2 * time.Second):
				t.Fatal("official dispatch did not honor client cancellation")
			}
		})
	}
}

func TestMixedOfficialProtocolHelpersReuseProviderModes(t *testing.T) {
	for _, provider := range []struct{ platform, mode, base string }{
		{PlatformDeepseek, AccountModePayG, DefaultDeepseekBaseURL},
		{PlatformDeepseek, AccountModePayG, DefaultDeepseekAnthropicBaseURL},
		{PlatformKimi, AccountModePayG, DefaultKimiPayGBaseURL},
		{PlatformKimi, AccountModePayG, DefaultKimiPayGAnthropicBaseURL},
		{PlatformKimi, AccountModeCoding, DefaultKimiCodingBaseURL},
		{PlatformKimi, AccountModeCoding, DefaultKimiCodingAnthropicBaseURL},
		{PlatformZhipu, AccountModePayG, DefaultZhipuPayGBaseURL},
		{PlatformZhipu, AccountModeCoding, DefaultZhipuCodingBaseURL},
		{PlatformZhipu, AccountModePayG, DefaultZhipuAnthropicBaseURL},
		{PlatformMiniMax, AccountModePayG, DefaultMiniMaxBaseURL},
		{PlatformMiniMax, AccountModePayG, DefaultMiniMaxAnthropicBaseURL},
	} {
		for _, protocol := range []string{APIProtocolAdaptive, APIProtocolChatCompletions, APIProtocolResponses, APIProtocolAnthropic} {
			t.Run(provider.base+"/"+protocol, func(t *testing.T) {
				native := mixedTestAccount()
				native.Platform = provider.platform
				native.Credentials["base_url"] = provider.base
				native.Credentials["account_mode"] = provider.mode
				native.Credentials["api_protocol"] = protocol
				if protocol == APIProtocolAdaptive {
					// The original import form saves one official base per wire
					// protocol; a Mixed import derives the same existing defaults.
					bases := make(map[string]any, 3)
					for _, wire := range []string{APIProtocolChatCompletions, APIProtocolResponses, APIProtocolAnthropic} {
						bases[wire] = native.defaultCNProtocolBaseURL(wire)
					}
					native.Credentials["api_base_urls"] = bases
				}
				mixed := cloneSnapshotAccount(native)
				mixed.Platform = PlatformMixed
				delete(mixed.Credentials, "account_mode")
				delete(mixed.Credentials, "api_base_urls")
				require.NotNil(t, mixedOfficialProviderView(&mixed))
				require.Equal(t, native.GetAPIProtocol(), mixed.GetAPIProtocol())
				require.Equal(t, native.GetOpenAIBaseURL(), mixed.GetOpenAIBaseURL())
				require.Equal(t, native.GetAnthropicProtocolBaseURL(), mixed.GetAnthropicProtocolBaseURL())
				require.Equal(t, native.GetOpenAIFormatBaseURL(), mixed.GetOpenAIFormatBaseURL())
				for _, wire := range []string{APIProtocolChatCompletions, APIProtocolResponses, APIProtocolAnthropic} {
					require.Equal(t, native.GetCNProtocolBaseURL(wire), mixed.GetCNProtocolBaseURL(wire))
				}
				body := []byte(`{"model":"alias"}`)
				require.Equal(t, deepSeekDuplicateCallIDRewrite(native, body), deepSeekDuplicateCallIDRewrite(&mixed, body))
				require.Equal(t, PlatformMixed, mixed.Platform)
			})
		}
	}
}
