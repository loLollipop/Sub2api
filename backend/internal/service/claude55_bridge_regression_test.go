//go:build unit

package service

import (
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

// Exercise the real request builders, conversion pumps and usage recorders:
// the billed effort must be what Anthropic received, not OpenAI's xhigh alias.
func TestClaude55BridgesBillFinalMaxEffort(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, chat := range []bool{false, true} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("native=%v/chat=%v/stream=%v", native, chat, stream), func(t *testing.T) {
					const model = "claude-opus-5-5"
					path := "/v1/responses"
					body := []byte(fmt.Sprintf(`{"model":"public-alias","input":"hi","reasoning":{"effort":"max"},"stream":%v}`, stream))
					if chat {
						path = "/v1/chat/completions"
						body = []byte(fmt.Sprintf(`{"model":"public-alias","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"max","stream":%v}`, stream))
					}
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, path, nil)
					upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
						StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
						Body: io.NopCloser(strings.NewReader(strings.ReplaceAll(miniAnthropicSSEStream(), "glm-4.7", model))),
					}}
					account := &Account{ID: 3, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Concurrency: 1,
						Credentials: map[string]any{"base_url": "https://api.anthropic.com", "api_key": "test-only",
							"api_protocol": APIProtocolAnthropic, "model_mapping": map[string]any{"public-alias": model}}}
					inputPrice, outputPrice, maxMultiplier := 1e-6, 2e-6, 3.0
					group := &Group{ID: 9, Platform: PlatformAnthropic, RateMultiplier: 0.15,
						ModelPricing: []ChannelModelPricing{{Models: []string{model}, BillingMode: BillingModeToken,
							InputPrice: &inputPrice, OutputPrice: &outputPrice, MaxReasoningEffortMultiplier: &maxMultiplier}}}
					key := &APIKey{ID: 7, UserID: 42, GroupID: &group.ID, Group: group}
					usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
					userRepo := &openAIRecordUsageUserRepoStub{}
					if native {
						account.Platform, group.Platform = PlatformMixed, PlatformMixed
						group.MixedProviderRateMultipliers = map[string]float64{"anthropic": 0.15}
						svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
						svc.cfg, svc.httpUpstream = rawChatCompletionsTestConfig(), upstream
						svc.resolver = NewModelPricingResolver(nil, svc.billingService)
						var result *OpenAIForwardResult
						var err error
						if chat {
							result, err = svc.forwardChatCompletionsViaNativeAnthropic(context.Background(), c, account, body, "")
						} else {
							result, err = svc.forwardResponsesViaNativeAnthropic(context.Background(), c, account, body, "")
						}
						require.NoError(t, err)
						require.NotNil(t, result.ReasoningEffort)
						require.Equal(t, "max", *result.ReasoningEffort)
						require.NoError(t, svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
							Result: result, APIKey: key, User: &User{ID: 42}, Account: account}))
					} else {
						svc := newGatewayRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{})
						svc.cfg, svc.httpUpstream = rawChatCompletionsTestConfig(), upstream
						svc.resolver = NewModelPricingResolver(nil, svc.billingService)
						var result *ForwardResult
						var err error
						if chat {
							result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, nil)
						} else {
							result, err = svc.ForwardAsResponses(context.Background(), c, account, body, nil)
						}
						require.NoError(t, err)
						require.NotNil(t, result.ReasoningEffort)
						require.Equal(t, "max", *result.ReasoningEffort)
						require.NoError(t, svc.RecordUsage(context.Background(), &RecordUsageInput{
							Result: result, APIKey: key, User: &User{ID: 42}, Account: account}))
					}
					require.Equal(t, "max", gjson.GetBytes(upstream.lastBody, "output_config.effort").String())
					require.NotNil(t, usageRepo.lastLog)
					log := usageRepo.lastLog
					want := (float64(log.InputTokens)*inputPrice + float64(log.OutputTokens)*outputPrice) * maxMultiplier
					require.Positive(t, want)
					require.InDelta(t, want, log.TotalCost, 1e-12)
					require.InDelta(t, want*0.15, log.ActualCost, 1e-12)
				})
			}
		}
	}
}

func TestClaude55BridgeInvalidParametersAre400BeforeTransport(t *testing.T) {
	for _, chat := range []bool{false, true} {
		for _, tc := range []struct{ model, responses, chat string }{
			{"claude-opus-5-5", `"tool_choice":"required"`, `"tool_choice":"required"`},
			{"claude-haiku-5-5", `"top_p":1`, `"top_p":1`},
			{"claude-sonnet-5-5", `"reasoning":{"effort":"minimal"}`, `"reasoning_effort":"minimal"`},
		} {
			t.Run(fmt.Sprintf("%s/chat=%v", tc.model, chat), func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				account := &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey,
					Credentials: map[string]any{"model_mapping": map[string]any{"alias": tc.model}}}
				body := []byte(`{"model":"alias","input":"hi",` + tc.responses + `}`)
				var err error
				// No credentials/transport: conversion must fail before either.
				if chat {
					body = []byte(`{"model":"alias","messages":[{"role":"user","content":"hi"}],` + tc.chat + `}`)
					_, err = (&GatewayService{}).ForwardAsChatCompletions(context.Background(), c, account, body, nil)
				} else {
					_, err = (&GatewayService{}).ForwardAsResponses(context.Background(), c, account, body, nil)
				}
				require.Error(t, err)
				require.Equal(t, http.StatusBadRequest, rec.Code)
				require.Contains(t, rec.Body.String(), "invalid_request_error")
			})
		}
	}
}

func TestHaiku55EnabledRejectedBeforeMessagesAndCountTokensTransport(t *testing.T) {
	for _, endpoint := range []string{"messages", "count_tokens", "mixed-messages"} {
		t.Run(endpoint, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			body := []byte(`{"model":"alias","max_tokens":100,"thinking":{"type":"enabled","budget_tokens":50},"messages":[{"role":"user","content":"hi"}]}`)
			account := &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey,
				Credentials: map[string]any{"model_mapping": map[string]any{"alias": "claude-haiku-5-5"}}}
			parsed := &ParsedRequest{Model: "alias", Body: NewRequestBodyRef(body)}
			var err error
			switch endpoint {
			case "messages":
				_, err = (&GatewayService{}).Forward(context.Background(), c, account, parsed)
			case "mixed-messages":
				account.Platform = PlatformMixed
				_, err = (&OpenAIGatewayService{}).forwardAnthropicViaNativeAnthropicEndpoint(context.Background(), c, account, body, "")
			default:
				err = (&GatewayService{}).ForwardCountTokens(context.Background(), c, account, parsed)
			}
			require.ErrorContains(t, err, "thinking")
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Contains(t, rec.Body.String(), "invalid_request_error")
		})
	}
}
