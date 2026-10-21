//go:build integration

package repository

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func newGrokRefusalIntegrationFixture(t *testing.T) (mediaBillingIntegrationFixture, *userPlatformQuotaServiceAdapter) {
	t.Helper()
	f := newMediaBillingIntegrationFixture(t, 10)
	quota := &userPlatformQuotaServiceAdapter{inner: &userPlatformQuotaRepository{client: testEntClient(t)}}
	limit := 100.0
	require.NoError(t, quota.BulkInsertInitial(context.Background(), []service.UserPlatformQuotaRecord{{
		UserID: f.user.ID, Platform: service.PlatformGrok,
		DailyLimitUSD: &limit, WeeklyLimitUSD: &limit, MonthlyLimitUSD: &limit,
	}}))
	// Commits use the shared database rather than a wrapping test transaction.
	// Remove pending jobs so unrelated worker-lease tests cannot claim them.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := integrationDB.ExecContext(ctx, `DELETE FROM grok_refusal_blocks WHERE user_id = $1`, f.user.ID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, `DELETE FROM media_billing_jobs WHERE user_id = $1`, f.user.ID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, `DELETE FROM usage_billing_dedup WHERE api_key_id = $1`, f.apiKey.ID)
		require.NoError(t, err)
	})
	return f, quota
}

func grokRefusalIntegrationJob(f mediaBillingIntegrationFixture, request string, cost float64) *service.MediaBillingJob {
	intent := f.intent(request, cost)
	intent.QuotaPlatform = service.PlatformGrok
	intent.Command.Normalize()
	return &service.MediaBillingJob{
		ID: "grok-refusal:" + request, Kind: service.MediaBillingKindGrokRefusal,
		UserID: f.user.ID, APIKeyID: f.apiKey.ID, AccountID: f.account.ID, GroupID: f.group.ID,
		Status: service.MediaBillingReady, Intent: intent,
	}
}

func grokRefusalIntegrationHash(identity string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
}

func assertGrokRefusalIntegrationState(t *testing.T, f mediaBillingIntegrationFixture, quota *userPlatformQuotaServiceAdapter, balance, cost float64, jobs, blocks, dedup int) {
	t.Helper()
	ctx := context.Background()
	assertMediaBillingWallet(t, ctx, f.user.ID, balance, 0)
	usage, err := quota.GetGrokRefusalQuotaUsage(ctx, f.user.ID, service.PlatformGrok)
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.InDelta(t, cost, usage.Daily, 1e-8)
	require.InDelta(t, cost, usage.Weekly, 1e-8)
	require.InDelta(t, cost, usage.Monthly, 1e-8)
	var gotJobs, gotBlocks, gotDedup int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT
		 (SELECT COUNT(*) FROM media_billing_jobs WHERE user_id = $1 AND kind = 'grok_refusal'),
		 (SELECT COUNT(*) FROM grok_refusal_blocks WHERE user_id = $1),
		 (SELECT COUNT(*) FROM usage_billing_dedup WHERE api_key_id = $2)
	`, f.user.ID, f.apiKey.ID).Scan(&gotJobs, &gotBlocks, &gotDedup))
	require.Equal(t, jobs, gotJobs, "durable jobs")
	require.Equal(t, blocks, gotBlocks, "conversation blocks")
	require.Equal(t, dedup, gotDedup, "request dedup claims")
}

func TestGrokRefusalIntegration_AtomicCommitAndDuplicate(t *testing.T) {
	ctx := context.Background()
	f, quota := newGrokRefusalIntegrationFixture(t)
	job := grokRefusalIntegrationJob(f, fmt.Sprintf("atomic-%d", f.user.ID), 0.25)
	key := grokRefusalIntegrationHash("conversation")
	committed, created, err := f.repo.CommitGrokRefusal(ctx, job, []string{key}, []string{key})
	require.NoError(t, err)
	require.True(t, created)
	require.NotNil(t, committed.CommitResult)
	require.True(t, committed.CommitResult.Applied)
	assertGrokRefusalIntegrationState(t, f, quota, 9.75, 0.25, 1, 1, 1)

	persisted, err := f.repo.GetMediaBillingJobByID(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, service.MediaBillingBilled, persisted.Status)
	require.Equal(t, job.Intent, persisted.Intent, "recoverable intent survives a repository reload")
	var fingerprint, blockJob string
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT request_fingerprint FROM usage_billing_dedup WHERE request_id = $1 AND api_key_id = $2`,
		job.Intent.Command.RequestID, f.apiKey.ID).Scan(&fingerprint))
	require.Equal(t, job.Intent.Command.RequestFingerprint, fingerprint)
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT job_id FROM grok_refusal_blocks WHERE user_id = $1 AND identity_hash = $2`, f.user.ID, key).Scan(&blockJob))
	require.Equal(t, job.ID, blockJob)

	// A retry with only the request ID exercises job idempotency independently
	// of the conversation lookup; another request learns an additional identity.
	duplicate, created, err := f.repo.CommitGrokRefusal(ctx, job, nil, []string{key})
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, job.ID, duplicate.ID)
	other := grokRefusalIntegrationJob(f, fmt.Sprintf("duplicate-%d", f.user.ID), 0.25)
	additional := grokRefusalIntegrationHash("extended-conversation")
	duplicate, created, err = f.repo.CommitGrokRefusal(ctx, other, []string{key}, []string{additional})
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, job.ID, duplicate.ID)
	found, err := f.repo.FindGrokRefusal(ctx, f.user.ID, []string{additional})
	require.NoError(t, err)
	require.True(t, found)
	assertGrokRefusalIntegrationState(t, f, quota, 9.75, 0.25, 1, 2, 1)
}

func TestGrokRefusalIntegration_ConcurrentConversationAndUserIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	first, firstQuota := newGrokRefusalIntegrationFixture(t)
	second, secondQuota := newGrokRefusalIntegrationFixture(t)
	key := grokRefusalIntegrationHash("same-conversation-for-both-users")
	type result struct {
		userID  int64
		job     *service.MediaBillingJob
		created bool
		err     error
	}
	const requestsPerUser = 4
	results := make(chan result, 2*requestsPerUser)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for _, f := range []mediaBillingIntegrationFixture{first, second} {
		for i := 0; i < requestsPerUser; i++ {
			job := grokRefusalIntegrationJob(f, fmt.Sprintf("concurrent-%d-%d", f.user.ID, i), 0.25)
			workers.Add(1)
			go func() {
				defer workers.Done()
				<-start
				persisted, created, err := f.repo.CommitGrokRefusal(ctx, job, []string{key}, []string{key})
				results <- result{userID: f.user.ID, job: persisted, created: created, err: err}
			}()
		}
	}
	close(start)
	workers.Wait()
	close(results)
	createdByUser := map[int64]int{}
	jobByUser := map[int64]string{}
	for r := range results {
		require.NoError(t, r.err)
		require.NotNil(t, r.job)
		require.Equal(t, r.userID, r.job.UserID)
		if r.created {
			createdByUser[r.userID]++
		}
		if existing := jobByUser[r.userID]; existing != "" {
			require.Equal(t, existing, r.job.ID, "all requests for one user return the same job")
		}
		jobByUser[r.userID] = r.job.ID
	}
	require.Equal(t, 1, createdByUser[first.user.ID])
	require.Equal(t, 1, createdByUser[second.user.ID])
	require.NotEqual(t, jobByUser[first.user.ID], jobByUser[second.user.ID])
	assertGrokRefusalIntegrationState(t, first, firstQuota, 9.75, 0.25, 1, 1, 1)
	assertGrokRefusalIntegrationState(t, second, secondQuota, 9.75, 0.25, 1, 1, 1)
}

func TestGrokRefusalIntegration_BlockConstraintFailureRollsBackEverything(t *testing.T) {
	ctx := context.Background()
	f, quota := newGrokRefusalIntegrationFixture(t)
	job := grokRefusalIntegrationJob(f, fmt.Sprintf("rollback-%d", f.user.ID), 0.25)
	key := grokRefusalIntegrationHash("rollback-conversation")
	// Correct length passes application validation; PostgreSQL rejects the
	// non-hex value after all accounting writes and the first block insertion.
	committed, created, err := f.repo.CommitGrokRefusal(ctx, job, []string{key}, []string{key, strings.Repeat("g", 64)})
	require.Error(t, err)
	var pgErr *pq.Error
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, pq.ErrorCode("23514"), pgErr.Code, "real PostgreSQL CHECK violation")
	require.False(t, created)
	require.Nil(t, committed)
	assertGrokRefusalIntegrationState(t, f, quota, 10, 0, 0, 0, 0)
	found, err := f.repo.FindGrokRefusal(ctx, f.user.ID, []string{key})
	require.NoError(t, err)
	require.False(t, found)

	// The same request remains retryable because its dedup claim rolled back.
	_, created, err = f.repo.CommitGrokRefusal(ctx, job, []string{key}, []string{key})
	require.NoError(t, err)
	require.True(t, created)
	assertGrokRefusalIntegrationState(t, f, quota, 9.75, 0.25, 1, 1, 1)
}

func TestGrokRefusalIntegration_ZeroPriceStillBlocks(t *testing.T) {
	ctx := context.Background()
	f, quota := newGrokRefusalIntegrationFixture(t)
	job := grokRefusalIntegrationJob(f, fmt.Sprintf("free-%d", f.user.ID), 0)
	key := grokRefusalIntegrationHash("free-conversation")
	committed, created, err := f.repo.CommitGrokRefusal(ctx, job, []string{key}, []string{key})
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, service.MediaBillingBilled, committed.Status)
	found, err := f.repo.FindGrokRefusal(ctx, f.user.ID, []string{key})
	require.NoError(t, err)
	require.True(t, found)
	_, created, err = f.repo.CommitGrokRefusal(ctx, job, []string{key}, []string{key})
	require.NoError(t, err)
	require.False(t, created)
	assertGrokRefusalIntegrationState(t, f, quota, 10, 0, 1, 1, 1)
}

func TestGrokRefusalIntegration_AbsoluteRedisSnapshotAndAdminReset(t *testing.T) {
	ctx := context.Background()
	f, quota := newGrokRefusalIntegrationFixture(t)
	cache := &billingCache{rdb: testRedis(t)}
	now := time.Now().UTC().Truncate(time.Second)
	day, week := timezone.StartOfDay(now), timezone.StartOfWeek(now)
	require.NoError(t, cache.SetUserPlatformQuotaCache(ctx, f.user.ID, service.PlatformGrok, &service.UserPlatformQuotaCacheEntry{
		DailyUsageUSD: 1, WeeklyUsageUSD: 2, MonthlyUsageUSD: 3,
		DailyWindowStart: &day, WeeklyWindowStart: &week, MonthlyWindowStart: &now,
		SchemaVersion: service.UserPlatformQuotaCacheSchemaV1,
	}, time.Minute))
	job := grokRefusalIntegrationJob(f, fmt.Sprintf("snapshot-%d", f.user.ID), 0.25)
	key := grokRefusalIntegrationHash("snapshot-conversation")
	_, created, err := f.repo.CommitGrokRefusal(ctx, job, []string{key}, []string{key})
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, cache.IncrUserPlatformQuotaUsageCache(ctx, f.user.ID, service.PlatformGrok, 0.5, time.Minute, false))
	entry, hit, err := cache.GetUserPlatformQuotaCache(ctx, f.user.ID, service.PlatformGrok)
	require.NoError(t, err)
	require.True(t, hit)
	require.InDelta(t, 1.5, entry.DailyUsageUSD, 1e-8, "Redis contains only ordinary usage")
	require.NotNil(t, entry.DailyWindowStart)
	require.NotNil(t, entry.WeeklyWindowStart)
	require.NotNil(t, entry.MonthlyWindowStart)
	snapshot := service.UserPlatformQuotaSnapshot{
		UserID: f.user.ID, Platform: service.PlatformGrok,
		DailyUsageUSD: entry.DailyUsageUSD, WeeklyUsageUSD: entry.WeeklyUsageUSD, MonthlyUsageUSD: entry.MonthlyUsageUSD,
		DailyWindowStart: *entry.DailyWindowStart, WeeklyWindowStart: *entry.WeeklyWindowStart, MonthlyWindowStart: *entry.MonthlyWindowStart,
	}
	// The flusher's absolute write can be replayed without losing or duplicating
	// the separately committed refusal component.
	for i := 0; i < 2; i++ {
		require.NoError(t, quota.BatchSnapshotUsage(ctx, []service.UserPlatformQuotaSnapshot{snapshot}, now))
	}
	assertGrokRefusalIntegrationState(t, f, quota, 9.75, 0.25, 1, 1, 1)
	base, err := quota.GetByUserPlatform(ctx, f.user.ID, service.PlatformGrok)
	require.NoError(t, err)
	require.NotNil(t, base)
	require.InDelta(t, 1.5, base.DailyUsageUSD, 1e-8)
	require.InDelta(t, 2.5, base.WeeklyUsageUSD, 1e-8)
	require.InDelta(t, 3.5, base.MonthlyUsageUSD, 1e-8)
	combined := [3]float64{1.75, 2.75, 3.75}
	assertCombined := func() {
		t.Helper()
		rows, err := quota.ListByUser(ctx, f.user.ID)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.InDelta(t, combined[0], rows[0].DailyUsageUSD, 1e-8)
		require.InDelta(t, combined[1], rows[0].WeeklyUsageUSD, 1e-8)
		require.InDelta(t, combined[2], rows[0].MonthlyUsageUSD, 1e-8)
	}
	assertCombined()
	for i, window := range []string{"daily", "weekly", "monthly"} {
		require.NoError(t, quota.ResetExpiredWindow(ctx, f.user.ID, service.PlatformGrok, window, now))
		combined[i] = 0
		assertCombined()
	}
	assertGrokRefusalIntegrationState(t, f, quota, 9.75, 0, 1, 1, 1)
}
