//go:build unit

package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func mixedBillingInput(result *OpenAIForwardResult, account *Account) *OpenAIRecordUsageInput {
	groupID := int64(218)
	rates := make(map[string]float64, len(mixedModelVendors))
	for _, vendor := range mixedModelVendors {
		rates[vendor] = 0.1
	}
	return &OpenAIRecordUsageInput{
		Result: result, Account: account, User: &User{ID: 20},
		APIKey:             &APIKey{ID: 10, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformMixed, RateMultiplier: 0.1, MixedProviderRateMultipliers: rates}},
		ChannelUsageFields: ChannelUsageFields{OriginalModel: result.Model},
	}
}

func TestMixedBillingUnknownUpstreamNeverUsesClientAliasOrFamilyGuess(t *testing.T) {
	for _, source := range []string{"", BillingModelSourceUpstream} {
		for _, model := range []string{"qwen3.8-27b-fp8", "vendor-claude-unpriced", "not-a-model-gpt-5.5-unpriced"} {
			t.Run(source+"/"+model, func(t *testing.T) {
				logs := &openAIRecordUsageLogRepoStub{inserted: true}
				users := &openAIRecordUsageUserRepoStub{}
				svc := newOpenAIRecordUsageServiceForTest(logs, users, &openAIRecordUsageSubRepoStub{}, nil)
				in := mixedBillingInput(&OpenAIForwardResult{RequestID: "missing-price", Model: "gpt-5.1", BillingModel: "gpt-5.1", UpstreamModel: model,
					Usage: OpenAIUsage{InputTokens: 200, OutputTokens: 50, CacheReadInputTokens: 80}}, mixedTestAccount())
				in.BillingModelSource = source
				in.ChannelMappedModel = "claude-sonnet-4"
				require.NoError(t, svc.RecordUsage(context.Background(), in))
				require.Equal(t, 1, logs.calls)
				require.Equal(t, 120, logs.lastLog.InputTokens)
				require.Equal(t, 80, logs.lastLog.CacheReadTokens)
				require.Equal(t, model, *logs.lastLog.UpstreamModel)
				require.Zero(t, logs.lastLog.TotalCost)
				require.Zero(t, logs.lastLog.ActualCost)
				require.Zero(t, users.deductCalls)
			})
		}
	}
}

func TestMixedDomesticAliasForwardsAndBillsUnchangedUpstreamTarget(t *testing.T) {
	for _, upstreamModel := range []string{"deepseek-v4-flash", "unpriced-domestic-model"} {
		account := mixedTestAccount()
		account.Credentials["model_mapping"] = map[string]any{"deepseek-ai/deepseek-client-alias": upstreamModel}
		upstream := &httpUpstreamRecorder{resp: mixedHTTPResponse(200, mixedChatToolJSON, false)}
		forwarder := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
		body := []byte(strings.ReplaceAll(string(mixedClientBody(APIProtocolResponses, false)), `"alias"`, `"deepseek-client-alias"`))
		result, _, _, err := mixedForward(t, forwarder, account, APIProtocolResponses, body)
		require.NoError(t, err)
		require.Equal(t, upstreamModel, result.UpstreamModel)
		require.Equal(t, upstreamModel, gjson.GetBytes(upstream.lastBody, "model").String())
		logs := &openAIRecordUsageLogRepoStub{inserted: true}
		users := &openAIRecordUsageUserRepoStub{}
		billing := newOpenAIRecordUsageServiceForTest(logs, users, &openAIRecordUsageSubRepoStub{}, nil)
		input := mixedBillingInput(result, account)
		require.NoError(t, billing.RecordUsage(context.Background(), input))
		require.Equal(t, upstreamModel, *logs.lastLog.UpstreamModel)
		require.Equal(t, 0.1, input.APIKey.Group.RateMultiplier)
		if upstreamModel == "unpriced-domestic-model" {
			require.Zero(t, logs.lastLog.ActualCost)
			require.Zero(t, users.deductCalls)
		} else {
			expected := expectedOpenAICost(t, billing, upstreamModel, result.Usage, 0.1)
			require.InDelta(t, canonicalUsageActualCost(expected), logs.lastLog.ActualCost, 1e-12)
		}
	}
}

func TestMixedBillingExplicitSourcesAndLegacyCandidatesPreserved(t *testing.T) {
	for _, tt := range []struct{ platform, source, actual, response, want string }{
		{PlatformMixed, BillingModelSourceRequested, "unknown-model", "", "gpt-5.1"},
		{PlatformMixed, BillingModelSourceChannelMapped, "unknown-model", "", "gpt-5.4"},
		{PlatformMixed, BillingModelSourceResponse, "gpt-5.4", "gpt-5.1", "gpt-5.1"},
		{PlatformOpenAI, "", "unknown-model", "", "gpt-5.1"},
		{PlatformMixed, "", "provider/deepseek-v4-flash", "", "deepseek-v4-flash"},
		{PlatformMixed, "", "gpt5.4", "", "gpt-5.4"},
	} {
		t.Run(fmt.Sprintf("%s/%s/%s", tt.platform, tt.source, tt.actual), func(t *testing.T) {
			logs := &openAIRecordUsageLogRepoStub{inserted: true}
			svc := newOpenAIRecordUsageServiceForTest(logs, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			in := mixedBillingInput(&OpenAIForwardResult{RequestID: "explicit", Model: "gpt-5.1", UpstreamModel: tt.actual, UpstreamResponseModel: tt.response,
				Usage: OpenAIUsage{InputTokens: 100, OutputTokens: 20}}, &Account{ID: 30, Platform: tt.platform, Type: AccountTypeAPIKey})
			in.BillingModelSource = tt.source
			in.ChannelMappedModel = "gpt-5.4"
			require.NoError(t, svc.RecordUsage(context.Background(), in))
			expected := expectedOpenAICost(t, svc, tt.want, in.Result.Usage, 0.1)
			require.InDelta(t, expected.ActualCost, logs.lastLog.ActualCost, 1e-12)
		})
	}
}

func TestMixedBillingUserGroupAndAccountMultipliersPreserved(t *testing.T) {
	logs := &openAIRecordUsageLogRepoStub{inserted: true}
	users := &openAIRecordUsageUserRepoStub{}
	userRate := 0.2
	rateRepo := &openAIUserGroupRateRepoStub{rate: &userRate}
	svc := newOpenAIRecordUsageServiceForTest(logs, users, &openAIRecordUsageSubRepoStub{}, rateRepo)
	account := mixedTestAccount()
	account.RateMultiplier = testPtrFloat64(0.7)
	in := mixedBillingInput(&OpenAIForwardResult{RequestID: "multipliers", Model: "alias", UpstreamModel: "gpt-5.4",
		Usage: OpenAIUsage{InputTokens: 1000, OutputTokens: 100}}, account)
	require.NoError(t, svc.RecordUsage(context.Background(), in))
	expected := expectedOpenAICost(t, svc, "gpt-5.4", in.Result.Usage, userRate)
	require.Equal(t, 0.1, in.APIKey.Group.RateMultiplier)
	require.Equal(t, userRate, logs.lastLog.RateMultiplier)
	require.Equal(t, 0.7, *logs.lastLog.AccountRateMultiplier)
	require.InDelta(t, canonicalUsageActualCost(expected), logs.lastLog.ActualCost, 1e-12)
	require.InDelta(t, logs.lastLog.ActualCost, users.lastAmount, 1e-12)
}

func TestMixedBillingConfiguredGroupAndChannelOverrideStillPriceUnknownActual(t *testing.T) {
	for _, channel := range []bool{false, true} {
		t.Run(fmt.Sprintf("channel=%t", channel), func(t *testing.T) {
			logs := &openAIRecordUsageLogRepoStub{inserted: true}
			svc := newOpenAIRecordUsageServiceForTest(logs, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			in := mixedBillingInput(&OpenAIForwardResult{RequestID: "override", Model: "gpt-5.1", UpstreamModel: "unpriced-claude-vendor",
				Usage: OpenAIUsage{InputTokens: 100, OutputTokens: 20}}, mixedTestAccount())
			card := ChannelModelPricing{Platform: PlatformMixed, Models: []string{"unpriced-claude-vendor"}, BillingMode: BillingModeToken,
				InputPrice: testPtrFloat64(1e-6), OutputPrice: testPtrFloat64(2e-6)}
			if channel {
				// Reuse the existing resolver fixture's group and platform binding.
				card.Platform = PlatformAnthropic
				in.APIKey.GroupID = groupIDPtr()
				in.APIKey.Group.ID = *in.APIKey.GroupID
				in.APIKey.Group.Platform = PlatformAnthropic
				svc.resolver = newResolverWithChannel(t, []ChannelModelPricing{card})
			} else {
				in.APIKey.Group.ModelPricing = []ChannelModelPricing{card}
				svc.resolver = NewModelPricingResolver(nil, svc.billingService)
			}
			require.NoError(t, svc.RecordUsage(context.Background(), in))
			require.InDelta(t, (100*1e-6+20*2e-6)*0.1, logs.lastLog.ActualCost, 1e-12)
		})
	}
}

func mixedBillingChatResponse(model, usage string, stream bool) *http.Response {
	if stream {
		return mixedHTTPResponse(200, "data: "+fmt.Sprintf(`{"id":"mixed-cost","model":%q,"choices":[{"index":0,"delta":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`, model)+
			"\n\ndata: "+`{"choices":[],"usage":`+usage+"}\n\ndata: [DONE]\n\n", true)
	}
	return mixedHTTPResponse(200, fmt.Sprintf(`{"id":"mixed-cost","model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":%s}`, model, usage), false)
}

func TestMixedBillingDeepSeekStandardAndVendorUsageAcrossIngress(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, protocol := range []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", protocol, stream), func(t *testing.T) {
				var firstLog *UsageLog
				for _, usage := range []string{
					`{"prompt_tokens":200,"completion_tokens":5,"total_tokens":205,"prompt_tokens_details":{"cached_tokens":128}}`,
					`{"prompt_tokens":200,"completion_tokens":5,"total_tokens":205,"prompt_cache_hit_tokens":128,"prompt_cache_miss_tokens":72}`,
				} {
					logs := &openAIRecordUsageLogRepoStub{inserted: true}
					users := &openAIRecordUsageUserRepoStub{}
					svc := newOpenAIRecordUsageServiceForTest(logs, users, &openAIRecordUsageSubRepoStub{}, nil, rawChatCompletionsTestConfig())
					upstream := &httpUpstreamRecorder{resp: mixedBillingChatResponse("deepseek-v4-flash", usage, stream)}
					svc.httpUpstream = upstream
					account := mixedTestAccount()
					account.Credentials["model_mapping"] = map[string]any{"gpt-5.1": "deepseek-v4-flash"}
					body := []byte(strings.ReplaceAll(string(mixedClientBody(protocol, stream)), `"alias"`, `"gpt-5.1"`))
					result, _, _, err := mixedForward(t, svc, account, protocol, body)
					require.NoError(t, err)
					require.Equal(t, "deepseek-v4-flash", result.UpstreamModel)
					require.Equal(t, 200, result.Usage.InputTokens)
					require.NoError(t, svc.RecordUsage(context.Background(), mixedBillingInput(result, account)))
					require.Equal(t, 1, logs.calls)
					require.Equal(t, 1, users.deductCalls)
					require.Equal(t, 72, logs.lastLog.InputTokens)
					require.Equal(t, 128, logs.lastLog.CacheReadTokens)
					require.Equal(t, 0.1, logs.lastLog.RateMultiplier)
					expected := expectedOpenAICost(t, svc, "deepseek-v4-flash", result.Usage, 0.1)
					require.InDelta(t, canonicalUsageActualCost(expected), logs.lastLog.ActualCost, 1e-12)
					if firstLog != nil {
						require.InDelta(t, firstLog.ActualCost, logs.lastLog.ActualCost, 1e-12)
					}
					firstLog = logs.lastLog
				}
			})
		}
	}
}

func TestMixedBillingDeepSeekExplicitStandardZeroWins(t *testing.T) {
	for _, standard := range []string{
		`"input_tokens_details":{"cached_tokens":0}`,
		`"prompt_tokens_details":{"cached_tokens":0}`,
		`"cache_read_input_tokens":0`,
	} {
		usage, ok := openAIUsageFromGJSON(gjson.Parse(`{"prompt_tokens":200,"completion_tokens":5,"prompt_cache_hit_tokens":128,` + standard + `}`))
		require.True(t, ok)
		require.Equal(t, 200, usage.InputTokens)
		require.Zero(t, usage.CacheReadInputTokens)
	}
}

func TestMixedBillingAnthropicCacheBreakdownAcrossIngressAndRefusal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	usageJSON := `{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":20,"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_5m_input_tokens":30,"ephemeral_1h_input_tokens":70}}`
	for _, protocol := range []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", protocol, stream), func(t *testing.T) {
				logs := &openAIRecordUsageLogRepoStub{inserted: true}
				users := &openAIRecordUsageUserRepoStub{}
				svc := newOpenAIRecordUsageServiceForTest(logs, users, &openAIRecordUsageSubRepoStub{}, nil, rawChatCompletionsTestConfig())
				responses := []*http.Response{mixedRefusal(APIProtocolChatCompletions)}
				if protocol != APIProtocolAnthropic {
					responses = append(responses, mixedRefusal(APIProtocolResponses))
				}
				payload := `{"id":"msg_cost","type":"message","role":"assistant","model":"claude-sonnet-5-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":` + usageJSON + `}`
				if protocol != APIProtocolAnthropic || stream {
					payload = "event: message_start\ndata: " + `{"type":"message_start","message":{"id":"msg_cost","type":"message","role":"assistant","model":"claude-sonnet-5-5","content":[],"usage":` + usageJSON + "}}\n\n" +
						"event: content_block_start\ndata: " + `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
						"event: content_block_delta\ndata: " + `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}` + "\n\n" +
						"event: content_block_stop\ndata: " + `{"type":"content_block_stop","index":0}` + "\n\n" +
						"event: message_delta\ndata: " + `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}` + "\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
				}
				responses = append(responses, mixedHTTPResponse(200, payload, protocol != APIProtocolAnthropic || stream))
				upstream := &httpUpstreamRecorder{responses: responses}
				svc.httpUpstream = upstream
				account := mixedTestAccount()
				account.Credentials["model_mapping"] = map[string]any{"alias": "claude-sonnet-5-5"}
				result, _, _, err := mixedForward(t, svc, account, protocol, mixedClientBody(protocol, stream))
				require.NoError(t, err)
				require.Len(t, upstream.requests, len(responses))
				require.Zero(t, logs.calls, "protocol refusals must not record or bill usage")
				require.Zero(t, users.deductCalls)
				require.Equal(t, 130, result.Usage.InputTokens)
				require.Equal(t, 30, result.Usage.CacheCreation5mTokens)
				require.Equal(t, 70, result.Usage.CacheCreation1hTokens)
				require.NoError(t, svc.RecordUsage(context.Background(), mixedBillingInput(result, account)))
				require.Equal(t, 1, logs.calls)
				require.Equal(t, 1, users.deductCalls)
				require.Equal(t, 10, logs.lastLog.InputTokens)
				require.Equal(t, 100, logs.lastLog.CacheCreationTokens)
				require.Equal(t, 30, logs.lastLog.CacheCreation5mTokens)
				require.Equal(t, 70, logs.lastLog.CacheCreation1hTokens)
				expected, err := svc.billingService.CalculateCost("claude-sonnet-5-5", UsageTokens{InputTokens: 10, OutputTokens: 5,
					CacheReadTokens: 20, CacheCreationTokens: 100, CacheCreation5mTokens: 30, CacheCreation1hTokens: 70}, 0.1)
				require.NoError(t, err)
				require.InDelta(t, 30*2.5e-6+70*4e-6, logs.lastLog.CacheCreationCost, 1e-12)
				require.InDelta(t, expected.ActualCost, logs.lastLog.ActualCost, 1e-12)
			})
		}
	}
}

func TestMixedBillingOpenAIUsageParsesDirectCacheBreakdown(t *testing.T) {
	for _, fields := range []string{
		`"cache_creation_5m_tokens":30,"cache_creation_1h_tokens":70`,
		`"cache_creation":{"ephemeral_5m_input_tokens":30,"ephemeral_1h_input_tokens":70}`,
	} {
		usage, ok := openAIUsageFromGJSON(gjson.Parse(`{"input_tokens":130,"cache_creation_input_tokens":100,"cache_read_input_tokens":20,` + fields + `}`))
		require.True(t, ok)
		require.Equal(t, 130, usage.InputTokens)
		require.Equal(t, 100, usage.CacheCreationInputTokens)
		require.Equal(t, 30, usage.CacheCreation5mTokens)
		require.Equal(t, 70, usage.CacheCreation1hTokens)
		var progressive OpenAIUsage
		(&OpenAIGatewayService{}).parseSSEUsageBytes([]byte(`{"type":"response.in_progress","response":{"usage":`+`{"input_tokens":130,"cache_creation_input_tokens":100,"cache_read_input_tokens":20,`+fields+`}}}`), &progressive)
		require.Equal(t, usage, progressive)
	}
}
