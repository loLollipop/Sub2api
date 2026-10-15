//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestProviderPartialRelayNeverFallsBackToOfficial(t *testing.T) {
	for _, protocol := range []string{APIProtocolAnthropic, APIProtocolResponses, APIProtocolChatCompletions} {
		a := commandCodeTestAccount(9921)
		a.Credentials["api_protocol"] = APIProtocolAdaptive
		a.Credentials["api_base_urls"] = map[string]any{protocol: "https://relay.example/v1"}
		for _, other := range []string{APIProtocolAnthropic, APIProtocolResponses, APIProtocolChatCompletions} {
			base := a.GetCNProtocolBaseURL(other)
			require.NotContains(t, base, "commandcode.ai")
			if other == protocol {
				require.Equal(t, "https://relay.example/v1", base)
			} else if base != "" {
				require.Equal(t, "https://relay.example/v1", base)
			}
		}
		for _, base := range []string{a.GetOpenAIBaseURL(), a.GetOpenAIFormatBaseURL()} {
			require.NotContains(t, base, "commandcode.ai")
		}
		svc := &OpenAIGatewayService{}
		target, targetErr := svc.openAIChatCompletionsTargetURL(a)
		if targetErr == nil {
			require.Equal(t, "https://relay.example/v1/chat/completions", target)
		}
		req, syncErr := buildOpenAIAPIKeyModelsRequest(context.Background(), a, func(base string) (string, error) { return base, nil })
		if syncErr == nil {
			require.Equal(t, "relay.example", req.URL.Host)
		}
		upstream := &modelCatalogAccountUpstream{}
		svc = &OpenAIGatewayService{httpUpstream: upstream}
		_ = svc.modelCatalogProtocols(context.Background(), a, "claude-sonnet-4")
		upstream.mu.Lock()
		for _, request := range upstream.requests {
			require.NotEqual(t, "api.commandcode.ai", request.URL.Host)
		}
		upstream.mu.Unlock()
	}
}

func TestProviderBalanceProbeDoesNotClearNewerCooldown(t *testing.T) {
	a := commandCodeUsageAccount()
	until := time.Now().Add(time.Hour)
	a.TempUnschedulableUntil = &until
	a.TempUnschedulableReason = cnBalanceLowReason("low balance")
	observed := *a
	repo := &commandCodeStatefulRepo{account: a}
	a.TempUnschedulableReason = commandCodeSpendLimitReason
	svc := &CNProviderBalanceCheckService{accountRepo: repo}
	outcome := svc.applyBalanceResult(context.Background(), &observed, &CNProviderBalanceResult{Available: true, Balance: 20}, 1)
	require.Equal(t, cnBalanceNoChange, outcome)
	require.Zero(t, repo.clearCalls)
	require.Equal(t, commandCodeSpendLimitReason, a.TempUnschedulableReason)
}

func TestClineProbeCannotReplaceUnprovableWalletLimit(t *testing.T) {
	now := time.Now()
	for _, reason := range []string{clineSpendLimitReason, clineInferenceCapReason} {
		for _, scope := range []string{clinePassRateLimitKey, clineCreditsRateLimitKey} {
			a := clineTestAccount(9984)
			setAccountModelRateLimitSnapshot(a, scope, now.Add(time.Hour), reason, now)
			repo := &clineAccountRepo{accounts: []Account{*a}}
			svc := &CNProviderQuotaService{accountRepo: repo}
			svc.applyClineWallets(context.Background(), a, false, nil, &CNProviderBalanceResult{Balance: 0}, now)
			require.Equal(t, reason, a.modelRateLimitReason(scope))
			svc.applyClineWallets(context.Background(), a, true, nil, &CNProviderBalanceResult{Balance: 20}, now)
			require.Equal(t, reason, a.modelRateLimitReason(scope))
		}
	}
}

func TestProviderCanonicalModelIsNotMappedTwice(t *testing.T) {
	a := clineTestAccount(9922)
	a.Credentials["model_mapping"] = map[string]any{"alias": "cline-pass/glm-5.3", "cline-pass/glm-5.3": "deepseek/deepseek-v4-flash"}
	repo := &commandCodeRateLimitRepo{}
	limits := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc := &OpenAIGatewayService{rateLimitService: limits}
	ctx := context.Background()
	_ = svc.handleOpenAIAccountUpstreamError(ctx, a, 429, http.Header{}, []byte(clinePassWeeklyBody), "cline-pass/glm-5.3")
	require.Contains(t, repo.modelLimits, clinePassRateLimitKey)
	require.NotContains(t, repo.modelLimits, clineCreditsRateLimitKey)
}

func TestClineRecognizedWalletFailureDoesNotWidenOnWriteError(t *testing.T) {
	for _, status := range []int{403, 429} {
		repo := &clineModelLimitFailRepo{}
		limits := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		a := clineTestAccount(9923)
		ctx := withCanonicalUpstreamLimitModel(context.Background(), "cline-pass/glm-5.3")
		require.False(t, limits.HandleUpstreamError(ctx, a, status, http.Header{}, []byte(clinePassWeeklyBody)))
		require.Zero(t, repo.rateLimitedCalls)
		require.Zero(t, repo.tempCalls)
		require.Zero(t, repo.setErrorCalls)
		require.True(t, a.isRateLimitActiveForKey(clinePassRateLimitKey))
	}
}

func TestProviderProbeDoesNotClearNewerCooldown(t *testing.T) {
	now := time.Now()
	t.Run("CommandCode", func(t *testing.T) {
		a := commandCodeUsageAccount()
		until := now.Add(time.Hour)
		a.TempUnschedulableUntil = &until
		a.TempUnschedulableReason = commandCodeUsageLimitReason
		observed := *a
		repo := &commandCodeStatefulRepo{account: a}
		a.TempUnschedulableReason = commandCodeSpendLimitReason
		usage := &commandCodeUsage{}
		require.NoError(t, json.Unmarshal([]byte(commandCodeCreditsBody(20)), &usage.credits))
		require.False(t, clearCommandCodeWindowCooldown(context.Background(), repo, &observed, usage, now))
		require.Equal(t, commandCodeSpendLimitReason, a.TempUnschedulableReason)
		require.Zero(t, repo.clearCalls)
	})
	t.Run("Cline", func(t *testing.T) {
		a := clineTestAccount(9924)
		setAccountModelRateLimitSnapshot(a, clineCreditsRateLimitKey, now.Add(time.Hour), clineCreditsReason, now)
		observed := *a
		observed.Extra = map[string]any{modelRateLimitsKey: map[string]any{clineCreditsRateLimitKey: map[string]any{
			"rate_limited_at": now.UTC().Format(time.RFC3339), "rate_limit_reset_at": now.Add(time.Hour).UTC().Format(time.RFC3339), "reason": clineCreditsReason,
		}}}
		repo := &clineAccountRepo{accounts: []Account{*a}}
		setAccountModelRateLimitSnapshot(&repo.accounts[0], clineCreditsRateLimitKey, now.Add(time.Hour), clineSpendLimitReason, now)
		svc := &CNProviderQuotaService{accountRepo: repo}
		svc.clearObservedClineWalletLimit(context.Background(), &observed, clineCreditsRateLimitKey)
		require.True(t, strings.HasPrefix(repo.accounts[0].modelRateLimitReason(clineCreditsRateLimitKey), clineSpendLimitReason))
		require.Empty(t, repo.modelLimits)
	})
}
