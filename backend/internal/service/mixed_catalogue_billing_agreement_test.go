//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMixedCatalogueChannelBillingAgreement(t *testing.T) {
	for _, tc := range []struct {
		name, request, channelTarget, expected string
	}{
		{"unchanged", "kimi-k2", "", "glm-5"},
		{"identity-map", "kimi-k2", "kimi-k2", "glm-5"},
		{"actual-map", "gpt-5.4", "kimi-k2", "kimi-k2"},
		{"unpriced-map", "gpt-5.4", "qa-unpriced-alias", "glm-5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			group := &Group{ID: 2, Platform: PlatformMixed, MixedProviderRateMultipliers: map[string]float64{"kimi": 0, "zhipu": 0.8}}
			ch := Channel{ID: 1, Status: StatusActive, GroupIDs: []int64{2}, BillingModelSource: BillingModelSourceChannelMapped}
			if tc.channelTarget != "" {
				ch.ModelMapping = map[string]map[string]string{PlatformMixed: {tc.request: tc.channelTarget}}
			}
			channels := newTestChannelService(makeStandardRepo(ch, map[int64]string{2: PlatformMixed}))
			mapping := channels.ResolveChannelMapping(ctx, 2, tc.request)
			account := &Account{ID: 4, Platform: PlatformMixed, Type: AccountTypeAPIKey,
				Credentials: map[string]any{"model_mapping": map[string]any{mapping.MappedModel: "glm-5"}}}
			billing := newGatewayRecordUsageServiceForTest(&openAIRecordUsageLogRepoStub{}, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}).billingService
			billing.fallbackPrices = mixedRateTestPricing()
			gateway := &GatewayService{channelService: channels, billingService: billing, resolver: NewModelPricingResolver(channels, billing)}
			cat := groupModelsCatalog{scopeMappings: []modelScopeMapping{{platform: PlatformMixed, mapping: map[string]string{mapping.MappedModel: "glm-5"}}}}
			require.Equal(t, []string{tc.expected}, gateway.mixedCatalogueBillingModels(ctx, group, tc.request, cat))
			key := &APIKey{ID: 3, UserID: 1, User: &User{ID: 1}, GroupID: &group.ID, Group: group}
			fields := mapping.ToUsageFields(tc.request, "glm-5")
			expected, err := billing.CalculateCost(tc.expected, UsageTokens{InputTokens: 100, OutputTokens: 50}, group.RateMultiplierForModel(tc.expected))
			require.NoError(t, err)
			for _, path := range []string{"openai", "anthropic"} {
				t.Run(path, func(t *testing.T) {
					logs := &openAIRecordUsageLogRepoStub{inserted: true}
					users := &openAIRecordUsageUserRepoStub{}
					if path == "openai" {
						svc := newOpenAIRecordUsageServiceForTest(logs, users, &openAIRecordUsageSubRepoStub{}, nil)
						svc.billingService.fallbackPrices = mixedRateTestPricing()
						svc.resolver = NewModelPricingResolver(channels, svc.billingService)
						require.NoError(t, svc.RecordUsage(ctx, &OpenAIRecordUsageInput{APIKey: key, User: key.User, Account: account, ChannelUsageFields: fields,
							Result: &OpenAIForwardResult{RequestID: "catalogue-agreement", Model: tc.request, UpstreamModel: "glm-5", BillingModel: "glm-5", Usage: OpenAIUsage{InputTokens: 100, OutputTokens: 50}},
						}))
					} else {
						svc := newGatewayRecordUsageServiceForTest(logs, users, &openAIRecordUsageSubRepoStub{})
						svc.billingService.fallbackPrices = mixedRateTestPricing()
						svc.resolver = NewModelPricingResolver(channels, svc.billingService)
						require.NoError(t, svc.RecordUsage(ctx, &RecordUsageInput{APIKey: key, User: key.User, Account: account, ChannelUsageFields: fields,
							Result: &ForwardResult{RequestID: "catalogue-agreement", Model: "glm-5", UpstreamModel: "glm-5", Usage: ClaudeUsage{InputTokens: 100, OutputTokens: 50}},
						}))
					}
					require.NotNil(t, logs.lastLog)
					require.InDelta(t, expected.ActualCost, logs.lastLog.ActualCost, 1e-12)
					require.Equal(t, group.RateMultiplierForModel(tc.expected), logs.lastLog.RateMultiplier)
					require.InDelta(t, expected.ActualCost, users.lastAmount, 1e-12)
				})
			}
		})
	}
}
