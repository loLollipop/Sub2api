//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type antigravityForceProxyRepo struct{ ProxyRepository }

func (antigravityForceProxyRepo) GetByID(context.Context, int64) (*Proxy, error) {
	return &Proxy{Protocol: "unsupported-test-proxy", Host: "127.0.0.1", Port: 1}, nil
}

func TestAntigravityForcedUsageBypassesBothCacheReads(t *testing.T) {
	proxyID := int64(1)
	account := &Account{ID: 6, Platform: PlatformAntigravity, Type: AccountTypeOAuth, ProxyID: &proxyID,
		Credentials: map[string]any{"access_token": "synthetic-local-token"}}
	s := &AccountUsageService{cache: NewUsageCache(),
		antigravityQuotaFetcher: NewAntigravityQuotaFetcher(antigravityForceProxyRepo{}, nil)}
	reset := time.Now().Add(time.Hour)
	stored := &UsageInfo{SubscriptionTier: "old-cached-plan", FiveHour: &UsageProgress{ResetsAt: &reset}}
	s.cache.antigravityCache.Store(account.ID, &antigravityUsageCache{usageInfo: stored, timestamp: time.Now()})

	normal, err := s.getUsageForAccount(context.Background(), account, false)
	require.NoError(t, err)
	require.Same(t, stored, normal)
	require.Positive(t, normal.FiveHour.RemainingSeconds)

	// A forced request must reach the real fetcher despite the fresh cache.
	// Its invalid proxy fails locally, so this test never contacts Google.
	forced, err := s.getUsageForAccount(context.Background(), account, true)
	require.NoError(t, err)
	require.NotSame(t, stored, forced)
	require.Empty(t, forced.SubscriptionTier)
	require.Contains(t, forced.Error, "create antigravity client failed")
	require.Equal(t, errorCodeNetworkError, forced.ErrorCode)
	current, ok := s.cachedAntigravityUsage(account.ID, false)
	require.True(t, ok)
	require.Same(t, forced, current, "future passive queries should use the new result")
}

func TestAntigravityUsageCacheMissAndExpiry(t *testing.T) {
	s := &AccountUsageService{cache: NewUsageCache()}
	_, ok := s.cachedAntigravityUsage(7, false)
	require.False(t, ok)
	s.cache.antigravityCache.Store(int64(7), &antigravityUsageCache{usageInfo: &UsageInfo{}, timestamp: time.Now().Add(-time.Hour)})
	_, ok = s.cachedAntigravityUsage(7, false)
	require.False(t, ok)
}
