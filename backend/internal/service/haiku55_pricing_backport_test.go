package service

import (
	"fmt"
	"os"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestHaiku55PricingSourcesAndPromptBoundaries(t *testing.T) {
	catalog, err := os.ReadFile("../../resources/model-pricing/model_prices_and_context_window.json")
	require.NoError(t, err)
	services := map[string]*BillingService{
		"billing fallback": NewBillingService(&config.Config{}, nil),
		"pricing fallback": NewBillingService(&config.Config{}, &PricingService{}),
		"bundled catalog":  NewBillingService(&config.Config{}, newStubPricingServiceFromJSON(t, string(catalog))),
	}
	for name, svc := range services {
		t.Run(name, func(t *testing.T) {
			for _, prompt := range []int{99999, 100000, 100001} {
				t.Run(fmt.Sprint(prompt), func(t *testing.T) {
					tokens := UsageTokens{
						InputTokens: prompt - 300, OutputTokens: 200,
						CacheCreationTokens: 200, CacheReadTokens: 100,
						CacheCreation5mTokens: 120, CacheCreation1hTokens: 80,
					}
					cost, err := svc.CalculateCost("claude-haiku-5-5", tokens, 0.15)
					require.NoError(t, err)
					multiplier := 1.0
					if prompt > 100000 {
						multiplier = 5
					}
					require.InDelta(t, float64(tokens.InputTokens)*0.1e-6*multiplier, cost.InputCost, 1e-12)
					require.InDelta(t, 200*0.5e-6*multiplier, cost.OutputCost, 1e-12)
					require.InDelta(t, (120*0.125e-6+80*0.2e-6)*multiplier, cost.CacheCreationCost, 1e-12)
					require.InDelta(t, 100*0.01e-6*multiplier, cost.CacheReadCost, 1e-12)
					total := cost.InputCost + cost.OutputCost + cost.CacheCreationCost + cost.CacheReadCost
					require.InDelta(t, total, cost.TotalCost, 1e-12)
					require.InDelta(t, total*0.15, cost.ActualCost, 1e-12)
					require.Equal(t, prompt > 100000, cost.LongContextBillingApplied)
				})
			}
		})
	}
}

func TestHaiku55CodexDescriptor(t *testing.T) {
	d := newConfiguredCodexModelDescriptor("claude-haiku-5-5")
	require.Equal(t, int64(1_000_000), d.ContextWindow)
	require.Equal(t, int64(1_000_000), d.MaxContextWindow)
	require.NotNil(t, d.DefaultReasoningLevel)
	require.Equal(t, "medium", *d.DefaultReasoningLevel)
	require.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, effortsFromConfiguredCodexLevels(d.SupportedReasoningLevels))
}
