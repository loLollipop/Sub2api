//go:build unit

package service

import (
	"math"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestBuiltinGeminiCacheStoragePrice(t *testing.T) {
	billing := NewBillingService(&config.Config{}, nil)
	for _, tc := range []struct {
		model string
		want  float64
	}{
		{"gemini-3.8-flash", 1e-6},
		{"gemini-3.7-flash", 1e-6},
		{"gemini-3.6-flash", 1e-6},
		{"gemini-3.8-flash-cyber", 1e-6},
		{"gemini-3.5-flash", 1e-6},
		{"gemini-3.5-flash-lite", 1e-6},
		{"gemini-3.1-flash-lite", 1e-6},
		{"gemini-3.1-pro-preview", 4.5e-6},
		{"gemini-3.1-pro-preview-customtools", 4.5e-6},
		{"gemini-3-flash-preview", 1e-6},
		{"gemini-3-pro-preview", 4.5e-6},
		{"gemini-2.5-pro", 4.5e-6},
		{"gemini-2.5-flash", 1e-6},
		{"gemini-2.5-flash-lite", 1e-6},
		{"gemini-2.0-flash-001", 1e-6},
		{"gemini-2.0-flash-lite-001", 1e-6},
		{"models/gemini-3.8-flash", 1e-6},
		{"google/gemini-2.5-pro", 4.5e-6},
		{"projects/p/locations/global/publishers/google/models/gemini-2.5-pro", 4.5e-6},
		{" Gemini-3.8-Flash ", 1e-6},
		{"gemini-3.8-flash-high", 1e-6},
	} {
		price, err := billing.CacheStoragePricePerTokenHour(tc.model)
		require.NoError(t, err, tc.model)
		require.Equal(t, tc.want, price, tc.model)
	}
	for _, model := range []string{"", "customer-alias", "gemini-3.9-flash", "gemini-3.9-pro-preview", "gemini-3.8-flash-lite", "gemini-pro-latest", "gemini-embedding-001", "gemini-2.5-pro-unrecognized", "claude-sonnet-5-5"} {
		_, err := billing.CacheStoragePricePerTokenHour(model)
		require.ErrorIs(t, err, ErrModelPricingUnavailable, model)
	}
}

func TestCacheStoragePrice_IdentifiedCatalogPriority(t *testing.T) {
	svc := &PricingService{}
	data, err := svc.parsePricingData([]byte(`{
		"gemini-3.8-flash":{"cache_storage_cost_per_token_per_hour":0.000002},
		"gemini-3.8-flash-high":{"cache_storage_cost_per_token_per_hour":0.000003},
		"gemini-3.8-flash-low":{"input_cost_per_token":0.000001},
		"google/gemini-3.8-flash":{"cache_storage_cost_per_token_per_hour":0.000004},
		"other-provider/gemini-3.8-flash":{"input_cost_per_token":0.000001},
		"vertex_ai/gemini-2.5-pro":{"cache_storage_cost_per_token_per_hour":0.000005},
		"gemini-3.1-pro":{"cache_storage_cost_per_token_per_hour":0},
		"customer-alias":{"cache_storage_cost_per_token_per_hour":0.000006},
		"gemini-unknown-flash-20261014":{"cache_storage_cost_per_token_per_hour":0.000007},
		"gemini-unknown-flash-20261015":{"cache_storage_cost_per_token_per_hour":0.000008}
	}`))
	require.NoError(t, err)
	svc.pricingData = data
	billing := NewBillingService(&config.Config{}, svc)
	for _, tc := range []struct {
		model string
		want  float64
	}{
		{"gemini-3.8-flash", 2e-6},
		{"gemini-3.8-flash-low", 2e-6},
		{"models/gemini-3.8-flash-high", 3e-6},
		{"google/gemini-3.8-flash", 4e-6},
		{"other-provider/gemini-3.8-flash", 2e-6},
		{"projects/p/locations/global/publishers/google/models/gemini-2.5-pro", 5e-6},
		{"gemini-3.1-pro-preview-customtools", 0},
		{"customer-alias", 6e-6},
		{"gemini-unknown-flash-20261015", 8e-6},
	} {
		for range 25 {
			price, err := billing.CacheStoragePricePerTokenHour(tc.model)
			require.NoError(t, err, tc.model)
			require.Equal(t, tc.want, price, tc.model)
		}
	}
	for _, model := range []string{"customer-alias-unknown", "gemini-unknown-flash", "gemini-unknown-flash-20261016", "claude-unknown-haiku"} {
		_, err := billing.CacheStoragePricePerTokenHour(model)
		require.ErrorIs(t, err, ErrModelPricingUnavailable, "must not match unrelated/version sibling: %s", model)
	}
}

func TestCacheStoragePrice_AmbiguousCatalogFailsClosed(t *testing.T) {
	a, b := 2e-6, 3e-6
	svc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"google/gemini-3.8-flash":    {CacheStorageCostPerTokenPerHour: &a},
		"vertex_ai/gemini-3.8-flash": {CacheStorageCostPerTokenPerHour: &b},
	}}
	billing := NewBillingService(&config.Config{}, svc)
	for range 25 {
		_, err := billing.CacheStoragePricePerTokenHour("gemini-3.8-flash")
		require.ErrorContains(t, err, "ambiguous")
	}
	price, err := billing.CacheStoragePricePerTokenHour("google/gemini-3.8-flash")
	require.NoError(t, err)
	require.Equal(t, a, price, "exact provider entry is unambiguous")
}

func TestCacheStoragePrice_InvalidExplicitNeverFallsBack(t *testing.T) {
	for _, rate := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		svc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
			"gemini-3.8-flash": {CacheStorageCostPerTokenPerHour: &rate},
		}}
		_, err := NewBillingService(&config.Config{}, svc).CacheStoragePricePerTokenHour("gemini-3.8-flash")
		require.Error(t, err)
	}
}

func TestQuoteGeminiCacheStorage_ExactTokenHoursAndAbsoluteMultiplier(t *testing.T) {
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 99
	billing := NewBillingService(cfg, nil)
	at := time.Date(2026, 12, 31, 23, 59, 59, 0, time.FixedZone("test", 8*3600))
	for _, tc := range []struct {
		tokens     int64
		duration   time.Duration
		multiplier float64
		wantHours  float64
		wantTotal  float64
		wantActual float64
	}{
		{1_000_000, 90 * time.Minute, 0.25, 1_500_000, 1.5, 0.375},
		{1_000_000, time.Hour, 0, 1_000_000, 1, 0},
		{1_000_000, time.Hour, 2, 1_000_000, 1, 2},
		{0, time.Hour, 1, 0, 0, 0},
		{100, 0, 1, 0, 0, 0},
		{1, time.Millisecond, 1, 1.0 / 3_600_000, 1e-6 / 3_600_000, 1e-6 / 3_600_000},
	} {
		input := GeminiCacheStorageQuoteInput{
			Model:           "projects/p/locations/global/publishers/google/models/gemini-3.8-flash",
			TotalTokenCount: tc.tokens, PricingAt: at, ExpireTime: at.Add(tc.duration), RateMultiplier: tc.multiplier,
		}
		quote, err := billing.QuoteGeminiCacheStorage(input)
		require.NoError(t, err)
		require.Equal(t, input, quote.GeminiCacheStorageQuoteInput, "preserve the actual target and frozen interval")
		require.InDelta(t, tc.wantHours, quote.TokenHours, 1e-12)
		require.InDelta(t, tc.wantTotal, quote.TotalCost, 1e-20)
		require.InDelta(t, tc.wantActual, quote.ActualCost, 1e-20)
		require.Equal(t, geminiFlashCacheStoragePrice, quote.PricePerTokenHour, "standard rate; no invented promotional cutoff")
		again, err := billing.QuoteGeminiCacheStorage(input)
		require.NoError(t, err)
		require.Equal(t, quote, again)
	}
}

func TestQuoteGeminiCacheStorage_InvalidInputs(t *testing.T) {
	billing := NewBillingService(&config.Config{}, nil)
	at := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	valid := GeminiCacheStorageQuoteInput{Model: "gemini-2.5-pro", TotalTokenCount: 100, PricingAt: at, ExpireTime: at.Add(time.Hour), RateMultiplier: 1}
	for _, tc := range []struct {
		name string
		edit func(*GeminiCacheStorageQuoteInput)
	}{
		{"negative tokens", func(i *GeminiCacheStorageQuoteInput) { i.TotalTokenCount = -1 }},
		{"negative multiplier", func(i *GeminiCacheStorageQuoteInput) { i.RateMultiplier = -1 }},
		{"NaN multiplier", func(i *GeminiCacheStorageQuoteInput) { i.RateMultiplier = math.NaN() }},
		{"infinite multiplier", func(i *GeminiCacheStorageQuoteInput) { i.RateMultiplier = math.Inf(1) }},
		{"negative infinite multiplier", func(i *GeminiCacheStorageQuoteInput) { i.RateMultiplier = math.Inf(-1) }},
		{"missing pricing time", func(i *GeminiCacheStorageQuoteInput) { i.PricingAt = time.Time{} }},
		{"missing expiry", func(i *GeminiCacheStorageQuoteInput) { i.ExpireTime = time.Time{} }},
		{"negative duration", func(i *GeminiCacheStorageQuoteInput) { i.ExpireTime = at.Add(-time.Second) }},
		{"duration overflow", func(i *GeminiCacheStorageQuoteInput) { i.ExpireTime = at.AddDate(500, 0, 0) }},
		{"unknown with zero multiplier", func(i *GeminiCacheStorageQuoteInput) { i.Model = "gemini-unknown-pro"; i.RateMultiplier = 0 }},
		{"unknown with no tokens", func(i *GeminiCacheStorageQuoteInput) { i.Model = "unknown"; i.TotalTokenCount = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := valid
			tc.edit(&input)
			quote, err := billing.QuoteGeminiCacheStorage(input)
			require.Error(t, err)
			require.Nil(t, quote)
		})
	}
}

func TestQuoteGeminiCacheStorage_AmountOverflow(t *testing.T) {
	at := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	price := math.MaxFloat64
	svc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"custom": {CacheStorageCostPerTokenPerHour: &price},
	}}
	billing := NewBillingService(&config.Config{}, svc)
	input := GeminiCacheStorageQuoteInput{Model: "custom", TotalTokenCount: 2, PricingAt: at, ExpireTime: at.Add(time.Hour), RateMultiplier: 0}
	_, err := billing.QuoteGeminiCacheStorage(input)
	require.ErrorContains(t, err, "overflow", "zero multiplier cannot conceal an invalid base amount")
	input.TotalTokenCount, input.RateMultiplier = 1, 2
	_, err = billing.QuoteGeminiCacheStorage(input)
	require.ErrorContains(t, err, "overflow")
	input.RateMultiplier = 1
	quote, err := billing.QuoteGeminiCacheStorage(input)
	require.NoError(t, err)
	require.Equal(t, math.MaxFloat64, quote.ActualCost)
}

func TestQuoteGeminiCacheStorage_ExplicitFreeStorage(t *testing.T) {
	svc := &PricingService{}
	data, err := svc.parsePricingData([]byte(`{"gemini-2.5-pro":{"cache_storage_cost_per_token_per_hour":0}}`))
	require.NoError(t, err)
	svc.pricingData = data
	at := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	quote, err := NewBillingService(&config.Config{}, svc).QuoteGeminiCacheStorage(GeminiCacheStorageQuoteInput{
		Model: "gemini-2.5-pro", TotalTokenCount: 1_000_000, PricingAt: at, ExpireTime: at.Add(time.Hour), RateMultiplier: 2,
	})
	require.NoError(t, err)
	require.Equal(t, float64(1_000_000), quote.TokenHours)
	require.Zero(t, quote.PricePerTokenHour)
	require.Zero(t, quote.TotalCost)
	require.Zero(t, quote.ActualCost)
}
