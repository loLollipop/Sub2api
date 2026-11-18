//go:build unit

package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestVertexEscapedCachedContentForwardAndDefaultBilling(t *testing.T) {
	const model = "gemini-3.7-flash"
	for _, slash := range []string{`\/`, `\u002f`} {
		for _, nested := range []bool{false, true} {
			t.Run(fmt.Sprintf("slash=%s/nested=%t", slash, nested), func(t *testing.T) {
				resource := strings.ReplaceAll("projects/my-project/locations/eu/cachedContents/42", "/", slash)
				body := `{"cachedContent":"` + resource + `","contents":[{"role":"user","parts":[{"text":"hi"}]}]}`
				if nested {
					body = `{"generateContentRequest":` + body + `}`
				}
				forwarder, upstream := newGeminiVertexAPIKeyTestService()
				account := geminiVertexAPIKeyTestAccount("my-project")
				account.Credentials["location"] = "global"
				c, _ := newGeminiNativeTestContext(t)
				result, err := forwarder.ForwardNative(context.Background(), c, account, model, "generateContent", false, []byte(body))
				require.NoError(t, err)
				require.Equal(t, "https://aiplatform.eu.rep.googleapis.com/v1/projects/my-project/locations/eu/publishers/google/models/gemini-3.7-flash:generateContent", upstream.lastReq.URL.String())
				require.Equal(t, 1.1, result.UpstreamPriceMultiplier)
				require.Equal(t, 10, result.Usage.InputTokens)
				require.Equal(t, 5, result.Usage.OutputTokens)

				group := &Group{ID: 10, Platform: PlatformGemini, Hydrated: true, RateMultiplier: .15}
				billing := NewBillingService(&config.Config{}, nil)
				channels := &ChannelService{}
				channels.cache.Store(populateChannelCache([]Channel{{ID: 1, Status: StatusActive, GroupIDs: []int64{group.ID}}}, map[int64]string{group.ID: PlatformGemini}))
				usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
				billingRepo := &openAIRecordUsageBillingRepoStub{}
				gateway := newGatewayRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})
				gateway.billingService, gateway.channelService = billing, channels
				gateway.resolver = NewModelPricingResolver(channels, billing)
				err = gateway.RecordUsage(context.Background(), &RecordUsageInput{Result: result,
					APIKey: &APIKey{ID: 501, GroupID: &group.ID, Group: group}, User: &User{ID: 601}, Account: account})
				require.NoError(t, err)
				wantTotal := (10*.75e-6 + 5*3.75e-6) * 1.1
				wantCharge := QuantizeUsageBillingAmount(wantTotal * .15)
				require.NotNil(t, usageRepo.lastLog)
				require.InDelta(t, wantTotal, usageRepo.lastLog.TotalCost, 1e-12)
				require.InDelta(t, wantCharge, usageRepo.lastLog.ActualCost, 1e-12)
				require.NotNil(t, usageRepo.lastLog.AccountStatsCost)
				require.InDelta(t, wantTotal, *usageRepo.lastLog.AccountStatsCost, 1e-12)
				require.Equal(t, 1, billingRepo.calls)
				require.InDelta(t, wantCharge, billingRepo.lastCmd.BalanceCost, 1e-12)
			})
		}
	}
}

func TestVertexForwardRecordUsagePricingPriority(t *testing.T) {
	const model = "gemini-3.7-flash"
	for _, source := range []string{"default", "group", "channel"} {
		for _, stats := range []string{"model", "customer", "custom"} {
			for _, multiplier := range []float64{0, .15, 1} {
				t.Run(fmt.Sprintf("%s/%s/%g", source, stats, multiplier), func(t *testing.T) {
					forwarder, upstream := newGeminiVertexAPIKeyTestService()
					account := geminiVertexAPIKeyTestAccount("my-project")
					account.Credentials["location"] = "us"
					c, _ := newGeminiNativeTestContext(t)
					result, err := forwarder.ForwardNative(context.Background(), c, account, model, "generateContent", false,
						[]byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
					require.NoError(t, err)
					require.Equal(t, "aiplatform.us.rep.googleapis.com", upstream.lastReq.URL.Host)
					require.Equal(t, 1.1, result.UpstreamPriceMultiplier)
					require.Equal(t, 10, result.Usage.InputTokens)
					require.Equal(t, 5, result.Usage.OutputTokens)

					billing := NewBillingService(&config.Config{}, nil)
					groupID := int64(10)
					group := &Group{ID: groupID, Platform: PlatformGemini, Hydrated: true, RateMultiplier: multiplier}
					card := ChannelModelPricing{Platform: PlatformGemini, Models: []string{model}, BillingMode: BillingModeToken, InputPrice: float64Ptr(.01), OutputPrice: float64Ptr(.01)}
					channel := Channel{ID: 1, Status: StatusActive, GroupIDs: []int64{groupID}}
					wantTotal := (10*.75e-6 + 5*3.75e-6) * 1.1
					wantStats := wantTotal
					if source == "group" {
						group.ModelPricing = []ChannelModelPricing{card}
						wantTotal = .15
					} else if source == "channel" {
						channel.ModelPricing = []ChannelModelPricing{card}
						wantTotal = .15
					}
					if stats == "customer" {
						channel.ApplyPricingToAccountStats = true
						wantStats = wantTotal
					} else if stats == "custom" {
						// Explicit account rules win even when customer pricing is enabled.
						channel.ApplyPricingToAccountStats = true
						channel.AccountStatsPricingRules = []AccountStatsPricingRule{{AccountIDs: []int64{account.ID}, Pricing: []ChannelModelPricing{card}}}
						wantStats = .15
					}
					channels := &ChannelService{}
					channels.cache.Store(populateChannelCache([]Channel{channel}, map[int64]string{groupID: PlatformGemini}))
					usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
					billingRepo := &openAIRecordUsageBillingRepoStub{}
					gateway := newGatewayRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})
					gateway.billingService, gateway.channelService = billing, channels
					gateway.resolver = NewModelPricingResolver(channels, billing)
					err = gateway.RecordUsage(context.Background(), &RecordUsageInput{Result: result,
						APIKey: &APIKey{ID: 501, GroupID: &groupID, Group: group}, User: &User{ID: 601}, Account: account})
					require.NoError(t, err)
					require.NotNil(t, usageRepo.lastLog)
					require.InDelta(t, wantTotal, usageRepo.lastLog.TotalCost, 1e-12)
					wantCharge := QuantizeUsageBillingAmount(wantTotal * multiplier)
					require.InDelta(t, wantCharge, usageRepo.lastLog.ActualCost, 1e-12)
					require.NotNil(t, usageRepo.lastLog.AccountStatsCost)
					require.InDelta(t, wantStats, *usageRepo.lastLog.AccountStatsCost, 1e-12)
					if multiplier > 0 {
						require.Equal(t, 1, billingRepo.calls)
						require.InDelta(t, wantCharge, billingRepo.lastCmd.BalanceCost, 1e-12)
					}
				})
			}
		}
	}
}

func TestVertexMultiplierPreservesZeroCustomPrices(t *testing.T) {
	billing := NewBillingService(&config.Config{}, nil)
	for _, source := range []string{"group", "channel"} {
		t.Run(source, func(t *testing.T) {
			card := ChannelModelPricing{Platform: PlatformGemini, Models: []string{"gemini-3.7-flash"}, BillingMode: BillingModeToken, InputPrice: float64Ptr(0), OutputPrice: float64Ptr(0)}
			group := &Group{ID: 10, Hydrated: true, Platform: PlatformGemini}
			var channels *ChannelService
			if source == "group" {
				group.ModelPricing = []ChannelModelPricing{card}
			} else {
				channels = &ChannelService{}
				channels.cache.Store(populateChannelCache([]Channel{{ID: 1, Status: StatusActive, GroupIDs: []int64{group.ID}, ModelPricing: []ChannelModelPricing{card}}}, map[int64]string{group.ID: PlatformGemini}))
			}
			gateway := &GatewayService{billingService: billing, resolver: NewModelPricingResolver(channels, billing)}
			cost := gateway.calculateRecordUsageCost(context.Background(), &ForwardResult{UpstreamPriceMultiplier: 1.1, Usage: ClaudeUsage{InputTokens: 10, OutputTokens: 5}},
				&APIKey{GroupID: &group.ID, Group: group}, "gemini-3.7-flash", 1, 1, time.Now())
			require.Zero(t, cost.TotalCost)
			require.Zero(t, cost.ActualCost)
		})
	}
}

func TestVertexMultiplierOnlyScalesTokenComponents(t *testing.T) {
	for _, mode := range []string{"", string(BillingModeToken), string(BillingModePerRequest), string(BillingModeImage)} {
		cost := &CostBreakdown{InputCost: 1, OutputCost: 2, CacheCreationCost: 3, CacheReadCost: 4, TotalCost: 10, ActualCost: 1.5, BillingMode: mode}
		applyUpstreamPriceFactor(cost, &ForwardResult{UpstreamPriceMultiplier: 1.1})
		factor := 1.0
		if mode == "" || mode == string(BillingModeToken) {
			factor = 1.1
		}
		require.InDelta(t, factor, cost.InputCost, 1e-12)
		require.InDelta(t, 2*factor, cost.OutputCost, 1e-12)
		require.InDelta(t, 3*factor, cost.CacheCreationCost, 1e-12)
		require.InDelta(t, 4*factor, cost.CacheReadCost, 1e-12)
		require.InDelta(t, 10*factor, cost.TotalCost, 1e-12)
		require.InDelta(t, 1.5*factor, cost.ActualCost, 1e-12)
	}
	for _, unset := range []*ForwardResult{nil, {}, {UpstreamPriceMultiplier: 1}} {
		require.Equal(t, 1.0, unset.upstreamPriceFactor())
	}
	billing := NewBillingService(&config.Config{}, nil)
	gateway := &GatewayService{billingService: billing, resolver: NewModelPricingResolver(nil, billing)}
	result := &ForwardResult{Model: "gemini-3.7-flash", Usage: ClaudeUsage{InputTokens: 10000, OutputTokens: 1000}, SearchCount: 2, UpstreamPriceMultiplier: 1.1}
	key := &APIKey{Group: &Group{ID: 10, Hydrated: true, SearchPricePer1k: float64Ptr(5)}}
	cost := gateway.calculateRecordUsageCost(context.Background(), result, key, result.Model, .15, 1, time.Now())
	want := (10000*.75e-6+1000*3.75e-6)*1.1 + .01
	require.InDelta(t, want, cost.TotalCost, 1e-12)
	require.InDelta(t, want*.15, cost.ActualCost, 1e-12)
}
