//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMixedVendorInflightCandidateRates(t *testing.T) {
	billing := newGatewayRecordUsageServiceForTest(&openAIRecordUsageLogRepoStub{}, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}).billingService
	billing.fallbackPrices = mixedRateTestPricing()
	id := int64(2)
	key := &APIKey{User: &User{ID: 1}, GroupID: &id, Group: &Group{ID: id, Platform: PlatformMixed, RateMultiplier: 9, MixedProviderRateMultipliers: map[string]float64{"kimi": 0.2, "zhipu": 0.8}}}
	r := newUserGroupRateResolver(&userGroupRateRepoHotpathStub{}, nil, time.Minute, nil, "test")
	d := inflightEstimateDeps{billing: billing, userGroupRate: r.Resolve}
	for model, rate := range map[string]float64{"kimi-k2": 0.2, "glm-5": 0.8} {
		request := InflightEstimateRequest{Model: model, BodyBytes: 400, MaxTokens: 50}
		got, priced := d.estimate(context.Background(), key, request)
		require.True(t, priced)
		price, err := billing.GetModelPricing(model)
		require.NoError(t, err)
		require.InDelta(t, rate*(100*price.InputPricePerToken+50*price.OutputPricePerToken), got, 1e-12)
	}
	key.Group.MixedProviderRateMultipliers["kimi"] = 0
	d.accountMappedModels = func(context.Context, *APIKey, string) []string { return []string{"glm-5"} }
	got, priced := d.estimate(context.Background(), key, InflightEstimateRequest{Model: "kimi-k2", BodyBytes: 400, MaxTokens: 50})
	require.True(t, priced)
	price, err := billing.GetModelPricing("glm-5")
	require.NoError(t, err)
	require.InDelta(t, 0.8*(100*price.InputPricePerToken+50*price.OutputPricePerToken), got, 1e-12, "free public alias must not bypass paid mapped model reservation")
	zero := 0.0
	d.userGroupRate = newUserGroupRateResolver(&userGroupRateRepoHotpathStub{rate: &zero}, nil, time.Minute, nil, "test").Resolve
	got, priced = d.estimate(context.Background(), key, InflightEstimateRequest{Model: "kimi-k2", BodyBytes: 400, MaxTokens: 50})
	require.True(t, priced)
	require.Zero(t, got)
	// Even an unpriced alias requires no reservation when a user override
	// makes every possible mapped vendor free.
	got, priced = d.estimate(context.Background(), key, InflightEstimateRequest{Model: "qa-unpriced-alias", BodyBytes: 400, MaxTokens: 50})
	require.True(t, priced)
	require.Zero(t, got)
	// A paid independent image rate must not make a free token request unpriced.
	key.Group.ImageRateIndependent = true
	key.Group.ImageRateMultiplier = 2
	d.userGroupRate = r.Resolve
	d.accountMappedModels = nil
	got, priced = d.estimate(context.Background(), key, InflightEstimateRequest{Model: "kimi-k2", BodyBytes: 400, MaxTokens: 50})
	require.True(t, priced)
	require.Zero(t, got)
	// Explicit requested billing must not reserve a paid account-mapping fallback.
	d.accountMappedModels = func(context.Context, *APIKey, string) []string { return []string{"glm-5"} }
	d.resolveMapping = func(context.Context, int64, string) ChannelMappingResult {
		return ChannelMappingResult{MappedModel: "glm-5", BillingModelSource: BillingModelSourceRequested}
	}
	got, priced = d.estimate(context.Background(), key, InflightEstimateRequest{Model: "kimi-k2", BodyBytes: 400, MaxTokens: 50})
	require.True(t, priced)
	require.Zero(t, got)
	// A real, priced channel mapping also fixes the charge identity. The
	// public model and account target must not turn its zero rate into a hold.
	d.resolveMapping = func(context.Context, int64, string) ChannelMappingResult {
		return ChannelMappingResult{MappedModel: "kimi-k2", Mapped: true, BillingModelSource: BillingModelSourceChannelMapped}
	}
	got, priced = d.estimate(context.Background(), key, InflightEstimateRequest{Model: "glm-5", BodyBytes: 400, MaxTokens: 50})
	require.True(t, priced)
	require.Zero(t, got)
	// An unpriced channel identity still falls back to the concrete target.
	d.resolveMapping = func(context.Context, int64, string) ChannelMappingResult {
		return ChannelMappingResult{MappedModel: "qa-unpriced-alias", Mapped: true, BillingModelSource: BillingModelSourceChannelMapped}
	}
	got, priced = d.estimate(context.Background(), key, InflightEstimateRequest{Model: "qa-unpriced-request", BodyBytes: 400, MaxTokens: 50})
	require.True(t, priced)
	require.Positive(t, got)
}

func TestMixedVendorPlazaModelRate(t *testing.T) {
	group := Group{ID: 2, Name: "Mixed", Platform: PlatformMixed, RateMultiplier: 9, MixedProviderRateMultipliers: map[string]float64{"kimi": 0.2, "zhipu": 0}}
	svc := newPlazaService([]Channel{plazaPricedChannel(1, "shop", []int64{2}, PlatformMixed, "kimi-k2")}, []Group{group}, nil)
	out, err := svc.ListGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 1)
	require.Equal(t, "kimi", out[0].Models[0].ModelVendor)
	require.NotNil(t, out[0].Models[0].RateMultiplier)
	require.Equal(t, 0.2, *out[0].Models[0].RateMultiplier)
}

func TestMixedVendorPlazaMappedBillingIdentities(t *testing.T) {
	group := Group{ID: 2, Name: "Mixed", Platform: PlatformMixed, RateMultiplier: 9, MixedProviderRateMultipliers: map[string]float64{"kimi": 0.2, "deepseek": 0.8, "zhipu": 0.4}}
	repo := &modelsListAccountRepoStub{byGroup: map[int64][]Account{2: {
		{Platform: PlatformMixed, Credentials: map[string]any{"model_mapping": map[string]any{"kimi-k2": "deepseek-v4-pro"}}},
		{Platform: PlatformMixed, Credentials: map[string]any{"model_mapping": map[string]any{"kimi-k2": "glm-5"}}},
	}}}
	gateway := &GatewayService{accountRepo: repo}
	svc := newPlazaService(nil, []Group{group}, nil)
	svc.SetListedModelCatalog(plazaGatewayModelCatalog{gateway: gateway})
	out, err := svc.ListGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 2)
	for _, m := range out[0].Models {
		require.Equal(t, "kimi-k2", m.Name, "public model identity must remain unchanged")
		require.Contains(t, []string{"deepseek-v4-pro", "glm-5"}, m.BillingModel)
		require.Equal(t, ModelVendor(m.BillingModel), m.ModelVendor)
		require.Equal(t, group.RateMultiplierForModel(m.BillingModel), *m.RateMultiplier)
	}
}

func TestMixedVendorPlazaMappedPriceDoesNotInheritAliasCard(t *testing.T) {
	billing := newGatewayRecordUsageServiceForTest(&openAIRecordUsageLogRepoStub{}, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}).billingService
	billing.fallbackPrices = mixedRateTestPricing()
	group := Group{ID: 2, Platform: PlatformMixed, ModelPricing: []ChannelModelPricing{
		{Models: []string{"kimi-k2"}, BillingMode: BillingModeImage, PerRequestPrice: testPtrFloat64(10)},
		{Models: []string{"glm-5"}, BillingMode: BillingModePerRequest, PerRequestPrice: testPtrFloat64(3)},
	}}
	repo := &modelsListAccountRepoStub{byGroup: map[int64][]Account{2: {
		{Platform: PlatformMixed, Credentials: map[string]any{"model_mapping": map[string]any{"kimi-k2": "glm-5"}}},
		{Platform: PlatformMixed, Credentials: map[string]any{"model_mapping": map[string]any{"kimi-k2": "qa-unpriced-target"}}},
	}}}
	svc := newPlazaService([]Channel{plazaPricedChannel(1, "alias", []int64{2}, PlatformMixed, "kimi-k2")}, []Group{group}, nil)
	svc.billingService = billing
	svc.resolver = NewModelPricingResolver(nil, billing)
	svc.SetListedModelCatalog(plazaGatewayModelCatalog{gateway: &GatewayService{accountRepo: repo}})
	out, err := svc.ListGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 2)
	for _, m := range out[0].Models {
		switch m.BillingModel {
		case "glm-5":
			require.NotNil(t, m.Pricing)
			require.Equal(t, BillingModePerRequest, m.Pricing.BillingMode)
			require.Equal(t, 3.0, *m.Pricing.PerRequestPrice)
		case "qa-unpriced-target":
			require.Nil(t, m.Pricing, "unknown candidate must not borrow the alias's image/token price")
		default:
			t.Fatalf("unexpected billing model %q", m.BillingModel)
		}
	}
}

func TestMixedVendorInflightNormalizedMappedIdentity(t *testing.T) {
	billing := newGatewayRecordUsageServiceForTest(&openAIRecordUsageLogRepoStub{}, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}).billingService
	id := int64(2)
	key := &APIKey{User: &User{ID: 1}, GroupID: &id, Group: &Group{ID: id, Platform: PlatformMixed, MixedProviderRateMultipliers: map[string]float64{"other": 0, "deepseek": 0.8}}}
	d := inflightEstimateDeps{billing: billing, resolver: NewModelPricingResolver(nil, billing), accountMappedModels: func(context.Context, *APIKey, string) []string {
		return []string{"provider/deepseek-v4-flash"}
	}}
	got, priced := d.estimate(context.Background(), key, InflightEstimateRequest{Model: "qa-unpriced-alias", BodyBytes: 400, MaxTokens: 50})
	require.True(t, priced)
	price, err := billing.GetModelPricing("deepseek-v4-flash")
	require.NoError(t, err)
	require.InDelta(t, 0.8*(100*price.InputPricePerToken+50*price.OutputPricePerToken), got, 1e-12)
	require.Positive(t, got, "untrusted prefix cannot make a normalized paid model skip its hold")
	d.resolveMapping = func(context.Context, int64, string) ChannelMappingResult {
		return ChannelMappingResult{MappedModel: "provider/deepseek-v4-flash", BillingModelSource: BillingModelSourceRequested}
	}
	got, priced = d.estimate(context.Background(), key, InflightEstimateRequest{Model: "provider/deepseek-v4-flash", BodyBytes: 400, MaxTokens: 50})
	require.True(t, priced)
	require.Zero(t, got, "priced requested identity retains its original vendor")
	d.resolveMapping = func(context.Context, int64, string) ChannelMappingResult {
		return ChannelMappingResult{MappedModel: "provider/deepseek-v4-flash", BillingModelSource: BillingModelSourceResponse}
	}
	got, priced = d.estimate(context.Background(), key, InflightEstimateRequest{Model: "qa-unpriced-alias", BodyBytes: 400, MaxTokens: 50})
	require.True(t, priced)
	require.Zero(t, got, "response-model baseline keeps the original identity before a declaration")
	d.resolveMapping = nil
	key.Group.ModelPricing = []ChannelModelPricing{{Models: []string{"provider/deepseek-v4-flash"}, InputPrice: testPtrFloat64(1e-6), OutputPrice: testPtrFloat64(2e-6)}}
	got, priced = d.estimate(context.Background(), key, InflightEstimateRequest{Model: "qa-unpriced-alias", BodyBytes: 400, MaxTokens: 50})
	require.True(t, priced)
	require.Zero(t, got, "explicit original-identity pricing must prevent normalization")
}

func TestMixedVendorCatalogueResponseSourceRetainsBaselineIdentity(t *testing.T) {
	billing := newGatewayRecordUsageServiceForTest(&openAIRecordUsageLogRepoStub{}, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}).billingService
	ch := Channel{ID: 1, Status: StatusActive, GroupIDs: []int64{2}, BillingModelSource: BillingModelSourceResponse}
	channels := newTestChannelService(makeStandardRepo(ch, map[int64]string{2: PlatformMixed}))
	accounts := &modelsListAccountRepoStub{byGroup: map[int64][]Account{2: {
		{Platform: PlatformMixed, Credentials: map[string]any{"model_mapping": map[string]any{"qa-unpriced-alias": "provider/deepseek-v4-flash"}}},
	}}}
	group := Group{ID: 2, Platform: PlatformMixed, MixedProviderRateMultipliers: map[string]float64{"other": 0, "deepseek": 0.8}}
	gateway := &GatewayService{accountRepo: accounts, billingService: billing, channelService: channels, resolver: NewModelPricingResolver(channels, billing)}
	cat := gateway.ensureGroupModelsCatalog(context.Background(), &group.ID)
	require.Equal(t, []string{"provider/deepseek-v4-flash"}, gateway.mixedCatalogueBillingModels(context.Background(), &group, "qa-unpriced-alias", cat))
	gateway.SupportedKeyProviders(context.Background(), &group)
	require.Equal(t, []string{"other"}, group.SupportedModelVendors)
}

func TestMixedVendorCatalogueExplicitSourcePricingFallback(t *testing.T) {
	for _, source := range []string{BillingModelSourceRequested, BillingModelSourceChannelMapped} {
		t.Run(source, func(t *testing.T) {
			billing := newGatewayRecordUsageServiceForTest(&openAIRecordUsageLogRepoStub{}, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}).billingService
			ch := Channel{ID: 1, Name: "test", Status: StatusActive, GroupIDs: []int64{2}, BillingModelSource: source}
			channels := newTestChannelService(makeStandardRepo(ch, map[int64]string{2: PlatformMixed}))
			accounts := &modelsListAccountRepoStub{byGroup: map[int64][]Account{2: {
				{Platform: PlatformMixed, Credentials: map[string]any{"model_mapping": map[string]any{"qa-unpriced-alias": "glm-5"}}},
			}}}
			group := Group{ID: 2, Platform: PlatformMixed, MixedProviderRateMultipliers: map[string]float64{"other": 0, "zhipu": 0.4}}
			gateway := &GatewayService{accountRepo: accounts, billingService: billing, channelService: channels, resolver: NewModelPricingResolver(channels, billing)}
			gateway.SupportedKeyProviders(context.Background(), &group)
			require.Equal(t, []string{"zhipu"}, group.SupportedModelVendors)
			svc := newPlazaService(nil, []Group{group}, nil)
			svc.billingService, svc.resolver = billing, gateway.resolver
			svc.SetListedModelCatalog(plazaGatewayModelCatalog{gateway: gateway})
			out, err := svc.ListGroups(context.Background())
			require.NoError(t, err)
			require.Len(t, out, 1)
			require.Len(t, out[0].Models, 1)
			require.Equal(t, "glm-5", out[0].Models[0].BillingModel)
			require.Equal(t, 0.4, *out[0].Models[0].RateMultiplier)
			// A requested price fixes the identity; unchanged channel_mapped
			// must still use the concrete account identity on both protocols.
			group.ModelPricing = []ChannelModelPricing{{Models: []string{"qa-unpriced-alias"}, InputPrice: testPtrFloat64(0)}}
			cat := gateway.ensureGroupModelsCatalog(context.Background(), &group.ID)
			expected := []string{"glm-5"}
			if source == BillingModelSourceRequested {
				expected = []string{"qa-unpriced-alias"}
			}
			require.Equal(t, expected, gateway.mixedCatalogueBillingModels(context.Background(), &group, "qa-unpriced-alias", cat))
		})
	}
}
