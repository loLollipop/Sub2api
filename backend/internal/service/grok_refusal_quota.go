package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// GrokRefusalQuotaUsage is a durable additive component. It is never written to
// the Redis quota counters: the dirty flusher replaces those counters with an
// absolute snapshot and must not overwrite or double count committed refusals.
type GrokRefusalQuotaUsage struct {
	Daily        float64    `json:"daily"`
	Weekly       float64    `json:"weekly"`
	Monthly      float64    `json:"monthly"`
	DailyStart   *time.Time `json:"daily_start,omitempty"`
	WeeklyStart  *time.Time `json:"weekly_start,omitempty"`
	MonthlyStart *time.Time `json:"monthly_start,omitempty"`
}

type GrokRefusalQuotaReader interface {
	GetGrokRefusalQuotaUsage(context.Context, int64, string) (*GrokRefusalQuotaUsage, error)
}

func (q *GrokRefusalQuotaUsage) Current(now time.Time) (daily, weekly, monthly float64) {
	if q == nil {
		return
	}
	if !quotaWindowExpired(q.DailyStart, timezone.StartOfDay(now)) {
		daily = q.Daily
	}
	if !quotaWindowExpired(q.WeeklyStart, timezone.StartOfWeek(now)) {
		weekly = q.Weekly
	}
	if !monthlyQuotaWindowExpired(q.MonthlyStart, now) {
		monthly = q.Monthly
	}
	return
}

// Add resets expired windows and joins the existing monthly window when first
// charged. Independent storage keeps a delayed base-counter snapshot harmless.
func (q *GrokRefusalQuotaUsage) Add(cost float64, now time.Time, baseMonthlyStart *time.Time) {
	daily, weekly, monthly := q.Current(now)
	day, week := timezone.StartOfDay(now), timezone.StartOfWeek(now)
	q.Daily = QuantizeUsageBillingAmount(daily + cost)
	q.Weekly = QuantizeUsageBillingAmount(weekly + cost)
	q.Monthly = QuantizeUsageBillingAmount(monthly + cost)
	q.DailyStart, q.WeeklyStart = &day, &week
	if monthlyQuotaWindowExpired(q.MonthlyStart, now) {
		start := now
		if !monthlyQuotaWindowExpired(baseMonthlyStart, now) {
			start = *baseMonthlyStart
		}
		q.MonthlyStart = &start
	}
}

func combinedRefusalQuotaUsage(base, refusal float64) float64 {
	if refusal == 0 {
		return base
	}
	return QuantizeUsageBillingAmount(base + refusal)
}

// MonthlyResetAt returns the first expiry that brings the combined usage below
// the limit. The base and refusal components may have different rolling starts.
func (q *GrokRefusalQuotaUsage) MonthlyResetAt(base float64, baseStart *time.Time, limit float64, now time.Time) time.Time {
	_, _, refusal := q.Current(now)
	baseExpiry := nextMonthlyResetFrom(baseStart, now)
	if monthlyQuotaWindowExpired(baseStart, now) {
		base = 0
	}
	if refusal <= 0 {
		return baseExpiry
	}
	refusalExpiry := q.MonthlyStart.Add(30 * 24 * time.Hour)
	if base <= 0 {
		return refusalExpiry
	}
	if baseExpiry.Equal(refusalExpiry) {
		return baseExpiry
	}
	if baseExpiry.Before(refusalExpiry) {
		if QuantizeUsageBillingAmount(refusal) < limit {
			return baseExpiry
		}
		return refusalExpiry
	}
	if combinedRefusalQuotaUsage(base, 0) < limit {
		return refusalExpiry
	}
	return baseExpiry
}

func (s *BillingCacheService) refusalQuotaUsage(ctx context.Context, userID int64, platform string) (*GrokRefusalQuotaUsage, error) {
	reader, ok := s.userPlatformQuotaRepo.(GrokRefusalQuotaReader)
	if !ok || platform != PlatformGrok {
		return nil, nil
	}
	quota, err := reader.GetGrokRefusalQuotaUsage(ctx, userID, platform)
	if err != nil {
		return nil, ErrBillingServiceUnavailable.WithCause(err)
	}
	return quota, nil
}
