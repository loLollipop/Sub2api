package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"
)

// Standard storage rates from the frozen upstream cache-storage helper:
// https://ai.google.dev/gemini-api/docs/pricing
// USD / token / hour; no context-length or service-tier multiplier is implied.
const (
	geminiProCacheStoragePrice   = 4.5e-6
	geminiFlashCacheStoragePrice = 1e-6
)

// Only identified published models receive a builtin rate. Unknown future
// versions and moving "latest" aliases require an explicit catalog rate.
// The 3.6/3.7/3.8 Flash promotional window is intentionally NOT implemented:
// the cutoff timezone and provider-specific storage eligibility remain unverified.
// These models use the standard rate unless an operator explicitly overrides it.
var geminiCacheStorageModelPrices = map[string]float64{
	"gemini-3.8-flash-cyber": geminiFlashCacheStoragePrice,
	"gemini-3.8-flash":       geminiFlashCacheStoragePrice,
	"gemini-3.7-flash":       geminiFlashCacheStoragePrice,
	"gemini-3.6-flash":       geminiFlashCacheStoragePrice,
	"gemini-3.5-flash-lite":  geminiFlashCacheStoragePrice,
	"gemini-3.5-flash":       geminiFlashCacheStoragePrice,
	"gemini-3.1-flash-lite":  geminiFlashCacheStoragePrice,
	"gemini-3.1-pro":         geminiProCacheStoragePrice,
	"gemini-3-flash":         geminiFlashCacheStoragePrice,
	"gemini-3-pro":           geminiProCacheStoragePrice,
	"gemini-2.5-flash-lite":  geminiFlashCacheStoragePrice,
	"gemini-2.5-flash":       geminiFlashCacheStoragePrice,
	"gemini-2.5-pro":         geminiProCacheStoragePrice,
	"gemini-2.0-flash-lite":  geminiFlashCacheStoragePrice,
	"gemini-2.0-flash":       geminiFlashCacheStoragePrice,
}

func validCacheStorageAmount(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func parseCacheStoragePricingField(rawEntry json.RawMessage) (*float64, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rawEntry, &fields); err != nil {
		return nil, nil // Preserve the parser's ordinary invalid-entry handling.
	}
	raw, present := fields["cache_storage_cost_per_token_per_hour"]
	if !present {
		return nil, nil
	}
	var price *float64
	if err := json.Unmarshal(raw, &price); err != nil {
		return nil, fmt.Errorf("invalid cache_storage_cost_per_token_per_hour: %w", err)
	}
	// JSON null follows the existing pricing override field-removal convention.
	if price != nil && !validCacheStorageAmount(*price) {
		return nil, fmt.Errorf("cache_storage_cost_per_token_per_hour must be finite and nonnegative")
	}
	return price, nil
}

// Existing fallback/override-only merging skips parser errors. Validate just
// storage-bearing entries before those merges, so even an override-only rate
// cannot disappear and accidentally expose a builtin. Other pricing fields
// keep their existing missing/invalid source-file behavior.
func (s *PricingService) validateCacheStoragePricingSources() error {
	if s == nil || s.cfg == nil {
		return nil
	}
	for _, source := range []struct {
		path     string
		fallback bool
	}{{s.cfg.Pricing.OverrideFile, false}, {s.cfg.Pricing.FallbackFile, true}} {
		path := strings.TrimSpace(source.path)
		if path == "" {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var entries map[string]json.RawMessage
		if err := json.Unmarshal(body, &entries); err != nil {
			if bytes.Contains(body, []byte(`"cache_storage_cost_per_token_per_hour"`)) {
				return fmt.Errorf("invalid cache storage pricing source: %w", err)
			}
			continue
		}
		if source.fallback {
			entries = s.applyPricingOverrides(entries)
		}
		for name, raw := range entries {
			if name == "sample_spec" {
				continue
			}
			price, err := parseCacheStoragePricingField(raw)
			if err != nil {
				return fmt.Errorf("cache storage pricing source for %s: %w", name, err)
			}
			if price != nil {
				var entry LiteLLMRawEntry
				if err := json.Unmarshal(raw, &entry); err != nil {
					return fmt.Errorf("cache storage pricing source entry for %s: %w", name, err)
				}
			}
		}
	}
	return nil
}

func cacheStorageModelIdentity(model string) string {
	return strings.ToLower(lastSegment(strings.TrimSpace(model)))
}

// These aliases identify the same published model. Arbitrary suffixes are not
// stripped, and there is no substring/family or cross-version catalog search.
func cacheStorageModelAlias(model string) string {
	model = normalizeGeminiThinkingTierAlias(model)
	switch model {
	case "gemini-3.1-pro-preview", "gemini-3.1-pro-preview-customtools":
		return "gemini-3.1-pro"
	case "gemini-3-pro-preview":
		return "gemini-3-pro"
	case "gemini-3-flash-preview":
		return "gemini-3-flash"
	case "gemini-2.0-flash-001":
		return "gemini-2.0-flash"
	case "gemini-2.0-flash-lite-001":
		return "gemini-2.0-flash-lite"
	}
	return model
}

func builtinGeminiCacheStoragePrice(model string) (float64, bool) {
	price, found := geminiCacheStorageModelPrices[cacheStorageModelAlias(cacheStorageModelIdentity(model))]
	return price, found
}

// identifiedCacheStoragePrice keeps the exact resource/provider/model entry
// ahead of known aliases. Unlike GetIdentifiedModelPricing's version scan,
// this storage-only lookup never picks an arbitrary sibling from a Go map.
func (s *PricingService) identifiedCacheStoragePrice(model string) (*float64, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw := strings.ToLower(strings.TrimSpace(model))
	native := cacheStorageModelIdentity(raw)
	keys := make([]string, 0, len(s.pricingData))
	for key := range s.pricingData {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, identity := range []string{raw, native, cacheStorageModelAlias(native)} {
		if identity == "" {
			continue
		}
		if pricing, found := s.pricingData[identity]; found && pricing != nil && pricing.CacheStorageCostPerTokenPerHour != nil {
			return pricing.CacheStorageCostPerTokenPerHour, nil
		}
		// A catalog may itself use provider-prefixed/resource or mixed-case keys.
		// Reject collisions rather than silently select one of two price cards.
		var matched *LiteLLMModelPricing
		for _, key := range keys {
			pricing := s.pricingData[key]
			if (strings.ToLower(strings.TrimSpace(key)) != identity && cacheStorageModelIdentity(key) != identity) || pricing == nil || pricing.CacheStorageCostPerTokenPerHour == nil {
				continue
			}
			if matched != nil {
				return nil, fmt.Errorf("ambiguous cache storage pricing for model: %s", model)
			}
			matched = pricing
		}
		if matched != nil {
			return matched.CacheStorageCostPerTokenPerHour, nil
		}
	}
	return nil, nil
}

// CacheStoragePricePerTokenHour returns a present catalog rate (including zero)
// before the standard builtin. Invalid explicit rates and unknown models fail
// closed. This does not resolve text token prices or change billing state.
func (s *BillingService) CacheStoragePricePerTokenHour(model string) (float64, error) {
	if strings.TrimSpace(model) == "" {
		return 0, fmt.Errorf("%w for cache storage model: %s", ErrModelPricingUnavailable, model)
	}
	if s != nil && s.pricingService != nil {
		price, err := s.pricingService.identifiedCacheStoragePrice(model)
		if err != nil {
			return 0, err
		}
		if price != nil {
			if !validCacheStorageAmount(*price) {
				return 0, fmt.Errorf("invalid cache storage price for model: %s", model)
			}
			return *price, nil
		}
	}
	if price, found := builtinGeminiCacheStoragePrice(model); found {
		return price, nil
	}
	return 0, fmt.Errorf("%w for cache storage model: %s", ErrModelPricingUnavailable, model)
}

// GeminiCacheStorageQuoteInput must contain the actual resolved upstream target,
// not a public alias before channel/account mapping. PricingAt is frozen by the
// caller; this helper never substitutes the current clock or a group default.
type GeminiCacheStorageQuoteInput struct {
	Model           string
	TotalTokenCount int64
	PricingAt       time.Time
	ExpireTime      time.Time
	RateMultiplier  float64 // Existing absolute billing multiplier; zero is free.
}

type GeminiCacheStorageQuote struct {
	GeminiCacheStorageQuoteInput
	TokenHours        float64
	PricePerTokenHour float64
	TotalCost         float64
	ActualCost        float64
}

// QuoteGeminiCacheStorage is a pure quote: fractional hours are not rounded up,
// the supplied interval and absolute multiplier are used exactly once, and no
// wallet, quota, subscription, usage, or upstream mutation is performed. It is
// deliberately not connected to routes, dependency injection, or workers yet.
func (s *BillingService) QuoteGeminiCacheStorage(input GeminiCacheStorageQuoteInput) (*GeminiCacheStorageQuote, error) {
	if input.TotalTokenCount < 0 || !validCacheStorageAmount(input.RateMultiplier) {
		return nil, fmt.Errorf("cache storage tokens and multiplier must be finite and nonnegative")
	}
	if input.PricingAt.IsZero() || input.ExpireTime.IsZero() || input.ExpireTime.Before(input.PricingAt) {
		return nil, fmt.Errorf("invalid cache storage quote interval")
	}
	duration := input.ExpireTime.Sub(input.PricingAt)
	// time.Time.Sub saturates on overflow. Do not silently underquote long TTLs.
	if !input.PricingAt.Add(duration).Equal(input.ExpireTime) {
		return nil, fmt.Errorf("cache storage quote duration overflow")
	}
	price, err := s.CacheStoragePricePerTokenHour(input.Model)
	if err != nil {
		return nil, err
	}
	tokenHours := float64(input.TotalTokenCount) * duration.Hours()
	total := tokenHours * price
	actual := total * input.RateMultiplier
	if !validCacheStorageAmount(tokenHours) || !validCacheStorageAmount(total) || !validCacheStorageAmount(actual) {
		return nil, fmt.Errorf("cache storage quote amount overflow")
	}
	return &GeminiCacheStorageQuote{
		GeminiCacheStorageQuoteInput: input,
		TokenHours:                   tokenHours,
		PricePerTokenHour:            price,
		TotalCost:                    total,
		ActualCost:                   actual,
	}, nil
}
