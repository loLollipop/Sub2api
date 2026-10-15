//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSystemOnePricingExactFallbackAndOverrides(t *testing.T) {
	s := NewBillingService(&config.Config{}, nil)
	p, err := s.GetModelPricing("jev-latest")
	require.NoError(t, err)
	require.InDelta(t, .042/1e6, p.InputPricePerToken, 1e-16)
	require.Zero(t, p.OutputPricePerToken)
	for _, model := range []string{"jev-unknown", "jev-1.13.0", "JEV-LATEST", "jev-gpt-6"} {
		require.Nil(t, s.getFallbackPricing(model), model)
	}
	catalog := &PricingService{pricingData: map[string]*LiteLLMModelPricing{"jev-latest": {InputCostPerToken: 2e-6, OutputCostPerToken: 3e-6}}}
	s = NewBillingService(&config.Config{}, catalog)
	p, err = s.GetModelPricing("jev-latest")
	require.NoError(t, err)
	require.Equal(t, 2e-6, p.InputPricePerToken)
	zero := 0.0
	p, err = s.GetModelPricingWithChannel("jev-latest", &ChannelModelPricing{InputPrice: &zero, OutputPrice: &zero})
	require.NoError(t, err)
	require.Zero(t, p.InputPricePerToken)
	require.Zero(t, p.OutputPricePerToken)
}

func TestSystemOnePreauthorizationAndUsageActualInputOnly(t *testing.T) {
	f := newPreauthorizationFixture()
	logs := &openAIRecordUsageBestEffortLogRepoStub{}
	billingRepo := &guardedUsageBillingApplyRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	s := newGatewayRecordUsageServiceWithBillingRepoForTest(logs, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})
	f.service.costCalculator = s.billingService
	req := balancePreauthorizationTestRequest()
	req.EstimatedInputTokens = 1_000_000
	req.CostInput = CostInput{Model: "jev-latest", RateMultiplier: 2.5, PricingAt: time.Now()}
	g, err := f.service.Preauthorize(context.Background(), req)
	require.NoError(t, err)
	require.InDelta(t, .105, g.HoldAmount(), 1e-8)
	w, ok := g.TransferToWorker()
	require.True(t, ok)
	groupID := int64(9)
	err = s.RecordUsage(ContextWithBalancePreauthorizationGuard(context.Background(), w), &RecordUsageInput{
		Result: &ForwardResult{RequestID: "request-1", Model: "jev-latest", Usage: ClaudeUsage{InputTokens: 1_000_000, OutputTokens: 99}},
		APIKey: &APIKey{ID: 7, UserID: 42, GroupID: &groupID, Group: &Group{ID: 9, Platform: PlatformTypeSafe, RateMultiplier: 2.5}},
		User:   &User{ID: 42}, Account: &Account{ID: 11, Platform: PlatformTypeSafe, Type: AccountTypeAPIKey},
	})
	require.NoError(t, err)
	require.NotNil(t, billingRepo.lastCmd)
	require.Equal(t, "jev-latest", billingRepo.lastCmd.Model)
	require.InDelta(t, .105, billingRepo.lastCmd.BalanceCost, 1e-8)
	require.InDelta(t, .105, f.wallet.lastActual, 1e-8)
	require.NoError(t, g.Refund(context.Background()))
	require.NotContains(t, f.recorder.snapshot(), "repo_begin_refund")
}

func TestSystemOneSchedulingAndScopes(t *testing.T) {
	for _, platform := range []string{PlatformOpenAI, PlatformMixed, PlatformAnthropic, PlatformGemini, PlatformCommandCode, PlatformCline} {
		require.False(t, groupUsableForRequest(&Group{Platform: platform}, PlatformTypeSafe, "jev-latest", map[string]struct{}{PlatformOpenAI: {}}))
	}
	require.True(t, groupUsableForRequest(&Group{Platform: PlatformTypeSafe}, PlatformTypeSafe, "jev-latest"))
	require.False(t, groupUsableForRequest(&Group{Platform: PlatformTypeSafe}, PlatformOpenAI, "gpt-6"))
	for _, scope := range []string{ModelScopeOpenAI, ModelScopeAnthropic, ModelScopeCN} {
		require.Error(t, CheckModelScope(ContextWithModelScope(context.Background(), scope), "jev-latest", "jev-latest"))
	}
	require.NoError(t, CheckModelScope(ContextWithModelScope(context.Background(), ModelScopeOther), "jev-latest", "jev-latest"))
}
