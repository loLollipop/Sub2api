package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

func TestLeaderboardWindowNaturalPeriods(t *testing.T) {
	tests := []struct{ period, now, start, previous, previousEnd, granularity string }{
		{"day", "2025-01-01T12:30:00Z", "2025-01-01T00:00:00Z", "2024-12-31T00:00:00Z", "2024-12-31T12:30:00Z", "hour"},
		{"week", "2025-01-01T12:30:00Z", "2024-12-30T00:00:00Z", "2024-12-23T00:00:00Z", "2024-12-25T12:30:00Z", "day"},
		{"month", "2025-01-31T12:30:00Z", "2025-01-01T00:00:00Z", "2024-12-01T00:00:00Z", "2024-12-31T12:30:00Z", "day"},
		{"month", "2024-03-31T12:30:00Z", "2024-03-01T00:00:00Z", "2024-02-01T00:00:00Z", "2024-03-01T00:00:00Z", "day"},
		{"month", "2025-03-31T12:30:00Z", "2025-03-01T00:00:00Z", "2025-02-01T00:00:00Z", "2025-03-01T00:00:00Z", "day"},
		{"year", "2024-12-31T12:30:00Z", "2024-01-01T00:00:00Z", "2023-01-01T00:00:00Z", "2024-01-01T00:00:00Z", "month"},
		{"year", "2024-02-29T12:30:00Z", "2024-01-01T00:00:00Z", "2023-01-01T00:00:00Z", "2023-03-01T12:30:00Z", "month"},
		{"year", "2025-01-01T00:00:00Z", "2025-01-01T00:00:00Z", "2024-01-01T00:00:00Z", "2024-01-01T00:00:00Z", "month"},
	}
	parse := func(value string) time.Time {
		result, err := time.Parse(time.RFC3339, value)
		require.NoError(t, err)
		return result
	}
	for _, tt := range tests {
		t.Run(tt.period+tt.now, func(t *testing.T) {
			window, err := leaderboardWindow(tt.period, parse(tt.now))
			require.NoError(t, err)
			require.Equal(t, parse(tt.start), window.Start)
			require.Equal(t, parse(tt.previous), window.PreviousStart)
			require.Equal(t, parse(tt.previousEnd), window.PreviousEnd)
			require.Equal(t, tt.granularity, window.Granularity)
			require.False(t, window.PreviousEnd.After(window.Start))
		})
	}
	window, err := leaderboardWindow("", parse("2025-01-01T12:30:00Z"))
	require.NoError(t, err)
	require.Equal(t, "week", window.Period)
	_, err = leaderboardWindow("all", time.Now())
	require.ErrorIs(t, err, ErrLeaderboardPeriod)
}

func TestLeaderboardWindowDSTUsesElapsedDuration(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	for _, tt := range []struct {
		month             time.Month
		day, previousHour int
	}{{time.March, 10, 11}, {time.November, 3, 13}} {
		now := time.Date(2024, tt.month, tt.day, 12, 0, 0, 0, loc)
		window, err := leaderboardWindow("day", now)
		require.NoError(t, err)
		require.Equal(t, tt.previousHour, window.PreviousEnd.Hour())
		require.Equal(t, now.Sub(window.Start), window.PreviousEnd.Sub(window.PreviousStart))
		require.Equal(t, "America/New_York", window.Timezone)
	}
	// Both occurrences of the fall-back hour remain distinct cache buckets.
	first := time.Date(2024, 11, 3, 5, 30, 0, 0, time.UTC).In(loc)
	second := first.Add(time.Hour)
	require.False(t, leaderboardBucketStart(first, "hour").Equal(leaderboardBucketStart(second, "hour")))
}

func TestLeaderboardWindowMidnightTransitions(t *testing.T) {
	for _, tc := range []struct{ zone, now, period, start, previous string }{
		{"America/Havana", "2026-03-08T12:00:00-04:00", "day", "2026-03-08T01:00:00-04:00", "2026-03-07T00:00:00-05:00"},
		{"America/Havana", "2026-03-08T12:00:00-04:00", "week", "2026-03-02T00:00:00-05:00", "2026-02-23T00:00:00-05:00"},
		{"America/Havana", "2026-03-08T12:00:00-04:00", "month", "2026-03-01T00:00:00-05:00", "2026-02-01T00:00:00-05:00"},
		{"America/Havana", "2026-11-01T12:00:00-05:00", "day", "2026-11-01T00:00:00-04:00", "2026-10-31T00:00:00-04:00"},
		{"America/Santiago", "2024-09-08T12:00:00-03:00", "day", "2024-09-08T01:00:00-03:00", "2024-09-07T00:00:00-04:00"},
	} {
		t.Run(tc.zone+tc.period+tc.now, func(t *testing.T) {
			loc, err := time.LoadLocation(tc.zone)
			require.NoError(t, err)
			now, err := time.Parse(time.RFC3339, tc.now)
			require.NoError(t, err)
			window, err := leaderboardWindow(tc.period, now.In(loc))
			require.NoError(t, err)
			require.Equal(t, tc.start, window.Start.Format(time.RFC3339))
			require.Equal(t, tc.previous, window.PreviousStart.Format(time.RFC3339))
			require.Equal(t, window.Start, leaderboardBucketStart(window.Start, window.Granularity))
		})
	}
}

func TestLeaderboardWindowYearAcrossLeapBoundary(t *testing.T) {
	for _, tc := range []struct{ zone, previous string }{
		{"America/New_York", "2025-01-01T00:00:00-05:00"},
		{"America/Havana", "2025-01-01T00:00:00-05:00"},
		{"Europe/Paris", "2025-01-01T00:00:00+01:00"},
		{"Australia/Sydney", "2025-01-01T00:00:00+11:00"},
	} {
		t.Run(tc.zone, func(t *testing.T) {
			loc, err := time.LoadLocation(tc.zone)
			require.NoError(t, err)
			done := make(chan *LeaderboardResponse, 1)
			go func() {
				window, _ := leaderboardWindow("year", time.Date(2026, 10, 25, 12, 0, 0, 0, loc))
				done <- window
			}()
			select {
			case window := <-done:
				require.NotNil(t, window)
				require.Equal(t, tc.previous, window.PreviousStart.Format(time.RFC3339))
			case <-time.After(2 * time.Second):
				t.Fatal("calendar scan stalled at a leap-year timezone boundary")
			}
		})
	}
}

type leaderboardReaderFunc func(context.Context, time.Time, time.Time, string) ([]LeaderboardUsageBucket, error)

func (f leaderboardReaderFunc) GetLeaderboardUsage(ctx context.Context, start, end time.Time, granularity string) ([]LeaderboardUsageBucket, error) {
	return f(ctx, start, end, granularity)
}

func TestLeaderboardGetTotalsPreviousOnlyAndCacheBoundary(t *testing.T) {
	now := time.Date(2025, 1, 8, 23, 59, 40, 0, timezone.Location())
	var calls int
	svc := NewLeaderboardService(leaderboardReaderFunc(func(_ context.Context, start, end time.Time, granularity string) ([]LeaderboardUsageBucket, error) {
		calls++
		if start.Day() == 6 {
			return []LeaderboardUsageBucket{{Date: start, Model: "a", TotalTokens: 100, Requests: 2}, {Date: start.AddDate(0, 0, 1), Model: "a", TotalTokens: 20, Requests: 1}, {Date: start, Model: "b", TotalTokens: 60, Requests: 1}}, nil
		}
		return []LeaderboardUsageBucket{{Date: start, Model: "a", TotalTokens: 200, Requests: 3}, {Date: start, Model: "retired", TotalTokens: 500, Requests: 5}}, nil
	}))
	svc.now = func() time.Time { return now }
	result, err := svc.Get(context.Background(), "week")
	require.NoError(t, err)
	require.Equal(t, int64(180), result.TotalTokens)
	require.Equal(t, int64(4), result.TotalRequests)
	require.Equal(t, []LeaderboardModel{{Model: "a", TotalTokens: 120, Requests: 3, PreviousTokens: 200, PreviousRequests: 3}, {Model: "b", TotalTokens: 60, Requests: 1}, {Model: "retired", PreviousTokens: 500, PreviousRequests: 5}}, result.Models)
	require.Len(t, result.Trend, 3)
	_, err = svc.Get(context.Background(), "")
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	now = now.Add(30 * time.Second) // Midnight invalidates even a 30-second-old week response.
	_, err = svc.Get(context.Background(), "week")
	require.NoError(t, err)
	require.Equal(t, 4, calls)
	now = now.Add(61 * time.Second)
	_, err = svc.Get(context.Background(), "week")
	require.NoError(t, err)
	require.Equal(t, 6, calls)
	require.Len(t, svc.cache, 1)
}

func TestLeaderboardGetEmptyAndFailedReads(t *testing.T) {
	var calls int
	svc := NewLeaderboardService(leaderboardReaderFunc(func(context.Context, time.Time, time.Time, string) ([]LeaderboardUsageBucket, error) {
		calls++
		return nil, nil
	}))
	result, err := svc.Get(context.Background(), "week")
	require.NoError(t, err)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"models":[]`)
	require.Contains(t, string(encoded), `"trend":[]`)
	_, err = svc.Get(context.Background(), "invalid")
	require.ErrorIs(t, err, ErrLeaderboardPeriod)
	require.Equal(t, 2, calls)
	broken := NewLeaderboardService(leaderboardReaderFunc(func(context.Context, time.Time, time.Time, string) ([]LeaderboardUsageBucket, error) {
		return nil, errors.New("offline")
	}))
	_, err = broken.Get(context.Background(), "week")
	require.EqualError(t, err, "offline")
	require.Empty(t, broken.cache)
}

func TestLeaderboardConcurrentRequestsShareQueries(t *testing.T) {
	var calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	svc := NewLeaderboardService(leaderboardReaderFunc(func(context.Context, time.Time, time.Time, string) ([]LeaderboardUsageBucket, error) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		return nil, nil
	}))
	var wg sync.WaitGroup
	errors := make(chan error, 20)
	for range 20 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := svc.Get(context.Background(), "week"); errors <- err }()
	}
	<-entered
	close(release)
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	require.Equal(t, int32(2), calls.Load())
}
