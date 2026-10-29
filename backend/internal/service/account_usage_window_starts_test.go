package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"

	"github.com/stretchr/testify/require"
)

type windowStartLegacyRepo struct {
	UsageLogRepository
	starts map[int64]time.Time
}

func (r *windowStartLegacyRepo) GetAccountWindowStats(_ context.Context, id int64, start time.Time) (*usagestats.AccountStats, error) {
	r.starts[id] = start
	switch id {
	case 2:
		return nil, errors.New("query failure")
	case 3:
		return nil, nil
	case 4:
		return &usagestats.AccountStats{}, nil
	}
	return &usagestats.AccountStats{StandardCost: 2.5}, nil
}

type windowStartBatchRepo struct {
	*windowStartLegacyRepo
	calls int
	err   error
}

func (r *windowStartBatchRepo) GetAccountWindowStatsByStart(_ context.Context, starts map[int64]time.Time) (map[int64]*usagestats.AccountStats, error) {
	r.calls++
	return map[int64]*usagestats.AccountStats{1: {StandardCost: 9}}, r.err
}
func TestAccountUsageWindowStartsOptionalBatchAndFallback(t *testing.T) {
	starts := map[int64]time.Time{1: time.Now().Add(-time.Hour), 2: time.Now().Add(-2 * time.Hour), 3: time.Now().Add(-3 * time.Hour), 4: time.Now().Add(-4 * time.Hour)}
	legacy := &windowStartLegacyRepo{starts: map[int64]time.Time{}}
	svc := &AccountUsageService{usageLogRepo: legacy}
	stats, err := svc.GetAccountWindowStatsBatchByStart(context.Background(), starts)
	require.NoError(t, err)
	require.Equal(t, starts, legacy.starts)
	require.Len(t, stats, 2)
	require.Equal(t, 2.5, stats[1].StandardCost)
	require.Zero(t, *stats[4])
	require.NotContains(t, stats, int64(2))
	require.NotContains(t, stats, int64(3))
	for _, failure := range []error{nil, errors.New("batch unavailable")} {
		repo := &windowStartBatchRepo{windowStartLegacyRepo: &windowStartLegacyRepo{starts: map[int64]time.Time{}}, err: failure}
		svc.usageLogRepo = repo
		stats, err = svc.GetAccountWindowStatsBatchByStart(context.Background(), starts)
		require.NoError(t, err)
		require.Equal(t, 1, repo.calls)
		if failure == nil {
			require.Equal(t, 9.0, stats[1].StandardCost)
			require.Empty(t, repo.starts)
		} else {
			require.Equal(t, 2.5, stats[1].StandardCost)
			require.Equal(t, starts, repo.starts)
		}
	}
	empty, err := (&AccountUsageService{}).GetAccountWindowStatsBatchByStart(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}
