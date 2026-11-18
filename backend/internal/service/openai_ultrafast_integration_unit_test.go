//go:build unit

package service

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func TestUltrafastPricingValidationAndClone(t *testing.T) {
	for _, value := range []float64{0, -1, 1, 1.25} {
		card := ChannelModelPricing{UltrafastMultiplier: pricingMultiplier(value), CacheWrite12hPrice: pricingMultiplier(0)}
		err := checkPricesNotNegative(card)
		if value <= 0 {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
		clone := card.Clone()
		*clone.UltrafastMultiplier = 7
		*clone.CacheWrite12hPrice = 9
		require.Equal(t, value, *card.UltrafastMultiplier)
		require.Zero(t, *card.CacheWrite12hPrice)
	}
	require.NoError(t, checkPricesNotNegative(ChannelModelPricing{}))
}

func TestUltrafastIntervalTimeReasoningAndGroupRate(t *testing.T) {
	svc := newTestBillingService()
	groupID := int64(8908)
	card := ChannelModelPricing{
		Platform: PlatformOpenAI, Models: []string{"gpt-6.2-sol"},
		InputPrice: pricingMultiplier(1e-6), OutputPrice: pricingMultiplier(2e-6),
		UltrafastMultiplier: pricingMultiplier(1.25), MaxReasoningEffortMultiplier: pricingMultiplier(3),
		Intervals:   []PricingInterval{{MinTokens: 100, InputMultiplier: pricingMultiplier(2)}},
		TimePricing: &ChannelTimePricing{Timezone: "UTC", Periods: []ChannelTimePricingPeriod{{StartTime: "07:00", EndTime: "09:00", Multiplier: 2}}},
	}
	resolver := NewModelPricingResolver(newChannelServiceWithPricings(groupID, []ChannelModelPricing{card}), svc)
	for _, at := range []time.Time{time.Date(2027, 3, 1, 7, 59, 59, 0, time.UTC), time.Date(2027, 3, 1, 8, 0, 0, 0, time.UTC)} {
		cost, err := svc.CalculateCostUnified(CostInput{Ctx: context.Background(), Model: "gpt-6.2-sol", GroupID: &groupID, Tokens: UsageTokens{InputTokens: 200, OutputTokens: 100}, RateMultiplier: 0.15, ServiceTier: "ultrafast", ReasoningEffort: "max", PricingAt: at, Resolver: resolver})
		require.NoError(t, err)
		require.InDelta(t, 200*1e-6*2*1.25*2*3, cost.InputCost, 1e-12)
		require.InDelta(t, 100*2e-6*1.25*2*3, cost.OutputCost, 1e-12)
		require.InDelta(t, cost.TotalCost*0.15, cost.ActualCost, 1e-12)
	}
}

func TestGPT62AccountStatsUseFinalUpstreamAndRequestTime(t *testing.T) {
	groupID := int64(8909)
	for _, at := range []time.Time{time.Date(2027, 3, 1, 7, 59, 59, 0, time.UTC), time.Date(2027, 3, 1, 8, 0, 0, 0, time.UTC)} {
		repo := &openAIRecordUsageLogRepoStub{inserted: true}
		svc := newOpenAIRecordUsageServiceForTest(repo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
		svc.channelService = newChannelServiceWithPricings(groupID, []ChannelModelPricing{{Platform: PlatformOpenAI, Models: []string{"gpt-6.2-sol"}, UltrafastMultiplier: pricingMultiplier(1.25)}})
		svc.resolver = NewModelPricingResolver(svc.channelService, svc.billingService)
		tier := "ultrafast"
		err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
			Result: &OpenAIForwardResult{RequestID: at.String(), Model: "gpt-6.2-sol", UpstreamModel: "gpt-6.2-astra", ServiceTier: &tier, Usage: OpenAIUsage{InputTokens: 1000, OutputTokens: 500}},
			APIKey: &APIKey{ID: 1007, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformOpenAI, RateMultiplier: 0.15, SubscriptionType: SubscriptionTypeSubscription}},
			User:   &User{ID: 2007}, Account: &Account{ID: 3007, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, PricingAt: at,
		})
		require.NoError(t, err)
		require.NotNil(t, repo.lastLog)
		require.InDelta(t, (1000*2e-6+500*10e-6)*1.25, repo.lastLog.TotalCost, 1e-12)
		require.InDelta(t, repo.lastLog.TotalCost*0.15, repo.lastLog.ActualCost, 1e-12)
		require.NotNil(t, repo.lastLog.AccountStatsCost)
		require.InDelta(t, (1000*8e-6+500*40e-6)*openAIUltrafastMultiplier("gpt-6.2-astra", at), *repo.lastLog.AccountStatsCost, 1e-12)
	}
}

func TestOpenAICacheWriteTTLCompatUsagePreservesBuckets(t *testing.T) {
	for _, field := range []string{"input_tokens_details", "prompt_tokens_details"} {
		for _, tc := range []struct {
			aggregate   int
			details     string
			short, long int
		}{
			{10, `{"ttl_30m_tokens":2,"ttl_12h_tokens":8}`, 2, 8},
			{0, `{"ttl_30m_tokens":2,"ttl_12h_tokens":8}`, 0, 0},
			{10, `{"ttl_30m_tokens":2}`, 0, 0},
			{10, `{"ttl_30m_tokens":2.5,"ttl_12h_tokens":7.5}`, 0, 0},
		} {
			raw := []byte(`{"input_tokens":15,"` + field + `":{"cache_write_tokens":` + strconv.Itoa(tc.aggregate) + `,"cache_write_tokens_details":` + tc.details + `}}`)
			var usage apicompat.ResponsesUsage
			require.NoError(t, json.Unmarshal(raw, &usage))
			converted := copyOpenAIUsageFromResponsesUsage(&usage)
			require.Equal(t, tc.aggregate, converted.CacheCreationInputTokens)
			require.Equal(t, tc.short, converted.CacheCreation30mInputTokens)
			require.Equal(t, tc.long, converted.CacheCreation12hInputTokens)
		}
	}
}
