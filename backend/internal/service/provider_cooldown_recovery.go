package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"
)

// Optional extension: probes may only remove the exact limit they observed.
// Repositories without compare-and-clear support leave the limit to expire.
type ProviderCooldownRecoveryRepository interface {
	ClearObservedModelRateLimit(ctx context.Context, id int64, scope string, observedJSON []byte) (bool, error)
	ClearObservedTempUnschedulable(ctx context.Context, id int64, observedUntil time.Time, observedReason string) (bool, error)
}

// Probe writes must also compare the observed state: a low balance result may
// not replace a newer request-side spend/inference limit with a weaker reason.
type ProviderCooldownProbeRepository interface {
	SetObservedModelRateLimit(ctx context.Context, id int64, scope string, observedJSON, replacementJSON []byte) (bool, error)
}

func clineProbeMayRecover(scope, reason string) bool {
	switch scope {
	case clinePassRateLimitKey:
		return strings.HasPrefix(reason, clinePassLimitReason) || strings.HasPrefix(reason, clinePassNoSubscriptionReason)
	case clineCreditsRateLimitKey:
		return strings.HasPrefix(reason, clineCreditsReason) || strings.HasPrefix(reason, clineCreditsLowReason)
	default:
		return false
	}
}

func (s *CNProviderQuotaService) clearObservedClineWalletLimit(ctx context.Context, account *Account, scope string) {
	repo, ok := s.accountRepo.(ProviderCooldownRecoveryRepository)
	if !ok || account == nil {
		return
	}
	limits, _ := account.Extra[modelRateLimitsKey].(map[string]any)
	observed, ok := limits[scope].(map[string]any)
	if !ok {
		return
	}
	raw, err := json.Marshal(observed)
	if err != nil {
		return
	}
	cleared, err := repo.ClearObservedModelRateLimit(ctx, account.ID, scope, raw)
	if err != nil {
		slog.Warn("cline_wallet_limit_clear_failed", "account_id", account.ID, "scope", scope, "error", err)
		return
	}
	if cleared {
		delete(limits, scope)
	}
}
