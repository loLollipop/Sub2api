package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Stable storage keys. These describe the priced model, never the transport or account.
var mixedModelVendors = []string{"deepseek", "kimi", "zhipu", "qwen", "minimax", "mimo", "stepfun", "doubao", "yi", "baidu", "spark", "hunyuan", "openai", "anthropic", "gemini", "grok", "other"}

// Request decoder rejects null entries: encoding/json otherwise turns null into
// float64(0), accidentally making a missing numeric input free. A null map is omission.
type MixedProviderRateMultipliers map[string]float64

func (rates *MixedProviderRateMultipliers) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		*rates = nil
		return nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	values := make(map[string]float64, len(raw))
	for vendor, value := range raw {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("mixed provider rate %q must be a number", vendor)
		}
		var rate float64
		if err := json.Unmarshal(value, &rate); err != nil {
			return err
		}
		values[vendor] = rate
	}
	if err := ValidateMixedProviderRateMultipliers(values); err != nil {
		return err
	}
	*rates = values
	return nil
}

func CloneMixedProviderRateMultipliers(rates map[string]float64) map[string]float64 {
	return maps.Clone(rates)
}

func ValidateMixedProviderRateMultipliers(rates map[string]float64) error {
	for vendor, rate := range rates {
		known := false
		for _, allowed := range mixedModelVendors {
			if vendor == allowed {
				known = true
				break
			}
		}
		if !known || math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 || rate > 999999.9999 {
			return infraerrors.BadRequest("INVALID_MIXED_PROVIDER_RATE", fmt.Sprintf("invalid mixed provider rate for %q (expected a known vendor and finite rate between 0 and 999999.9999)", vendor))
		}
	}
	return nil
}

// ModelVendor deliberately shares namespace validation with the four key scopes.
// Arbitrary namespaces and mismatched vendor/family pairs never borrow a vendor rate.
func ModelVendor(model string) string {
	scope := ClassifyModelScope(model)
	if scope == ModelScopeOpenAI || scope == ModelScopeAnthropic {
		return scope
	}
	model = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(model)), "models/")
	if scope == ModelScopeCN {
		if _, family, ok := strings.Cut(model, "/"); ok {
			model = family
		}
		if vendor := bareCNModelAliasVendor(model); vendor != "" {
			return vendor
		}
		for _, entry := range []struct {
			vendor   string
			families []string
		}{
			{"deepseek", []string{"deepseek"}}, {"kimi", []string{"kimi", "moonshot"}},
			{"zhipu", []string{"glm", "chatglm", "cogview", "cogvideo"}}, {"qwen", []string{"qwen", "qwq"}},
			{"minimax", []string{"minimax", "abab"}}, {"mimo", []string{"mimo"}}, {"stepfun", []string{"step"}},
			{"doubao", []string{"doubao"}}, {"yi", []string{"yi"}}, {"baidu", []string{"ernie"}}, {"spark", []string{"spark"}}, {"hunyuan", []string{"hunyuan"}},
		} {
			for _, family := range entry.families {
				if modelFamily(model, family) {
					return entry.vendor
				}
			}
		}
	}
	for _, entry := range []struct{ vendor, namespace, family string }{
		{"gemini", "google", "gemini"}, {"gemini", "gemini", "gemini"},
		{"grok", "x-ai", "grok"}, {"grok", "xai", "grok"}, {"grok", "grok", "grok"},
	} {
		if !strings.Contains(model, "/") && modelFamily(model, entry.family) {
			return entry.vendor
		}
		if namespace, family, ok := strings.Cut(model, "/"); ok && namespace == entry.namespace && !strings.Contains(family, "/") && modelFamily(family, entry.family) {
			return entry.vendor
		}
	}
	return "other"
}

// Mixed rates are absolute; an unconfigured vendor uses the standard 1x rate.
// An explicitly configured zero is meaningful. Ordinary groups ignore the map.
func (g *Group) RateMultiplierForModel(model string) float64 {
	if g == nil {
		return 1
	}
	if g.Platform == PlatformMixed {
		if rate, exists := g.MixedProviderRateMultipliers[ModelVendor(model)]; exists {
			return rate
		}
		return 1
	}
	return g.RateMultiplier
}

// Projection/estimation must use the same concrete pricing normalization as
// Mixed settlement. Explicit group/channel cards always retain their identity.
func mixedBillingPricingIdentity(ctx context.Context, billing *BillingService, resolver *ModelPricingResolver, groupID *int64, group *Group, model string, upstream bool) string {
	model = strings.TrimSpace(model)
	if billing == nil || !upstream {
		return model
	}
	if resolver != nil {
		p := resolver.Resolve(ctx, PricingInput{Model: model, GroupID: groupID, Group: group})
		if p != nil && (p.Source == PricingSourceGroup || p.Source == PricingSourceChannel) {
			return model
		}
	}
	normalized := normalizeModelNameForPricing(strings.ToLower(lastSegment(model)))
	if normalized != model && (isDeepSeekModel(normalized) || !billing.HasIdentifiedTokenPricing(model)) && billing.HasIdentifiedTokenPricing(normalized) {
		return normalized
	}
	return model
}

func mixedBillingModelPriced(ctx context.Context, billing *BillingService, resolver *ModelPricingResolver, groupID *int64, group *Group, model string) bool {
	if resolver != nil {
		p := resolver.Resolve(ctx, PricingInput{Model: model, GroupID: groupID, Group: group})
		if p != nil && (p.BasePricing != nil || p.Mode == BillingModeImage || p.Mode == BillingModePerRequest || p.Mode == BillingModeVideo) {
			return true
		}
	}
	if billing != nil {
		p, err := billing.GetModelPricing(model)
		return err == nil && p != nil
	}
	return false
}
