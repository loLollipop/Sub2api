//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestGrokRefusalQuotaExactMoneyBoundaries(t *testing.T) {
	now := time.Now()
	for _, amounts := range []struct {
		name               string
		base, price, limit float64
		count              int
	}{
		{"ten_refusals", 0, .1, 1, 10},
		{"mixed_usage", .7, .2, .9, 1},
	} {
		for _, path := range []string{"hit", "miss", "database_fallback"} {
			t.Run(amounts.name+"/"+path, func(t *testing.T) {
				q := &GrokRefusalQuotaUsage{}
				for i := 0; i < amounts.count; i++ {
					q.Add(amounts.price, now, &now)
				}
				require.Equal(t, QuantizeUsageBillingAmount(amounts.price*float64(amounts.count)), q.Daily)
				require.Equal(t, q.Daily, q.Weekly)
				require.Equal(t, q.Daily, q.Monthly)
				base := &UserPlatformQuotaRecord{DailyLimitUSD: &amounts.limit, DailyUsageUSD: amounts.base, DailyWindowStart: q.DailyStart}
				cache := &fakeFullCache{}
				if path == "hit" {
					cache.entry = &UserPlatformQuotaCacheEntry{SchemaVersion: UserPlatformQuotaCacheSchemaV1, DailyLimitUSD: &amounts.limit, DailyUsageUSD: amounts.base, DailyWindowStart: q.DailyStart}
				}
				if path == "database_fallback" {
					cache.getErr = errors.New("redis unavailable")
				}
				svc := &BillingCacheService{cfg: &config.Config{}, cache: cache, userPlatformQuotaRepo: &refusalQuotaTestRepo{fakeQuotaRepo: &fakeQuotaRepo{rec: base}, usage: q}}
				require.ErrorIs(t, svc.checkUserPlatformQuotaEligibility(context.Background(), 1, PlatformGrok), ErrUserPlatformDailyQuotaExhausted)
			})
		}
	}
}

func TestGrokRefusalMonthlyRetryMetadataAcrossCachePaths(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	start := now.Add(-29 * 24 * time.Hour)
	limit := 1.0
	for _, path := range []string{"hit", "miss", "database_fallback"} {
		t.Run(path, func(t *testing.T) {
			quota := &GrokRefusalQuotaUsage{Monthly: 1, MonthlyStart: &start}
			base := &UserPlatformQuotaRecord{MonthlyLimitUSD: &limit}
			cache := &fakeFullCache{}
			if path == "hit" {
				cache.entry = &UserPlatformQuotaCacheEntry{SchemaVersion: UserPlatformQuotaCacheSchemaV1, MonthlyLimitUSD: &limit}
			}
			if path == "database_fallback" {
				cache.getErr = errors.New("redis unavailable")
			}
			svc := &BillingCacheService{cfg: &config.Config{}, cache: cache, userPlatformQuotaRepo: &refusalQuotaTestRepo{fakeQuotaRepo: &fakeQuotaRepo{rec: base}, usage: quota}}
			err := svc.checkUserPlatformQuotaEligibility(context.Background(), 1, PlatformGrok)
			require.ErrorIs(t, err, ErrUserPlatformMonthlyQuotaExhausted)
			var appErr *infraerrors.ApplicationError
			require.ErrorAs(t, err, &appErr)
			require.Equal(t, start.Add(30*24*time.Hour).Format(time.RFC3339), appErr.Metadata["window_resets_at"])
		})
	}
}

func TestGrokRefusalMonthlyExpiryUsesBothComponents(t *testing.T) {
	now := time.Now()
	early := now.Add(-29 * 24 * time.Hour)
	late := now.Add(-20 * 24 * time.Hour)
	for _, tc := range []struct {
		name                    string
		base, refusal, limit    float64
		baseStart, refusalStart *time.Time
		want                    time.Time
	}{
		{"refusal_only", 0, 1, 1, nil, &early, early.Add(30 * 24 * time.Hour)},
		{"refusal_first", .7, .2, .9, &late, &early, early.Add(30 * 24 * time.Hour)},
		{"base_decimal_precision", .8999999999, .1, .9, &late, &early, early.Add(30 * 24 * time.Hour)},
		{"base_still_exhausted", 1, .2, .9, &late, &early, late.Add(30 * 24 * time.Hour)},
		{"base_first", .7, .2, .9, &early, &late, early.Add(30 * 24 * time.Hour)},
		{"refusal_still_exhausted", .7, 1, .9, &early, &late, late.Add(30 * 24 * time.Hour)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := &GrokRefusalQuotaUsage{Monthly: tc.refusal, MonthlyStart: tc.refusalStart}
			require.Equal(t, tc.want, q.MonthlyResetAt(tc.base, tc.baseStart, tc.limit, now))
		})
	}
}
