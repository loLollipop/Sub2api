package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type revisionUserGroupRateRepo struct {
	UserGroupRateRepository
	mu          sync.Mutex
	rate        *float64
	revision    atomic.Uint64
	calls       atomic.Int64
	readStarted chan struct{}
	readRelease chan struct{}
}

func (r *revisionUserGroupRateRepo) UserGroupRateRevision() uint64 { return r.revision.Load() }

func (r *revisionUserGroupRateRepo) GetByUserAndGroup(ctx context.Context, _, _ int64) (*float64, error) {
	call := r.calls.Add(1)
	r.mu.Lock()
	var snapshot *float64
	if r.rate != nil {
		value := *r.rate
		snapshot = &value
	}
	r.mu.Unlock()
	if call == 1 && r.readStarted != nil {
		close(r.readStarted)
		select {
		case <-r.readRelease:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return snapshot, nil
}

func (r *revisionUserGroupRateRepo) setRate(rate *float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rate = nil
	if rate != nil {
		value := *rate
		r.rate = &value
	}
	r.revision.Add(1)
}

func (r *revisionUserGroupRateRepo) SyncUserGroupRates(_ context.Context, _ int64, rates map[int64]*float64) error {
	r.setRate(rates[10])
	return nil
}

func (r *revisionUserGroupRateRepo) SyncGroupRateMultipliers(_ context.Context, _ int64, entries []GroupRateMultiplierInput) error {
	if len(entries) == 0 {
		r.setRate(nil)
	} else {
		r.setRate(&entries[0].RateMultiplier)
	}
	return nil
}

func (r *revisionUserGroupRateRepo) DeleteByGroupID(_ context.Context, _ int64) error {
	r.setRate(nil)
	return nil
}
func (r *revisionUserGroupRateRepo) DeleteByUserID(_ context.Context, _ int64) error {
	r.setRate(nil)
	return nil
}

func TestUserGroupRateRevision_RefreshesBothGatewayCaches(t *testing.T) {
	repo := &revisionUserGroupRateRepo{}
	ctx := context.Background()
	resolvers := []*userGroupRateResolver{
		newUserGroupRateResolver(repo, nil, time.Minute, nil, "service.gateway"),
		newUserGroupRateResolver(repo, nil, time.Minute, nil, "service.openai_gateway"),
	}
	for _, step := range []struct {
		rate *float64
		want float64
	}{
		{nil, 1.5}, {revisionRatePtr(0), 0}, {revisionRatePtr(0.5), 0.5}, {revisionRatePtr(2), 2}, {nil, 1.5},
	} {
		repo.setRate(step.rate)
		before := repo.calls.Load()
		for _, resolver := range resolvers {
			require.Equal(t, 1.5, resolver.ResolveSnapshotOnly(42, 10, 1.5), "stale snapshots must fall back")
		}
		require.Equal(t, before, repo.calls.Load(), "snapshot resolution must be DB-free")
		for _, resolver := range resolvers {
			require.Equal(t, step.want, resolver.Resolve(ctx, 42, 10, 1.5))
			require.Equal(t, step.want, resolver.Resolve(ctx, 42, 10, 1.5))
			require.Equal(t, step.want, resolver.ResolveSnapshotOnly(42, 10, 1.5))
		}
		require.Equal(t, before+2, repo.calls.Load(), "each local resolver loads once per revision")
	}
}

func revisionRatePtr(value float64) *float64 { return &value }

func receiveRevisionRate(t *testing.T, result <-chan float64) float64 {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("rate resolution did not complete")
		return 0
	}
}

func TestUserGroupRateRevision_InFlightOldReadCannotReturnOrRepopulateStaleRate(t *testing.T) {
	for _, old := range []*float64{nil, revisionRatePtr(3)} {
		t.Run(func() string {
			if old == nil {
				return "cached absence"
			}
			return "old override"
		}(), func(t *testing.T) {
			repo := &revisionUserGroupRateRepo{rate: old, readStarted: make(chan struct{}), readRelease: make(chan struct{})}
			resolver := newUserGroupRateResolver(repo, nil, time.Minute, nil, "test")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			oldResult := make(chan float64, 1)
			go func() { oldResult <- resolver.Resolve(ctx, 42, 10, 1.5) }()
			select {
			case <-repo.readStarted:
			case <-ctx.Done():
				t.Fatal("old read did not start")
			}
			repo.setRate(revisionRatePtr(0))
			newResult := make(chan float64, 1)
			go func() { newResult <- resolver.Resolve(ctx, 42, 10, 1.5) }()
			// The new generation must complete before the old query is released:
			// this specifically detects singleflight keys that omit the revision.
			require.Zero(t, receiveRevisionRate(t, newResult))
			close(repo.readRelease)
			require.Zero(t, receiveRevisionRate(t, oldResult))
			require.Zero(t, resolver.ResolveSnapshotOnly(42, 10, 1.5))
			cached, ok := resolver.cache.Get("42:10")
			require.True(t, ok)
			require.Equal(t, cachedUserGroupRate{Present: true, Value: 0, Revision: repo.UserGroupRateRevision()}, cached)
			require.Equal(t, int64(2), repo.calls.Load())
		})
	}
}
