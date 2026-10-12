package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Exercise the actual scheduling entry point, not only the model predicate:
// a mapped sibling must not hide an explicitly enabled passthrough replacement.
func TestOpenAIPassthroughFailover(t *testing.T) {
	for _, advanced := range []string{"true", "false"} {
		for _, scenario := range []string{"excluded", "rate_limited", "runtime_blocked", "concurrency_full", "slot_race", "sticky_busy"} {
			t.Run("advanced="+advanced+"/"+scenario, func(t *testing.T) {
				ctx := context.Background()
				groupID := int64(7)
				accounts := []Account{
					{ID: 1257, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID}, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-6.1-sol": "gpt-6.1-sol"}}},
					{ID: 1258, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, GroupIDs: []int64{groupID}, Extra: map[string]any{"openai_passthrough": true}},
					{ID: 1259, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: -100, GroupIDs: []int64{groupID + 1}, Extra: map[string]any{"openai_passthrough": true}},
				}
				var acquiredIDs, releasedIDs []int64
				concurrencyCache := schedulerTestConcurrencyCache{
					acquireResults: map[int64]bool{1257: false, 1258: true},
					loadMap: map[int64]*AccountLoadInfo{
						1257: {AccountID: 1257, LoadRate: 0},
						1258: {AccountID: 1258, LoadRate: 0},
					},
					waitCounts:  map[int64]int{1257: 999},
					acquiredIDs: &acquiredIDs, releasedIDs: &releasedIDs,
				}
				var excluded map[int64]struct{}
				sessionHash := ""
				cache := &schedulerTestGatewayCache{}
				switch scenario {
				case "excluded":
					excluded = map[int64]struct{}{1257: {}}
				case "rate_limited":
					reset := time.Now().Add(time.Hour)
					accounts[0].RateLimitResetAt = &reset
				case "concurrency_full":
					concurrencyCache.loadMap[1257].LoadRate = 100
					concurrencyCache.loadMap[1257].CurrentConcurrency = 1
				case "sticky_busy":
					sessionHash = "passthrough_failover"
					cache.sessionBindings = map[string]int64{"openai:" + sessionHash: 1257}
				}
				cfg := newSchedulerTestSubscriptionPriorityConfig()
				cfg.Gateway.OpenAIScheduler.StickyEscapeEnabled = true
				cfg.Gateway.Scheduling.StickySessionMaxWaiting = 2
				cfg.Gateway.Scheduling.LoadBatchEnabled = true
				svc := &OpenAIGatewayService{
					accountRepo: schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}},
					cache:       cache, cfg: cfg,
					rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService(advanced, "false"),
					concurrencyService: NewConcurrencyService(concurrencyCache),
				}
				if scenario == "runtime_blocked" {
					for range 2 {
						svc.recordOpenAIAccountModelTransientFailure(&accounts[0], "gpt-6.1-sol", time.Now())
					}
					require.True(t, svc.isOpenAIAccountRequestRuntimeBlocked(&accounts[0], "gpt-6.1-sol", false))
				}
				selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", sessionHash, "gpt-6.1-sol", excluded, OpenAIUpstreamTransportHTTPSSE, false)
				require.NoError(t, err)
				require.NotNil(t, selection)
				require.NotNil(t, selection.Account)
				if !selection.Acquired {
					// The handler rejects a busy WaitPlan without writing a response,
					// excludes that account and reselects with FillScheduling enabled.
					require.Equal(t, int64(1257), selection.Account.ID)
					require.NotNil(t, selection.WaitPlan)
					require.Contains(t, []string{"concurrency_full", "slot_race", "sticky_busy"}, scenario)
					excluded = map[int64]struct{}{1257: {}}
					selection, _, err = svc.SelectAccountWithScheduler(ctx, &groupID, "", sessionHash, "gpt-6.1-sol", excluded, OpenAIUpstreamTransportHTTPSSE, false)
					require.NoError(t, err)
					require.NotNil(t, selection)
					require.NotNil(t, selection.Account)
				}
				require.Equal(t, int64(1258), selection.Account.ID)
				require.True(t, selection.Acquired)
				require.Nil(t, selection.WaitPlan)
				require.Contains(t, acquiredIDs, int64(1258))
				require.NotContains(t, acquiredIDs, int64(1259), "passthrough must not bypass group membership")
				if scenario == "excluded" || scenario == "rate_limited" || scenario == "runtime_blocked" {
					require.NotContains(t, acquiredIDs, int64(1257))
				}
				require.NotNil(t, selection.ReleaseFunc)
				selection.ReleaseFunc()
				require.Equal(t, []int64{1258}, releasedIDs)
			})
		}
	}
}
