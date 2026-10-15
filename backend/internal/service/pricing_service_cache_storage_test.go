//go:build unit

package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestParsePricingData_CacheStoragePresence(t *testing.T) {
	svc := &PricingService{}
	data, err := svc.parsePricingData([]byte(`{
		"missing":{"input_cost_per_token":0.000001},
		"null":{"input_cost_per_token":0.000001,"cache_storage_cost_per_token_per_hour":null},
		"free":{"cache_storage_cost_per_token_per_hour":0},
		"storage-only":{"cache_storage_cost_per_token_per_hour":0.000002},
		"explicit-token-zero":{"input_cost_per_token":0,"output_cost_per_token":0,"cache_storage_cost_per_token_per_hour":0}
	}`))
	require.NoError(t, err)
	require.Nil(t, data["missing"].CacheStorageCostPerTokenPerHour)
	require.Nil(t, data["null"].CacheStorageCostPerTokenPerHour)
	require.NotNil(t, data["free"].CacheStorageCostPerTokenPerHour)
	require.Zero(t, *data["free"].CacheStorageCostPerTokenPerHour)
	require.Equal(t, 2e-6, *data["storage-only"].CacheStorageCostPerTokenPerHour)
	require.True(t, data["free"].TokenPricingAbsent)
	require.True(t, data["storage-only"].TokenPricingAbsent)
	require.False(t, data["explicit-token-zero"].TokenPricingAbsent)
	require.True(t, data["storage-only"].InputPriceAbsent)
	require.True(t, data["free"].InputPriceAbsent)
	require.False(t, data["missing"].InputPriceAbsent)
	require.False(t, data["explicit-token-zero"].InputPriceAbsent)
	missingJSON, err := json.Marshal(data["missing"])
	require.NoError(t, err)
	require.NotContains(t, string(missingJSON), "cache_storage_cost_per_token_per_hour")
	freeJSON, err := json.Marshal(data["free"])
	require.NoError(t, err)
	require.Contains(t, string(freeJSON), `"cache_storage_cost_per_token_per_hour":0`)

	svc.pricingData = data
	billing := NewBillingService(&config.Config{}, svc)
	for _, model := range []string{"free", "storage-only"} {
		require.False(t, billing.HasIdentifiedTokenPricing(model))
		_, err = billing.GetModelPricing(model)
		require.ErrorIs(t, err, ErrModelPricingUnavailable)
		_, err = billing.CalculateCost(model, UsageTokens{InputTokens: 100}, 1)
		require.ErrorIs(t, err, ErrModelPricingUnavailable)
	}
}

func TestParsePricingData_CacheStorageInvalidFailsClosed(t *testing.T) {
	for _, value := range []string{"-0.000001", "1e1000", `"NaN"`, `"Infinity"`, "true", "{}", "NaN"} {
		t.Run(value, func(t *testing.T) {
			data, err := (&PricingService{}).parsePricingData([]byte(`{
				"valid":{"input_cost_per_token":0.000001},
				"gemini-3.8-flash":{"cache_storage_cost_per_token_per_hour":` + value + `}
			}`))
			require.Error(t, err, "must reject the catalog, not silently drop the explicit rate")
			require.Nil(t, data)
		})
	}
	_, err := (&PricingService{}).parsePricingData([]byte(`{
		"valid":{"input_cost_per_token":0.000001},
		"gemini-3.8-flash":{"input_cost_per_token":"invalid","cache_storage_cost_per_token_per_hour":0}
	}`))
	require.Error(t, err, "a valid explicit storage rate must not be discarded with another invalid field")
}

func TestPricingOverride_CacheStorageZeroAndMissing(t *testing.T) {
	svc := newPricingServiceWithOverride(t, `{
		"gemini-3.8-flash":{"cache_storage_cost_per_token_per_hour":0},
		"gemini-2.5-pro":{"cache_storage_cost_per_token_per_hour":null},
		"customer-alias":{"cache_storage_cost_per_token_per_hour":0.000007}
	}`)
	data, err := svc.parsePricingData([]byte(`{
		"gemini-3.8-flash":{"input_cost_per_token":0.000001,"cache_storage_cost_per_token_per_hour":0.000002},
		"gemini-2.5-pro":{"input_cost_per_token":0.000001,"cache_storage_cost_per_token_per_hour":0.000003}
	}`))
	require.NoError(t, err)
	data = svc.mergeOverrideOnlyModels(data)
	svc.pricingData = data
	billing := NewBillingService(&config.Config{}, svc)
	price, err := billing.CacheStoragePricePerTokenHour("gemini-3.8-flash")
	require.NoError(t, err)
	require.Zero(t, price, "explicit zero override takes priority over catalog and builtin")
	price, err = billing.CacheStoragePricePerTokenHour("gemini-2.5-pro")
	require.NoError(t, err)
	require.Equal(t, geminiProCacheStoragePrice, price, "removed field permits the standard builtin")
	price, err = billing.CacheStoragePricePerTokenHour("customer-alias")
	require.NoError(t, err)
	require.Equal(t, 7e-6, price)
	require.True(t, data["customer-alias"].TokenPricingAbsent)
}

func TestParsePricingData_CacheStorageInvalidSourcesFailClosed(t *testing.T) {
	for _, source := range []string{"override", "fallback"} {
		for _, value := range []string{"-1", "1e1000", `"NaN"`, "NaN"} {
			t.Run(source+value, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "pricing.json")
				require.NoError(t, os.WriteFile(path, []byte(`{"gemini-3.8-flash":{"cache_storage_cost_per_token_per_hour":`+value+`}}`), 0600))
				svc := &PricingService{cfg: &config.Config{}}
				if source == "override" {
					svc.cfg.Pricing.OverrideFile = path
				} else {
					svc.cfg.Pricing.FallbackFile = path
				}
				_, _, err := svc.buildPricingData([]byte(`{"unrelated":{"input_cost_per_token":0.000001}}`))
				require.Error(t, err, "invalid storage-only sources must not disappear in a swallowed merge error")
			})
		}
	}
}

func TestPricingOverride_CacheStorageCorrectsFallback(t *testing.T) {
	svc := newPricingServiceWithOverride(t, `{"gemini-3.8-flash":{"cache_storage_cost_per_token_per_hour":0}}`)
	path := filepath.Join(t.TempDir(), "fallback.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"gemini-3.8-flash":{"cache_storage_cost_per_token_per_hour":-1}}`), 0600))
	svc.cfg.Pricing.FallbackFile = path
	data, _, err := svc.buildPricingData([]byte(`{"unrelated":{"input_cost_per_token":0.000001}}`))
	require.NoError(t, err, "a valid explicit override can correct a bad fallback rate")
	svc.pricingData = data
	price, err := NewBillingService(&config.Config{}, svc).CacheStoragePricePerTokenHour("gemini-3.8-flash")
	require.NoError(t, err)
	require.Zero(t, price)
	require.True(t, data["gemini-3.8-flash"].TokenPricingAbsent)
}

func TestPricingOverride_CacheStorageNullCatalogEntry(t *testing.T) {
	svc := newPricingServiceWithOverride(t, `{"gemini-3.8-flash":{"cache_storage_cost_per_token_per_hour":0}}`)
	var data map[string]*LiteLLMModelPricing
	var err error
	require.NotPanics(t, func() {
		data, _, err = svc.buildPricingData([]byte(`{"gemini-3.8-flash":null}`))
	})
	require.NoError(t, err)
	require.NotNil(t, data["gemini-3.8-flash"])
	require.NotNil(t, data["gemini-3.8-flash"].CacheStorageCostPerTokenPerHour)
	require.Zero(t, *data["gemini-3.8-flash"].CacheStorageCostPerTokenPerHour)
	require.True(t, data["gemini-3.8-flash"].TokenPricingAbsent)
}

func TestParsePricingData_CacheStorageWhitespaceSourcePathFailsClosed(t *testing.T) {
	for _, source := range []string{"override", "fallback"} {
		t.Run(source, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "pricing.json")
			require.NoError(t, os.WriteFile(path, []byte(`{"gemini-3.8-flash":{"cache_storage_cost_per_token_per_hour":-1}}`), 0600))
			svc := &PricingService{cfg: &config.Config{}}
			if source == "override" {
				svc.cfg.Pricing.OverrideFile = " " + path + " "
			} else {
				svc.cfg.Pricing.FallbackFile = " " + path + " "
			}
			_, _, err := svc.buildPricingData([]byte(`{"unrelated":{"input_cost_per_token":0.000001}}`))
			require.Error(t, err, "the validator and loader must normalize the same source path")
		})
	}
}

func TestPricingOverride_CacheStorageWhitespaceSourcePathKeepsExplicitZero(t *testing.T) {
	for _, source := range []string{"override", "fallback"} {
		t.Run(source, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "pricing.json")
			require.NoError(t, os.WriteFile(path, []byte(`{"gemini-3.8-flash":{"cache_storage_cost_per_token_per_hour":0}}`), 0600))
			svc := &PricingService{cfg: &config.Config{}}
			if source == "override" {
				svc.cfg.Pricing.OverrideFile = " " + path + " "
			} else {
				svc.cfg.Pricing.FallbackFile = " " + path + " "
			}
			data, _, err := svc.buildPricingData([]byte(`{"unrelated":{"input_cost_per_token":0.000001}}`))
			require.NoError(t, err)
			svc.pricingData = data
			price, err := NewBillingService(&config.Config{}, svc).CacheStoragePricePerTokenHour("gemini-3.8-flash")
			require.NoError(t, err)
			require.Zero(t, price)
		})
	}
}
