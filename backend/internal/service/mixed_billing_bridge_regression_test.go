//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func TestMixedBillingProviderDeepSeekPreservesOfficialPeakAndOverrides(t *testing.T) {
	const upstreamModel = "provider/deepseek-v4-flash"
	for _, override := range []string{"", "group", "channel"} {
		t.Run(override, func(t *testing.T) {
			logs := &openAIRecordUsageLogRepoStub{inserted: true}
			svc := newOpenAIRecordUsageServiceForTest(logs, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			// A real catalog lookup recognizes the prefixed identity. Deliberately
			// stale prices prove the existing official-price override still runs.
			catalog := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
				"deepseek-v4-flash": {InputCostPerToken: 1e-6, OutputCostPerToken: 2e-6},
			}}
			svc.billingService = NewBillingService(svc.cfg, catalog)
			svc.resolver = NewModelPricingResolver(nil, svc.billingService)
			in := mixedBillingInput(&OpenAIForwardResult{RequestID: "prefixed-official", Model: "alias", UpstreamModel: upstreamModel,
				Usage: OpenAIUsage{InputTokens: 100, OutputTokens: 20}}, mixedTestAccount())
			in.PricingAt = time.Date(2026, 10, 13, 9, 30, 0, 0, time.UTC)
			card := ChannelModelPricing{Platform: PlatformMixed, Models: []string{upstreamModel}, BillingMode: BillingModeToken,
				InputPrice: testPtrFloat64(1e-6), OutputPrice: testPtrFloat64(2e-6)}
			if override == "group" {
				in.APIKey.Group.ModelPricing = []ChannelModelPricing{card}
			} else if override == "channel" {
				repo := &mockChannelRepository{
					listAllFn: func(context.Context) ([]Channel, error) {
						return []Channel{{ID: 9, Status: StatusActive, GroupIDs: []int64{218}, ModelPricing: []ChannelModelPricing{card}}}, nil
					},
					getGroupPlatformsFn: func(context.Context, []int64) (map[int64]string, error) {
						return map[int64]string{218: PlatformMixed}, nil
					},
				}
				svc.resolver = NewModelPricingResolver(NewChannelService(repo, nil, nil, nil, nil), svc.billingService)
			}
			require.NoError(t, svc.RecordUsage(context.Background(), in))
			if override != "" {
				// Explicit per-identity prices must not acquire official peak factors.
				require.InDelta(t, (100*1e-6+20*2e-6)*0.1, logs.lastLog.ActualCost, 1e-12)
				return
			}
			expected, err := svc.billingService.CalculateCostUnified(CostInput{
				Ctx: context.Background(), Model: "deepseek-v4-flash", GroupID: in.APIKey.GroupID, Group: in.APIKey.Group,
				Tokens: UsageTokens{InputTokens: 100, OutputTokens: 20}, RateMultiplier: 0.1,
				PricingAt: in.PricingAt, Resolver: svc.resolver,
			})
			require.NoError(t, err)
			require.InDelta(t, expected.ActualCost, logs.lastLog.ActualCost, 1e-12)
			require.InDelta(t, (100*2.2e-7+20*6.6e-7)*2*0.1, logs.lastLog.ActualCost, 1e-12)
		})
	}
}

func TestMixedBillingResponsesUpstreamUsageAcrossIngress(t *testing.T) {
	for _, tc := range []struct {
		name, model, usage             string
		input, read, write, ttl5, ttl1 int
	}{
		{"ttl", "claude-sonnet-5-5", `{"input_tokens":130,"output_tokens":5,"cache_read_input_tokens":20,"cache_creation_input_tokens":100,"cache_creation_5m_tokens":30,"cache_creation_1h_tokens":70}`, 10, 20, 100, 30, 70},
		{"ttl-details-only", "claude-sonnet-5-5", `{"input_tokens":130,"output_tokens":5,"cache_read_input_tokens":20,"cache_creation":{"ephemeral_5m_input_tokens":30,"ephemeral_1h_input_tokens":70}}`, 10, 20, 100, 30, 70},
		{"deepseek-hit", "deepseek-v4-flash", `{"input_tokens":200,"output_tokens":5,"prompt_cache_hit_tokens":128}`, 72, 128, 0, 0, 0},
		{"explicit-zero", "deepseek-v4-flash", `{"input_tokens":200,"output_tokens":5,"prompt_cache_hit_tokens":128,"input_tokens_details":{"cached_tokens":0}}`, 200, 0, 0, 0, 0},
	} {
		var firstCost *float64
		for _, protocol := range []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", tc.name, protocol, stream), func(t *testing.T) {
					logs := &openAIRecordUsageLogRepoStub{inserted: true}
					svc := newOpenAIRecordUsageServiceForTest(logs, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil, rawChatCompletionsTestConfig())
					svc.resolver = NewModelPricingResolver(nil, svc.billingService)
					responses := []*http.Response{mixedRefusal(APIProtocolChatCompletions)}
					if protocol == APIProtocolAnthropic {
						responses = append(responses, mixedRefusal(APIProtocolAnthropic))
					}
					payload := strings.ReplaceAll(mixedResponsesToolSSE(), `{"input_tokens":9,"output_tokens":4,"total_tokens":13,"input_tokens_details":{"cached_tokens":3}}`, tc.usage)
					payload = strings.ReplaceAll(payload, `"model":"actual"`, `"model":"`+tc.model+`"`)
					if protocol == APIProtocolResponses && !stream {
						payload = `{"id":"resp_1","object":"response","model":"` + tc.model + `","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":` + tc.usage + `}`
					}
					responses = append(responses, mixedHTTPResponse(200, payload, protocol != APIProtocolResponses || stream))
					upstream := &httpUpstreamRecorder{responses: responses}
					svc.httpUpstream = upstream
					account := mixedTestAccount()
					account.Credentials["model_mapping"] = map[string]any{"alias": tc.model}
					result, _, _, err := mixedForward(t, svc, account, protocol, mixedClientBody(protocol, stream))
					require.NoError(t, err)
					require.Len(t, upstream.requests, len(responses))
					require.Equal(t, tc.write, result.Usage.CacheCreationInputTokens)
					require.Equal(t, tc.ttl5, result.Usage.CacheCreation5mTokens)
					require.Equal(t, tc.ttl1, result.Usage.CacheCreation1hTokens)
					require.Equal(t, tc.read, result.Usage.CacheReadInputTokens)
					in := mixedBillingInput(result, account)
					in.PricingAt = time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
					require.NoError(t, svc.RecordUsage(context.Background(), in))
					require.Equal(t, tc.input, logs.lastLog.InputTokens)
					require.Equal(t, tc.ttl5, logs.lastLog.CacheCreation5mTokens)
					require.Equal(t, tc.ttl1, logs.lastLog.CacheCreation1hTokens)
					expected, err := svc.billingService.CalculateCostUnified(CostInput{Ctx: context.Background(), Model: tc.model,
						GroupID: in.APIKey.GroupID, Group: in.APIKey.Group, Resolver: svc.resolver, PricingAt: in.PricingAt, RateMultiplier: 0.1,
						Tokens: UsageTokens{InputTokens: tc.input, OutputTokens: 5, CacheReadTokens: tc.read, CacheCreationTokens: tc.write, CacheCreation5mTokens: tc.ttl5, CacheCreation1hTokens: tc.ttl1}})
					require.NoError(t, err)
					require.InDelta(t, expected.TotalCost, logs.lastLog.TotalCost, 1e-12)
					require.InDelta(t, canonicalUsageActualCost(expected), logs.lastLog.ActualCost, 1e-12)
					if firstCost != nil {
						require.InDelta(t, *firstCost, logs.lastLog.ActualCost, 1e-12)
					}
					cost := logs.lastLog.ActualCost
					firstCost = &cost
				})
			}
		}
	}
}

func TestMixedBillingAnthropicPartialTTLDeltaPreservesOmittedBucket(t *testing.T) {
	var usage ClaudeUsage
	for _, step := range []struct {
		raw                   string
		ttl5, ttl1, aggregate int
	}{
		{`{"input_tokens":10,"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_5m_input_tokens":30,"ephemeral_1h_input_tokens":70}}`, 30, 70, 100},
		{`{"output_tokens":5,"cache_creation_input_tokens":105,"cache_creation":{"ephemeral_5m_input_tokens":35}}`, 35, 70, 105},
		{`{"cache_creation":{"ephemeral_1h_input_tokens":0}}`, 35, 0, 35},
		{`{"cache_creation":{"ephemeral_5m_input_tokens":0}}`, 0, 0, 0},
		{`{"output_tokens":6}`, 0, 0, 0},
	} {
		var parsed apicompat.AnthropicUsage
		require.NoError(t, json.Unmarshal([]byte(step.raw), &parsed))
		mergeAnthropicUsage(&usage, parsed)
		require.Equal(t, step.ttl5, usage.CacheCreation5mTokens)
		require.Equal(t, step.ttl1, usage.CacheCreation1hTokens)
		require.Equal(t, step.aggregate, usage.CacheCreationInputTokens)
	}
}
