package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"golang.org/x/sync/singleflight"
)

var ErrLeaderboardPeriod = errors.New("period must be day, week, month, or year")

// LeaderboardUsageBucket contains only public, requested-model aggregates.
type LeaderboardUsageBucket struct {
	Date        time.Time
	Model       string
	TotalTokens int64
	Requests    int64
}

// LeaderboardUsageReader deliberately stays separate from UsageLogRepository.
type LeaderboardUsageReader interface {
	GetLeaderboardUsage(context.Context, time.Time, time.Time, string) ([]LeaderboardUsageBucket, error)
}

type LeaderboardModel struct {
	Model            string `json:"model"`
	TotalTokens      int64  `json:"total_tokens"`
	Requests         int64  `json:"requests"`
	PreviousTokens   int64  `json:"previous_tokens"`
	PreviousRequests int64  `json:"previous_requests"`
}

type LeaderboardTrend struct {
	Date        time.Time `json:"date"`
	Model       string    `json:"model"`
	TotalTokens int64     `json:"total_tokens"`
}

type LeaderboardResponse struct {
	Period        string             `json:"period"`
	Timezone      string             `json:"timezone"`
	Start         time.Time          `json:"start"`
	End           time.Time          `json:"end"`
	PreviousStart time.Time          `json:"previous_start"`
	PreviousEnd   time.Time          `json:"previous_end"`
	UpdatedAt     time.Time          `json:"updated_at"`
	Granularity   string             `json:"granularity"`
	TotalTokens   int64              `json:"total_tokens"`
	TotalRequests int64              `json:"total_requests"`
	Models        []LeaderboardModel `json:"models"`
	Trend         []LeaderboardTrend `json:"trend"`
}

type LeaderboardService struct {
	reader LeaderboardUsageReader
	now    func() time.Time
	mu     sync.Mutex
	cache  map[string]*LeaderboardResponse
	flight singleflight.Group
}

func NewLeaderboardService(reader LeaderboardUsageReader) *LeaderboardService {
	return &LeaderboardService{reader: reader, now: time.Now, cache: make(map[string]*LeaderboardResponse)}
}

func ProvideLeaderboardService(repo UsageLogRepository) *LeaderboardService {
	reader, _ := repo.(LeaderboardUsageReader)
	return NewLeaderboardService(reader)
}

func leaderboardWindow(period string, now time.Time) (*LeaderboardResponse, error) {
	if period == "" {
		period = "week"
	}
	// Calendar arithmetic must not inherit a normalized, nonexistent midnight.
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	var startDate, previousDate time.Time
	granularity := "day"
	switch period {
	case "day":
		startDate, granularity = day, "hour"
		previousDate = startDate.AddDate(0, 0, -1)
	case "week":
		startDate = day.AddDate(0, 0, -(int(day.Weekday())+6)%7)
		previousDate = startDate.AddDate(0, 0, -7)
	case "month":
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		previousDate = startDate.AddDate(0, -1, 0)
	case "year":
		startDate, granularity = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC), "month"
		previousDate = startDate.AddDate(-1, 0, 0)
	default:
		return nil, ErrLeaderboardPeriod
	}
	start := leaderboardCalendarStart(startDate, now.Location())
	previous := leaderboardCalendarStart(previousDate, now.Location())
	previousEnd := previous.Add(now.Sub(start))
	if previousEnd.After(start) {
		previousEnd = start
	}
	return &LeaderboardResponse{
		Period: period, Timezone: now.Location().String(), Start: start, End: now,
		PreviousStart: previous, PreviousEnd: previousEnd, UpdatedAt: now, Granularity: granularity,
		Models: make([]LeaderboardModel, 0), Trend: make([]LeaderboardTrend, 0),
	}, nil
}

// Resolve the first real instant of a calendar date. time.Date alone may pick
// yesterday when DST skips midnight, or the second of two repeated midnights.
func leaderboardCalendarStart(date time.Time, loc *time.Location) time.Time {
	approx := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, loc)
	cursor, limit := approx.Add(-48*time.Hour), approx.Add(48*time.Hour)
	for cursor.Before(limit) {
		_, offset := cursor.Zone()
		_, end := cursor.ZoneBounds()
		// Go's POSIX timezone extrapolation can return a past interval end
		// around leap-year boundaries. Never let that stall the bounded scan.
		if !end.IsZero() && !end.After(cursor) {
			end = cursor.Add(time.Hour)
		}
		if end.IsZero() || end.After(limit) {
			end = limit
		}
		candidate := date.Add(-time.Duration(offset) * time.Second)
		if candidate.Before(cursor) {
			candidate = cursor
		}
		if candidate.Before(end) {
			return candidate.In(loc)
		}
		cursor = end
	}
	return approx
}

func (s *LeaderboardService) cached(window *LeaderboardResponse) *LeaderboardResponse {
	s.mu.Lock()
	defer s.mu.Unlock()
	value := s.cache[window.Period]
	if value != nil && value.Timezone == window.Timezone && value.Start.Equal(window.Start) &&
		leaderboardBucketStart(window.End, window.Granularity).Equal(leaderboardBucketStart(value.End, value.Granularity)) &&
		!window.End.Before(value.End) && window.End.Sub(value.UpdatedAt) < time.Minute {
		return value
	}
	return nil
}

func leaderboardBucketStart(t time.Time, granularity string) time.Time {
	switch granularity {
	case "hour":
		return t.Add(-time.Duration(t.Minute())*time.Minute - time.Duration(t.Second())*time.Second - time.Duration(t.Nanosecond()))
	case "month":
		return leaderboardCalendarStart(time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC), t.Location())
	default:
		return leaderboardCalendarStart(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), t.Location())
	}
}

// Get caches only aggregates. Feature and authentication checks belong to the
// handler and run before every call, including cache hits.
func (s *LeaderboardService) Get(ctx context.Context, period string) (*LeaderboardResponse, error) {
	window, err := leaderboardWindow(period, s.now().In(timezone.Location()))
	if err != nil {
		return nil, err
	}
	if value := s.cached(window); value != nil {
		return value, nil
	}
	flightKey := window.Period + ":" + window.Timezone + ":" + leaderboardBucketStart(window.End, window.Granularity).Format(time.RFC3339)
	result := s.flight.DoChan(flightKey, func() (any, error) {
		window, err := leaderboardWindow(window.Period, s.now().In(timezone.Location()))
		if err != nil {
			return nil, err
		}
		if value := s.cached(window); value != nil {
			return value, nil
		}
		if s.reader == nil {
			return nil, fmt.Errorf("leaderboard usage reader is unavailable")
		}
		// One cancelled visitor must not cancel a query shared by other visitors.
		queryCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		current, err := s.reader.GetLeaderboardUsage(queryCtx, window.Start, window.End, window.Granularity)
		if err != nil {
			return nil, err
		}
		previous, err := s.reader.GetLeaderboardUsage(queryCtx, window.PreviousStart, window.PreviousEnd, window.Granularity)
		if err != nil {
			return nil, err
		}
		models := make(map[string]LeaderboardModel)
		for _, row := range current {
			model := models[row.Model]
			model.Model = row.Model
			model.TotalTokens += row.TotalTokens
			model.Requests += row.Requests
			models[row.Model] = model
			window.TotalTokens += row.TotalTokens
			window.TotalRequests += row.Requests
			window.Trend = append(window.Trend, LeaderboardTrend{Date: row.Date.In(timezone.Location()), Model: row.Model, TotalTokens: row.TotalTokens})
		}
		for _, row := range previous {
			model := models[row.Model]
			model.Model = row.Model
			model.PreviousTokens += row.TotalTokens
			model.PreviousRequests += row.Requests
			models[row.Model] = model
		}
		for _, model := range models {
			window.Models = append(window.Models, model)
		}
		sort.Slice(window.Models, func(i, j int) bool {
			a, b := window.Models[i], window.Models[j]
			if a.TotalTokens != b.TotalTokens {
				return a.TotalTokens > b.TotalTokens
			}
			return a.Model < b.Model
		})
		sort.Slice(window.Trend, func(i, j int) bool {
			a, b := window.Trend[i], window.Trend[j]
			if !a.Date.Equal(b.Date) {
				return a.Date.Before(b.Date)
			}
			return a.Model < b.Model
		})
		s.mu.Lock()
		s.cache[window.Period] = window
		s.mu.Unlock()
		return window, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case value := <-result:
		if value.Err != nil {
			return nil, value.Err
		}
		return value.Val.(*LeaderboardResponse), nil
	}
}
