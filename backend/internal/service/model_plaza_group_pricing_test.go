//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPlazaGroupPricing_OpenAICompatibleAliasesKeepTheirBillingIdentity(t *testing.T) {
	for _, source := range []string{"", BillingModelSourceChannelMapped, BillingModelSourceRequested} {
		t.Run("source="+source, func(t *testing.T) {
			channel := Channel{
				ID: 1, Name: "cn-openai", Status: StatusActive, GroupIDs: []int64{10}, BillingModelSource: source,
				ModelMapping: map[string]map[string]string{PlatformOpenAI: {"gpt-5.4": "deepseek-v4-pro"}},
				ModelPricing: []ChannelModelPricing{
					{Platform: PlatformOpenAI, Models: []string{"gpt-5.4"}, BillingMode: BillingModeToken, InputPrice: testPtrFloat64(9e-6), OutputPrice: testPtrFloat64(19e-6)},
					{Platform: PlatformOpenAI, Models: []string{"deepseek-v4-pro"}, BillingMode: BillingModeToken, InputPrice: testPtrFloat64(0.7e-6), OutputPrice: testPtrFloat64(1.3e-6)},
				},
			}
			groups := []Group{{ID: 10, Name: "国模", Platform: PlatformOpenAI, RateMultiplier: 0.5}}
			svc := newPlazaServiceWithBilling([]Channel{channel}, groups, map[int64]string{10: PlatformOpenAI}, nil)
			out, err := svc.ListGroups(context.Background())
			require.NoError(t, err)
			require.Len(t, out, 1)
			model := plazaModelsByName(out[0].Models)["gpt-5.4"]
			require.Equal(t, PlatformOpenAI, model.Platform, "request platform must remain openai for channel price lookup")
			require.NotNil(t, model.Pricing)
			require.NotNil(t, model.OfficialPricing)
			wantInput, wantOutput, officialName := 0.7e-6, 1.3e-6, "deepseek-v4-pro"
			if source == BillingModelSourceRequested {
				wantInput, wantOutput, officialName = 9e-6, 19e-6, "gpt-5.4"
			}
			require.InDelta(t, wantInput, *model.Pricing.InputPrice, 1e-15)
			require.InDelta(t, wantOutput, *model.Pricing.OutputPrice, 1e-15)
			official, err := svc.billingService.GetModelPricing(officialName)
			require.NoError(t, err)
			require.InDelta(t, official.InputPricePerToken, *model.OfficialPricing.InputPrice, 1e-15)
			require.Equal(t, "gpt-5.4", model.Name, "keep the copyable public model ID")
		})
	}
}

func TestPlazaGroupPricing_SamePublicModelHasIndependentPricesAndReferences(t *testing.T) {
	channels := []Channel{
		{ID: 1, Name: "cn", Status: StatusActive, GroupIDs: []int64{10}, ModelMapping: map[string]map[string]string{PlatformOpenAI: {"public-model": "deepseek-v4-pro"}}, ModelPricing: []ChannelModelPricing{{Platform: PlatformOpenAI, Models: []string{"deepseek-v4-pro"}, InputPrice: testPtrFloat64(0.7e-6), OutputPrice: testPtrFloat64(1.3e-6)}}},
		{ID: 2, Name: "gpt", Status: StatusActive, GroupIDs: []int64{20}, ModelMapping: map[string]map[string]string{PlatformOpenAI: {"public-model": "gpt-5.4"}}, ModelPricing: []ChannelModelPricing{{Platform: PlatformOpenAI, Models: []string{"gpt-5.4"}, InputPrice: testPtrFloat64(8e-6), OutputPrice: testPtrFloat64(18e-6)}}},
	}
	groups := []Group{{ID: 10, Name: "国模", Platform: PlatformOpenAI, RateMultiplier: 0.5}, {ID: 20, Name: "GPT", Platform: PlatformOpenAI, RateMultiplier: 1}}
	svc := newPlazaServiceWithBilling(channels, groups, map[int64]string{10: PlatformOpenAI, 20: PlatformOpenAI}, nil)
	out, err := svc.ListGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 2)
	for _, group := range out {
		model := plazaModelsByName(group.Models)["public-model"]
		wantInput, officialName := 0.7e-6, "deepseek-v4-pro"
		if group.ID == 20 {
			wantInput, officialName = 8e-6, "gpt-5.4"
		}
		require.InDelta(t, wantInput, *model.Pricing.InputPrice, 1e-15)
		official, err := svc.billingService.GetModelPricing(officialName)
		require.NoError(t, err)
		require.InDelta(t, official.InputPricePerToken, *model.OfficialPricing.InputPrice, 1e-15, "official price memo must not cross groups' mapped billing models")
	}
}

func TestPlazaGroupPricing_TokenMetadataFollowsSelectedBillingCard(t *testing.T) {
	for _, source := range []string{BillingModelSourceChannelMapped, BillingModelSourceRequested} {
		for _, groupCard := range []bool{false, true} {
			t.Run(source+map[bool]string{false: "/channel", true: "/group"}[groupCard], func(t *testing.T) {
				requested := ChannelModelPricing{Platform: PlatformOpenAI, Models: []string{"public-model"}, BillingMode: BillingModeToken, InputPrice: testPtrFloat64(1e-6), MaxReasoningEffortMultiplier: testPtrFloat64(3), ImageInputPrice: testPtrFloat64(0.01)}
				mapped := ChannelModelPricing{Platform: PlatformOpenAI, Models: []string{"gpt-5.4"}, BillingMode: BillingModeToken, InputPrice: testPtrFloat64(2e-6), MaxReasoningEffortMultiplier: testPtrFloat64(7), ImageInputPrice: testPtrFloat64(0.02)}
				channel := Channel{ID: 1, Name: "aliases", Status: StatusActive, GroupIDs: []int64{10}, BillingModelSource: source,
					ModelMapping: map[string]map[string]string{PlatformOpenAI: {"public-model": "gpt-5.4"}}, ModelPricing: []ChannelModelPricing{requested, mapped}}
				group := Group{ID: 10, Name: "Test", Platform: PlatformOpenAI, RateMultiplier: 1}
				if groupCard {
					requested.MaxReasoningEffortMultiplier = testPtrFloat64(4)
					mapped.MaxReasoningEffortMultiplier = testPtrFloat64(8)
					group.ModelPricing = []ChannelModelPricing{requested, mapped}
				}
				svc := newPlazaServiceWithBilling([]Channel{channel}, []Group{group}, map[int64]string{10: PlatformOpenAI}, nil)
				out, err := svc.ListGroups(context.Background())
				require.NoError(t, err)
				model := plazaModelsByName(out[0].Models)["public-model"]
				selected := requested
				if source == BillingModelSourceChannelMapped {
					selected = mapped
				}
				require.Equal(t, selected.MaxReasoningEffortMultiplier, model.Pricing.MaxReasoningEffortMultiplier)
				require.Equal(t, selected.ImageInputPrice, model.Pricing.ImageInputPrice)
			})
		}
	}
}

func TestPlazaGroupPricing_NonTokenAliasesRespectBillingSourceAndGroupCards(t *testing.T) {
	for _, mode := range []BillingMode{BillingModePerRequest, BillingModeImage} {
		for _, source := range []string{BillingModelSourceChannelMapped, BillingModelSourceRequested} {
			for _, groupCard := range []bool{false, true} {
				t.Run(string(mode)+"/"+source+map[bool]string{false: "/channel", true: "/group"}[groupCard], func(t *testing.T) {
					channel := Channel{
						ID: 1, Name: "cn", Status: StatusActive, GroupIDs: []int64{10}, BillingModelSource: source,
						ModelMapping: map[string]map[string]string{PlatformOpenAI: {"public-image": "seedream-image"}},
						ModelPricing: []ChannelModelPricing{
							{Platform: PlatformOpenAI, Models: []string{"public-image"}, BillingMode: mode, PerRequestPrice: testPtrFloat64(0.09)},
							{Platform: PlatformOpenAI, Models: []string{"seedream-image"}, BillingMode: mode, PerRequestPrice: testPtrFloat64(0.03)},
						},
					}
					group := Group{ID: 10, Name: "国模图片", Platform: PlatformOpenAI, RateMultiplier: 1}
					selected, want := "seedream-image", 0.03
					if source == BillingModelSourceRequested {
						selected, want = "public-image", 0.09
					}
					if groupCard {
						group.ModelPricing = []ChannelModelPricing{{Models: []string{selected}, BillingMode: mode, PerRequestPrice: testPtrFloat64(0.02)}}
						want = 0.02
					}
					if mode == BillingModeImage {
						group.ImagePrice1K = testPtrFloat64(0.01)
					}
					svc := newPlazaServiceWithBilling([]Channel{channel}, []Group{group}, map[int64]string{10: PlatformOpenAI}, nil)
					out, err := svc.ListGroups(context.Background())
					require.NoError(t, err)
					model := plazaModelsByName(out[0].Models)["public-image"]
					require.Equal(t, mode, model.Pricing.BillingMode)
					require.InDelta(t, want, *model.Pricing.PerRequestPrice, 1e-12)
					if mode == BillingModeImage {
						gateway := &OpenAIGatewayService{billingService: svc.billingService, resolver: svc.resolver}
						apiKey := &APIKey{GroupID: &group.ID, Group: &group}
						actual := gateway.calculateOpenAIImageCost(context.Background(), selected, apiKey, &OpenAIForwardResult{ImageSize: "1K", ImageCount: 1}, 1)
						display1K := *model.Pricing.PerRequestPrice
						if groupCard {
							require.Empty(t, model.Pricing.Intervals, "model card precedes generic group image prices")
						} else {
							require.Len(t, model.Pricing.Intervals, 3)
							display1K = *model.Pricing.Intervals[0].PerRequestPrice
							require.InDelta(t, want, *model.Pricing.Intervals[1].PerRequestPrice, 1e-12)
						}
						require.InDelta(t, actual.ActualCost, display1K, 1e-12, "plaza quote must follow actual image billing priority")
					}
				})
			}
		}
	}
}

func TestPlazaGroupPricing_UnpricedMappedModelFallsBackToPublicPrice(t *testing.T) {
	channel := Channel{
		ID: 1, Name: "cn", Status: StatusActive, GroupIDs: []int64{10},
		ModelMapping: map[string]map[string]string{PlatformOpenAI: {"public-model": "unpriced-target"}},
		ModelPricing: []ChannelModelPricing{{Platform: PlatformOpenAI, Models: []string{"public-model"}, InputPrice: testPtrFloat64(0.8e-6), OutputPrice: testPtrFloat64(1.6e-6)}},
	}
	svc := newPlazaServiceWithBilling([]Channel{channel}, []Group{{ID: 10, Name: "国模", Platform: PlatformOpenAI}}, map[int64]string{10: PlatformOpenAI}, nil)
	out, err := svc.ListGroups(context.Background())
	require.NoError(t, err)
	model := plazaModelsByName(out[0].Models)["public-model"]
	require.NotNil(t, model.Pricing)
	require.InDelta(t, 0.8e-6, *model.Pricing.InputPrice, 1e-15)
	require.Nil(t, model.OfficialPricing)
}
