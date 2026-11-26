//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type cursorRefreshRepo struct {
	channelMonitorV2RepoStub
	watermark *ChannelMonitorV2AggregationWatermark
	history   int
}

func (r *cursorRefreshRepo) GetAggregationWatermark(context.Context) (*ChannelMonitorV2AggregationWatermark, error) {
	return r.watermark, nil
}

func (r *cursorRefreshRepo) RecomputeRange(context.Context, time.Time, time.Time) error {
	r.history++
	return nil
}

func TestChannelMonitorV2RefreshesEarlierDurableCursor(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Minute)
	for _, tc := range []struct {
		name     string
		dbCursor time.Time
		want     time.Time
		history  int
	}{
		{"peer completed history", now.Add(-91 * 24 * time.Hour), now.Add(-91 * 24 * time.Hour), 0},
		{"later cursor cannot rewind", now.Add(-10 * 24 * time.Hour), now.Add(-30 * 24 * time.Hour), 1},
		{"empty cursor cannot rewind", time.Time{}, now.Add(-30 * 24 * time.Hour), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &cursorRefreshRepo{watermark: &ChannelMonitorV2AggregationWatermark{
				HasData: true, DataThrough: now, BackfillCursor: tc.dbCursor,
			}}
			a := NewChannelMonitorV2Aggregator(repo, nil, nil)
			a.cursorLoaded = true
			a.hasAggregated = true
			a.backfillAt = now.Add(-30 * 24 * time.Hour)
			a.liveThrough = now
			a.lastBackfillAt = now.Add(-time.Hour)
			a.runOnce()
			require.Equal(t, tc.history, repo.history)
			if tc.history == 0 {
				require.Equal(t, tc.want, a.backfillAt)
			} else {
				require.True(t, a.backfillAt.Before(tc.want), "only successful local history work advances the cursor")
			}
		})
	}
}
