//go:build unit

package service

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func cacheCreateQuoteFixture(inputPrice, storagePrice float64) (*BillingService, GeminiCacheCreateQuoteInput) {
	pricing := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"cache-quote-model": {InputCostPerToken: inputPrice, CacheStorageCostPerTokenPerHour: &storagePrice},
	}}
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 99
	at := time.Date(2026, 10, 15, 1, 0, 0, 0, time.UTC)
	return NewBillingService(cfg, pricing), GeminiCacheCreateQuoteInput{
		GeminiCacheStorageQuoteInput: GeminiCacheStorageQuoteInput{
			Model: "cache-quote-model", TotalTokenCount: 1000,
			PricingAt: at, ExpireTime: at.Add(90 * time.Minute), RateMultiplier: 0.25,
		},
	}
}

func TestQuoteGeminiCacheCreate_FractionalDurationAndAbsoluteMultiplier(t *testing.T) {
	billing, input := cacheCreateQuoteFixture(1e-6, 2e-6)
	for _, multiplier := range []float64{0, 0.25, 2} {
		t.Run(strconv.FormatFloat(multiplier, 'g', -1, 64), func(t *testing.T) {
			input.RateMultiplier = multiplier
			quote, err := billing.QuoteGeminiCacheCreate(context.Background(), input)
			require.NoError(t, err)
			require.Equal(t, input.GeminiCacheStorageQuoteInput, quote.GeminiCacheStorageQuoteInput)
			require.Equal(t, input.GeminiCacheStorageQuoteInput, quote.StorageQuote.GeminiCacheStorageQuoteInput)
			require.Equal(t, float64(1500), quote.StorageQuote.TokenHours)
			require.InDelta(t, 0.001, quote.InputQuote.TotalCost, 1e-15)
			require.InDelta(t, 0.003, quote.StorageQuote.TotalCost, 1e-15)
			require.InDelta(t, 0.001*multiplier, quote.InputQuote.ActualCost, 1e-15)
			require.InDelta(t, 0.003*multiplier, quote.StorageQuote.ActualCost, 1e-15)
			require.Equal(t, 0.004*multiplier, quote.ChargeAmount)
			require.Equal(t, quote.ChargeAmount, quote.ReservationAmount)
			require.Equal(t, 0.001*multiplier, quote.InputOnlyChargeAmount)
			require.Equal(t, string(BillingModeToken), quote.InputQuote.BillingMode)
			require.Zero(t, quote.InputQuote.OutputCost)
			require.Zero(t, quote.InputQuote.CacheCreationCost)
			require.Zero(t, quote.InputQuote.CacheReadCost)
			again, err := billing.QuoteGeminiCacheCreate(context.Background(), input)
			require.NoError(t, err)
			require.Equal(t, quote, again)
		})
	}
}

func TestQuoteGeminiCacheCreate_SumsRawAmountsBeforeRounding(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		input, storage, charge, hold float64
		inputOnly                    float64
	}{
		{"below unit crosses half", 3e-9, 3e-9, 1e-8, 1e-8, 0},
		{"half away", 2e-9, 3e-9, 1e-8, 1e-8, 0},
		{"avoid rounding both up", 6e-9, 6e-9, 1e-8, 2e-8, 1e-8},
		{"reserve positive dust", 1e-10, 1e-10, 0, 1e-8, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			billing, input := cacheCreateQuoteFixture(tc.input, tc.storage)
			input.TotalTokenCount, input.RateMultiplier = 1, 1
			input.ExpireTime = input.PricingAt.Add(time.Hour)
			quote, err := billing.QuoteGeminiCacheCreate(context.Background(), input)
			require.NoError(t, err)
			require.Equal(t, tc.input, quote.InputQuote.ActualCost)
			require.Equal(t, tc.storage, quote.StorageQuote.ActualCost)
			require.Equal(t, tc.charge, quote.ChargeAmount)
			require.Equal(t, tc.hold, quote.ReservationAmount)
			require.Equal(t, tc.inputOnly, quote.InputOnlyChargeAmount)
		})
	}
}

func TestQuoteGeminiCacheCreate_InputOnlyCompensationQuote(t *testing.T) {
	billing, input := cacheCreateQuoteFixture(1.25e-6, 2e-6)
	input.TotalTokenCount, input.RateMultiplier = 10, 1.25
	quote, err := billing.QuoteGeminiCacheCreate(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 0.00001563, quote.InputOnlyChargeAmount)
	require.Equal(t, 0.00005313, quote.ChargeAmount)
	require.Equal(t, 0.00005313, quote.ReservationAmount)
	require.InDelta(t, 0.000015625, quote.InputQuote.ActualCost, 1e-20)
}

func TestQuoteGeminiCacheCreate_NativeModelAndZeroUsage(t *testing.T) {
	billing, input := cacheCreateQuoteFixture(1e-6, 2e-6)
	input.Model = "gemini-3.8-flash"
	native, err := billing.CalculateCostUnified(CostInput{
		Ctx: context.Background(), Model: input.Model,
		Tokens:         UsageTokens{InputTokens: int(input.TotalTokenCount)},
		RateMultiplier: input.RateMultiplier, PricingAt: input.PricingAt,
		Resolver: NewModelPricingResolver(nil, billing),
	})
	require.NoError(t, err)
	quote, err := billing.QuoteGeminiCacheCreate(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, *native, quote.InputQuote)
	require.Equal(t, geminiFlashCacheStoragePrice, quote.StorageQuote.PricePerTokenHour)

	input.ExpireTime = input.PricingAt
	quote, err = billing.QuoteGeminiCacheCreate(context.Background(), input)
	require.NoError(t, err)
	require.Zero(t, quote.StorageQuote.ActualCost)
	require.Equal(t, quote.InputOnlyChargeAmount, quote.ChargeAmount)

	input.TotalTokenCount = 0
	quote, err = billing.QuoteGeminiCacheCreate(context.Background(), input)
	require.NoError(t, err)
	require.Zero(t, quote.ChargeAmount)
	require.Zero(t, quote.ReservationAmount)
}

func TestQuoteGeminiCacheCreate_NativeChannelInputAndFrozenTimePricing(t *testing.T) {
	billing, input := cacheCreateQuoteFixture(1e-6, 2e-6)
	cs := newTestChannelServiceWithCache(t, &channelCache{
		pricingByGroupModel: map[channelModelKey]*ChannelModelPricing{
			{groupID: 1, model: input.Model}: {
				BillingMode: BillingModeToken, InputPrice: testPtrFloat64(2e-6),
				OutputPrice: testPtrFloat64(100), CacheWritePrice: testPtrFloat64(100),
				MaxReasoningEffortMultiplier: testPtrFloat64(50),
				TimePricing: &ChannelTimePricing{Timezone: "Asia/Shanghai", Periods: []ChannelTimePricingPeriod{{
					StartTime: "09:00", EndTime: "12:00", Multiplier: 2,
				}}},
			},
		},
		channelByGroupID:        map[int64]*Channel{1: {ID: 1, Status: StatusActive}},
		groupPlatform:           map[int64]string{1: ""},
		wildcardByGroupPlatform: map[channelGroupPlatformKey][]*wildcardPricingEntry{},
		mappingByGroupModel:     map[channelModelKey]string{},
		wildcardMappingByGP:     map[channelGroupPlatformKey][]*wildcardMappingEntry{},
		byID:                    map[int64]*Channel{},
	})
	groupID := int64(1)
	input.GroupID, input.Resolver = &groupID, NewModelPricingResolver(cs, billing)
	quote, err := billing.QuoteGeminiCacheCreate(context.Background(), input)
	require.NoError(t, err)
	require.InDelta(t, 0.004, quote.InputQuote.TotalCost, 1e-15)
	require.InDelta(t, 0.001, quote.InputQuote.ActualCost, 1e-15)
	require.InDelta(t, 0.00075, quote.StorageQuote.ActualCost, 1e-15)
	require.Equal(t, 0.00175, quote.ChargeAmount)
	require.Zero(t, quote.InputQuote.OutputCost)
	require.Zero(t, quote.InputQuote.CacheCreationCost)

	input.PricingAt = input.PricingAt.Add(3 * time.Hour)
	input.ExpireTime = input.ExpireTime.Add(3 * time.Hour)
	quote, err = billing.QuoteGeminiCacheCreate(context.Background(), input)
	require.NoError(t, err)
	require.InDelta(t, 0.0005, quote.InputQuote.ActualCost, 1e-15)
	require.Equal(t, 0.00125, quote.ChargeAmount)
}

func TestQuoteGeminiCacheCreate_GroupOverrideWithoutResolver(t *testing.T) {
	billing, input := cacheCreateQuoteFixture(1e-6, 2e-6)
	input.Group = &Group{ModelPricing: []ChannelModelPricing{{
		Models: []string{input.Model}, InputPrice: testPtrFloat64(3e-6),
	}}}
	quote, err := billing.QuoteGeminiCacheCreate(context.Background(), input)
	require.NoError(t, err)
	require.InDelta(t, 0.00075, quote.InputQuote.ActualCost, 1e-15)
	require.Equal(t, 0.0015, quote.ChargeAmount)
}

func TestQuoteGeminiCacheCreate_RequiresBothPricingIdentities(t *testing.T) {
	for _, tc := range []struct {
		name, catalog string
	}{
		{"missing token card even with free storage", `{"custom":{"cache_storage_cost_per_token_per_hour":0}}`},
		{"missing storage card", `{"custom":{"input_cost_per_token":0.000001}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			billing, input := cacheCreateQuoteFixture(1e-6, 2e-6)
			data, err := billing.pricingService.parsePricingData([]byte(tc.catalog))
			require.NoError(t, err)
			billing.pricingService.pricingData = data
			input.Model, input.RateMultiplier, input.TotalTokenCount = "custom", 0, 0
			quote, err := billing.QuoteGeminiCacheCreate(context.Background(), input)
			require.ErrorIs(t, err, ErrModelPricingUnavailable)
			require.Nil(t, quote)
		})
	}

	billing, input := cacheCreateQuoteFixture(0, 0)
	quote, err := billing.QuoteGeminiCacheCreate(context.Background(), input)
	require.NoError(t, err, "explicit free token and storage cards remain valid")
	require.Zero(t, quote.ChargeAmount)
}

func TestQuoteGeminiCacheCreate_ConfiguredCardNeedsInputPricing(t *testing.T) {
	for _, tc := range []struct {
		name string
		card ChannelModelPricing
		ok   bool
	}{
		{"output only", ChannelModelPricing{OutputPrice: testPtrFloat64(1)}, false},
		{"cache only", ChannelModelPricing{CacheWritePrice: testPtrFloat64(1)}, false},
		{"explicit zero input", ChannelModelPricing{InputPrice: testPtrFloat64(0)}, true},
		{"explicit input", ChannelModelPricing{InputPrice: testPtrFloat64(1e-6)}, true},
		{"per request", ChannelModelPricing{BillingMode: BillingModePerRequest, PerRequestPrice: testPtrFloat64(1)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			billing, input := cacheCreateQuoteFixture(0, 0)
			billing.pricingService.pricingData[input.Model].TokenPricingAbsent = true
			tc.card.Models = []string{input.Model}
			input.Group = &Group{ModelPricing: []ChannelModelPricing{tc.card}}
			quote, err := billing.QuoteGeminiCacheCreate(context.Background(), input)
			if tc.ok {
				require.NoError(t, err)
				require.NotNil(t, quote)
			} else {
				require.ErrorIs(t, err, ErrModelPricingUnavailable)
				require.Nil(t, quote)
			}
		})
	}
}

func TestQuoteGeminiCacheCreate_OutputOnlyCatalogDoesNotMakeInputFree(t *testing.T) {
	for _, configuredOutput := range []bool{false, true} {
		for _, explicitInput := range []bool{false, true} {
			t.Run(fmt.Sprintf("output_override=%t/explicit_input=%t", configuredOutput, explicitInput), func(t *testing.T) {
				billing, input := cacheCreateQuoteFixture(0, 0)
				catalog := `{"cache-quote-model":{"output_cost_per_token":0.000002,"cache_storage_cost_per_token_per_hour":0}}`
				if explicitInput {
					catalog = `{"cache-quote-model":{"input_cost_per_token":0,"output_cost_per_token":0.000002,"cache_storage_cost_per_token_per_hour":0}}`
				}
				data, err := billing.pricingService.parsePricingData([]byte(catalog))
				require.NoError(t, err)
				billing.pricingService.pricingData = data
				if configuredOutput {
					input.Group = &Group{ModelPricing: []ChannelModelPricing{{
						Models: []string{input.Model}, OutputPrice: testPtrFloat64(3e-6),
					}}}
				}
				quote, err := billing.QuoteGeminiCacheCreate(context.Background(), input)
				if !explicitInput {
					require.ErrorIs(t, err, ErrModelPricingUnavailable)
					require.Nil(t, quote)
				} else {
					require.NoError(t, err)
					require.Zero(t, quote.ChargeAmount)
				}
			})
		}
	}
}

func TestQuantizeGeminiCacheCreateAmount_RejectsLossyFloatRoundtrip(t *testing.T) {
	amount := decimal.RequireFromString("100000000000.000001")
	for _, roundUp := range []bool{false, true} {
		_, err := quantizeGeminiCacheCreateAmount(amount, roundUp)
		require.Error(t, err, "eight-place money must not silently lose precision on float conversion")
	}
	billing, input := cacheCreateQuoteFixture(1e11, 1e-6)
	input.TotalTokenCount, input.RateMultiplier = 1, 1
	input.ExpireTime = input.PricingAt.Add(time.Hour)
	quote, err := billing.QuoteGeminiCacheCreate(context.Background(), input)
	require.Error(t, err)
	require.Nil(t, quote)
}

func TestQuoteGeminiCacheCreate_ConfiguredInputPresenceOverridesCatalog(t *testing.T) {
	for _, tc := range []struct {
		name string
		card ChannelModelPricing
		ok   bool
	}{
		{"explicit free flat", ChannelModelPricing{InputPrice: testPtrFloat64(0)}, true},
		{"explicit free interval", ChannelModelPricing{Intervals: []PricingInterval{{InputPrice: testPtrFloat64(0)}}}, true},
		{"output only interval", ChannelModelPricing{Intervals: []PricingInterval{{OutputPrice: testPtrFloat64(1)}}}, false},
		{"input multiplier cannot create missing price", ChannelModelPricing{Intervals: []PricingInterval{{InputMultiplier: testPtrFloat64(0)}}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			billing, input := cacheCreateQuoteFixture(0, 0)
			data, err := billing.pricingService.parsePricingData([]byte(`{"cache-quote-model":{"output_cost_per_token":0.000002,"cache_storage_cost_per_token_per_hour":0}}`))
			require.NoError(t, err)
			billing.pricingService.pricingData = data
			groupID := int64(1)
			cs := newTestChannelServiceWithCache(t, &channelCache{
				pricingByGroupModel: map[channelModelKey]*ChannelModelPricing{{groupID: groupID, model: input.Model}: &tc.card},
				channelByGroupID:    map[int64]*Channel{groupID: {ID: 1, Status: StatusActive}},
				groupPlatform:       map[int64]string{groupID: ""},
			})
			input.GroupID, input.Resolver = &groupID, NewModelPricingResolver(cs, billing)
			quote, err := billing.QuoteGeminiCacheCreate(context.Background(), input)
			if tc.ok {
				require.NoError(t, err)
				require.Zero(t, quote.ChargeAmount)
			} else {
				require.ErrorIs(t, err, ErrModelPricingUnavailable)
				require.Nil(t, quote)
			}
		})
	}
}

func TestQuoteGeminiCacheCreate_InvalidInputs(t *testing.T) {
	billing, valid := cacheCreateQuoteFixture(1e-6, 2e-6)
	for _, tc := range []struct {
		name string
		edit func(*GeminiCacheCreateQuoteInput)
	}{
		{"negative tokens", func(i *GeminiCacheCreateQuoteInput) { i.TotalTokenCount = -1 }},
		{"negative multiplier", func(i *GeminiCacheCreateQuoteInput) { i.RateMultiplier = -1 }},
		{"NaN multiplier", func(i *GeminiCacheCreateQuoteInput) { i.RateMultiplier = math.NaN() }},
		{"infinite multiplier", func(i *GeminiCacheCreateQuoteInput) { i.RateMultiplier = math.Inf(1) }},
		{"missing model", func(i *GeminiCacheCreateQuoteInput) { i.Model = "" }},
		{"missing pricing time", func(i *GeminiCacheCreateQuoteInput) { i.PricingAt = time.Time{} }},
		{"missing expiry", func(i *GeminiCacheCreateQuoteInput) { i.ExpireTime = time.Time{} }},
		{"negative duration", func(i *GeminiCacheCreateQuoteInput) { i.ExpireTime = i.PricingAt.Add(-time.Second) }},
		{"duration overflow", func(i *GeminiCacheCreateQuoteInput) { i.ExpireTime = i.PricingAt.AddDate(500, 0, 0) }},
		{"empty resolver", func(i *GeminiCacheCreateQuoteInput) { i.Resolver = &ModelPricingResolver{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := valid
			tc.edit(&input)
			quote, err := billing.QuoteGeminiCacheCreate(context.Background(), input)
			require.Error(t, err)
			require.Nil(t, quote)
		})
	}
	var nilBilling *BillingService
	quote, err := nilBilling.QuoteGeminiCacheCreate(context.Background(), valid)
	require.Error(t, err)
	require.Nil(t, quote)
	if strconv.IntSize == 32 {
		valid.TotalTokenCount = int64(math.MaxInt32) + 1
		quote, err = billing.QuoteGeminiCacheCreate(context.Background(), valid)
		require.ErrorContains(t, err, "int range")
		require.Nil(t, quote)
	}
}

func TestQuoteGeminiCacheCreate_InvalidPricesAndAmountOverflow(t *testing.T) {
	for _, tc := range []struct {
		name           string
		input, storage float64
		multiplier     float64
	}{
		{"negative input", -1, 0, 0},
		{"NaN input", math.NaN(), 0, 0},
		{"infinite input", math.Inf(1), 0, 0},
		{"negative storage", 0, -1, 0},
		{"NaN storage", 0, math.NaN(), 0},
		{"infinite storage", 0, math.Inf(1), 0},
		{"input multiplication overflow", math.MaxFloat64, 0, 2},
		{"storage multiplication overflow", 0, math.MaxFloat64, 2},
		{"input numeric overflow", 1e12, 0, 1},
		{"storage numeric overflow", 0, 1e12, 1},
		{"combined numeric overflow", 6e11, 6e11, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			billing, input := cacheCreateQuoteFixture(tc.input, tc.storage)
			input.TotalTokenCount, input.RateMultiplier = 1, tc.multiplier
			input.ExpireTime = input.PricingAt.Add(time.Hour)
			quote, err := billing.QuoteGeminiCacheCreate(context.Background(), input)
			require.Error(t, err)
			require.Nil(t, quote)
		})
	}
}

func TestQuantizeGeminiCacheCreateAmount_NumericBoundary(t *testing.T) {
	for _, amount := range []string{"1000000000000", "999999999999.999999999", "999999999999.99999999", "-0.000000001"} {
		for _, roundUp := range []bool{false, true} {
			_, err := quantizeGeminiCacheCreateAmount(decimal.RequireFromString(amount), roundUp)
			require.Error(t, err, "amount=%s roundUp=%t", amount, roundUp)
		}
	}
	value, err := quantizeGeminiCacheCreateAmount(decimal.NewFromFloat(math.Nextafter(1e12, 0)), true)
	require.NoError(t, err)
	require.Less(t, value, 1e12)
}
