package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type groupCapacityAccountRepoStub struct {
	AccountRepository
	rows      []GroupAccountCapacityRow
	requested []int64
}

func (s *groupCapacityAccountRepoStub) ListSchedulableCapacityByGroupIDs(_ context.Context, groupIDs []int64) ([]GroupAccountCapacityRow, error) {
	s.requested = append([]int64(nil), groupIDs...)
	return append([]GroupAccountCapacityRow(nil), s.rows...), nil
}

type groupCapacityGroupRepoStub struct {
	GroupRepository
	groupIDs  []int64
	listCalls int
}

func (s *groupCapacityGroupRepoStub) ListActiveIDs(context.Context) ([]int64, error) {
	s.listCalls++
	return append([]int64(nil), s.groupIDs...), nil
}

type groupCapacityConcurrencyCacheStub struct {
	ConcurrencyCache
	counts    map[int64]int
	requested []int64
	err       error
}

func (s *groupCapacityConcurrencyCacheStub) GetAccountConcurrencyBatch(_ context.Context, accountIDs []int64) (map[int64]int, error) {
	s.requested = append([]int64(nil), accountIDs...)
	out := make(map[int64]int, len(accountIDs))
	for _, id := range accountIDs {
		out[id] = s.counts[id]
	}
	return out, s.err
}

type groupCapacitySessionCacheStub struct {
	SessionLimitCache
	counts       map[int64]int
	requested    []int64
	idleTimeouts map[int64]time.Duration
	err          error
	missing      map[int64]bool
}

func (s *groupCapacitySessionCacheStub) GetActiveSessionCountBatch(_ context.Context, accountIDs []int64, idleTimeouts map[int64]time.Duration) (map[int64]int, error) {
	s.requested = append([]int64(nil), accountIDs...)
	s.idleTimeouts = make(map[int64]time.Duration, len(idleTimeouts))
	for id, timeout := range idleTimeouts {
		s.idleTimeouts[id] = timeout
	}
	out := make(map[int64]int, len(accountIDs))
	for _, id := range accountIDs {
		if s.missing[id] {
			continue
		}
		out[id] = s.counts[id]
	}
	return out, s.err
}

type groupCapacityRPMCacheStub struct {
	RPMCache
	counts    map[int64]int
	requested []int64
	err       error
}

func (s *groupCapacityRPMCacheStub) GetRPMBatch(_ context.Context, accountIDs []int64) (map[int64]int, error) {
	s.requested = append([]int64(nil), accountIDs...)
	out := make(map[int64]int, len(accountIDs))
	for _, id := range accountIDs {
		out[id] = s.counts[id]
	}
	return out, s.err
}

func TestGetAllGroupCapacityBatchAggregatesRuntimeAndLimits(t *testing.T) {
	accountRepo := &groupCapacityAccountRepoStub{
		rows: []GroupAccountCapacityRow{
			{
				GroupID:     10,
				AccountID:   1,
				Concurrency: 2,
				Extra: map[string]any{
					"max_sessions":                 3,
					"session_idle_timeout_minutes": 7,
					"base_rpm":                     11,
				},
			},
			{
				GroupID:     20,
				AccountID:   1,
				Concurrency: 2,
				Extra: map[string]any{
					"max_sessions":                 3,
					"session_idle_timeout_minutes": 7,
					"base_rpm":                     11,
				},
			},
			{
				GroupID:     20,
				AccountID:   2,
				Concurrency: 4,
				Extra: map[string]any{
					"max_sessions":                 1,
					"session_idle_timeout_minutes": 9,
					"base_rpm":                     13,
				},
			},
		},
	}
	groupRepo := &groupCapacityGroupRepoStub{groupIDs: []int64{10, 20}}
	concurrencyCache := &groupCapacityConcurrencyCacheStub{counts: map[int64]int{1: 1, 2: 2}}
	sessionCache := &groupCapacitySessionCacheStub{counts: map[int64]int{1: 2, 2: 1}}
	rpmCache := &groupCapacityRPMCacheStub{counts: map[int64]int{1: 5, 2: 7}}
	svc := NewGroupCapacityService(
		accountRepo,
		groupRepo,
		NewConcurrencyService(concurrencyCache),
		sessionCache,
		rpmCache,
	)

	results, err := svc.GetAllGroupCapacity(context.Background())
	require.NoError(t, err)

	require.Equal(t, 1, groupRepo.listCalls)
	require.Equal(t, []int64{10, 20}, accountRepo.requested)
	require.Equal(t, []int64{1, 2}, concurrencyCache.requested)
	require.ElementsMatch(t, []int64{1, 2}, sessionCache.requested)
	require.ElementsMatch(t, []int64{1, 2}, rpmCache.requested)
	require.Equal(t, 7*time.Minute, sessionCache.idleTimeouts[1])
	require.Equal(t, 9*time.Minute, sessionCache.idleTimeouts[2])

	require.Equal(t, []GroupCapacitySummary{
		{
			GroupID:         10,
			ConcurrencyUsed: 1,
			ConcurrencyMax:  2,
			SessionsUsed:    2,
			SessionsMax:     3,
			RPMUsed:         5,
			RPMMax:          11,
			RuntimeKnown:    true,
		},
		{
			GroupID:         20,
			ConcurrencyUsed: 3,
			ConcurrencyMax:  6,
			SessionsUsed:    3,
			SessionsMax:     4,
			RPMUsed:         12,
			RPMMax:          24,
			RuntimeKnown:    true,
		},
	}, results)
}

func TestGetAllGroupCapacityBatchKeepsEmptyGroupRows(t *testing.T) {
	accountRepo := &groupCapacityAccountRepoStub{
		rows: []GroupAccountCapacityRow{
			{GroupID: 20, AccountID: 2, Concurrency: 4},
		},
	}
	groupRepo := &groupCapacityGroupRepoStub{groupIDs: []int64{10, 20}}
	svc := NewGroupCapacityService(accountRepo, groupRepo, nil, nil, nil)

	results, err := svc.GetAllGroupCapacity(context.Background())
	require.NoError(t, err)

	require.Equal(t, []GroupCapacitySummary{
		{GroupID: 10, RuntimeKnown: true},
		{GroupID: 20, ConcurrencyMax: 4},
	}, results)
}

func TestGetAllGroupCapacityBatchMarksAllEmptyGroupsKnown(t *testing.T) {
	accountRepo := &groupCapacityAccountRepoStub{}
	groupRepo := &groupCapacityGroupRepoStub{groupIDs: []int64{10, 20}}
	svc := NewGroupCapacityService(accountRepo, groupRepo, nil, nil, nil)

	results, err := svc.GetAllGroupCapacity(context.Background())
	require.NoError(t, err)
	require.Equal(t, []GroupCapacitySummary{
		{GroupID: 10, RuntimeKnown: true},
		{GroupID: 20, RuntimeKnown: true},
	}, results)
}

func TestGetAllGroupCapacityMarksRuntimeUnknownWhenRedisReadFails(t *testing.T) {
	accountRepo := &groupCapacityAccountRepoStub{rows: []GroupAccountCapacityRow{{GroupID: 10, AccountID: 1, Concurrency: 2}}}
	groupRepo := &groupCapacityGroupRepoStub{groupIDs: []int64{10}}
	concurrencyCache := &groupCapacityConcurrencyCacheStub{err: context.DeadlineExceeded}
	svc := NewGroupCapacityService(accountRepo, groupRepo, NewConcurrencyService(concurrencyCache), nil, nil)

	results, err := svc.GetAllGroupCapacity(context.Background())
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, 2, results[0].ConcurrencyMax)
	require.Zero(t, results[0].ConcurrencyUsed)
	require.False(t, results[0].RuntimeKnown)
}

// A failing optional cache must only hide runtime for groups that use that limit.
func TestGetAllGroupCapacityRuntimeKnownPerGroup(t *testing.T) {
	for _, failure := range []string{"sessions", "rpm", "missing-session", "missing-rpm", "missing-concurrency"} {
		t.Run(failure, func(t *testing.T) {
			repo := &groupCapacityAccountRepoStub{rows: []GroupAccountCapacityRow{
				{GroupID: 10, AccountID: 1, Concurrency: 2},
				{GroupID: 20, AccountID: 2, Concurrency: 3, Extra: map[string]any{"max_sessions": 4}},
				{GroupID: 30, AccountID: 3, Concurrency: 5, Extra: map[string]any{"base_rpm": 6}},
			}}
			concurrency := NewConcurrencyService(&groupCapacityConcurrencyCacheStub{})
			var sessions SessionLimitCache = &groupCapacitySessionCacheStub{}
			var rpm RPMCache = &groupCapacityRPMCacheStub{}
			switch failure {
			case "sessions":
				sessions = &groupCapacitySessionCacheStub{err: context.DeadlineExceeded}
			case "rpm":
				rpm = &groupCapacityRPMCacheStub{err: context.DeadlineExceeded}
			case "missing-session":
				sessions = nil
			case "missing-rpm":
				rpm = nil
			case "missing-concurrency":
				concurrency = NewConcurrencyService(nil)
			}
			svc := NewGroupCapacityService(repo, &groupCapacityGroupRepoStub{groupIDs: []int64{10, 20, 30, 40}}, concurrency, sessions, rpm)
			rows, err := svc.GetAllGroupCapacity(context.Background())
			require.NoError(t, err)
			require.Equal(t, failure != "missing-concurrency", rows[0].RuntimeKnown)
			require.Equal(t, failure != "missing-concurrency" && failure != "sessions" && failure != "missing-session", rows[1].RuntimeKnown)
			require.Equal(t, failure != "missing-concurrency" && failure != "rpm" && failure != "missing-rpm", rows[2].RuntimeKnown)
			require.True(t, rows[3].RuntimeKnown)
			require.Equal(t, 3, rows[1].ConcurrencyMax)
			require.Equal(t, 4, rows[1].SessionsMax)
			require.Equal(t, 6, rows[2].RPMMax)
		})
	}
}

type groupCapacitySequentialRepoStub struct {
	AccountRepository
	accounts []Account
}

func (s *groupCapacitySequentialRepoStub) ListSchedulableByGroupID(context.Context, int64) ([]Account, error) {
	return s.accounts, nil
}

func TestGetAllGroupCapacitySequentialHandlesMissingCaches(t *testing.T) {
	repo := &groupCapacitySequentialRepoStub{accounts: []Account{{ID: 1, Concurrency: 2, Extra: map[string]any{"max_sessions": 3, "base_rpm": 4}}}}
	for _, concurrency := range []*ConcurrencyService{nil, NewConcurrencyService(nil), NewConcurrencyService(&groupCapacityConcurrencyCacheStub{})} {
		svc := NewGroupCapacityService(repo, &groupCapacityGroupRepoStub{groupIDs: []int64{10}}, concurrency, nil, nil)
		rows, err := svc.GetAllGroupCapacity(context.Background())
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.False(t, rows[0].RuntimeKnown)
		require.Equal(t, 2, rows[0].ConcurrencyMax)
	}
	repo.accounts = nil
	svc := NewGroupCapacityService(repo, &groupCapacityGroupRepoStub{groupIDs: []int64{10}}, nil, nil, nil)
	rows, err := svc.GetAllGroupCapacity(context.Background())
	require.NoError(t, err)
	require.Equal(t, []GroupCapacitySummary{{GroupID: 10, RuntimeKnown: true}}, rows)
}

// The real session cache keeps successful pipeline commands but may return nil
// error alongside missing failed commands (for example NOSCRIPT after restart).
func TestGetAllGroupCapacityPartialSessionResultsAreUnknown(t *testing.T) {
	concurrency := NewConcurrencyService(&groupCapacityConcurrencyCacheStub{})
	sessions := &groupCapacitySessionCacheStub{missing: map[int64]bool{1: true}}
	t.Run("batch isolates missing accounts and preserves genuine zero", func(t *testing.T) {
		repo := &groupCapacityAccountRepoStub{rows: []GroupAccountCapacityRow{
			{GroupID: 10, AccountID: 1, Concurrency: 2, Extra: map[string]any{"max_sessions": 3}},
			{GroupID: 20, AccountID: 2, Concurrency: 2, Extra: map[string]any{"max_sessions": 3}},
			{GroupID: 30, AccountID: 3, Concurrency: 2},
			{GroupID: 50, AccountID: 1, Concurrency: 2, Extra: map[string]any{"max_sessions": 3}},
		}}
		svc := NewGroupCapacityService(repo, &groupCapacityGroupRepoStub{groupIDs: []int64{10, 20, 30, 40, 50}}, concurrency, sessions, nil)
		rows, err := svc.GetAllGroupCapacity(context.Background())
		require.NoError(t, err)
		require.False(t, rows[0].RuntimeKnown)
		require.True(t, rows[1].RuntimeKnown, "a successful zero count is known")
		require.Zero(t, rows[1].SessionsUsed)
		require.True(t, rows[2].RuntimeKnown, "no session limit")
		require.True(t, rows[3].RuntimeKnown, "empty group")
		require.False(t, rows[4].RuntimeKnown, "shared failed account")
	})
	t.Run("sequential distinguishes missing from genuine zero", func(t *testing.T) {
		repo := &groupCapacitySequentialRepoStub{accounts: []Account{{ID: 1, Concurrency: 2, Extra: map[string]any{"max_sessions": 3}}}}
		svc := NewGroupCapacityService(repo, &groupCapacityGroupRepoStub{groupIDs: []int64{10}}, concurrency, sessions, nil)
		rows, err := svc.GetAllGroupCapacity(context.Background())
		require.NoError(t, err)
		require.False(t, rows[0].RuntimeKnown)
		repo.accounts[0].ID = 2
		rows, err = svc.GetAllGroupCapacity(context.Background())
		require.NoError(t, err)
		require.True(t, rows[0].RuntimeKnown)
		require.Zero(t, rows[0].SessionsUsed)
	})
}
