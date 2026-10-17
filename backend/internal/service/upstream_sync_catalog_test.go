//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	gocache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/require"
)

func TestGPT55RetirementCatalogCachePreservesEmpty(t *testing.T) {
	groupID := int64(87)
	repo := &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: {{
		ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.5": "gpt-5.5"}},
	}}}}
	svc := &GatewayService{accountRepo: repo, modelsListCache: gocache.New(time.Minute, time.Minute), modelsListCacheTTL: time.Minute}
	for i := 0; i < 2; i++ {
		models := svc.GetAvailableModels(context.Background(), &groupID, PlatformOpenAI)
		require.NotNil(t, models, "filtered catalog must survive cache round trip")
		require.Empty(t, models)
		require.Empty(t, (plazaGatewayModelCatalog{gateway: svc}).GetAvailableModels(context.Background(), &groupID, PlatformOpenAI))
	}
	require.Equal(t, int64(1), repo.listByGroupCalls.Load())
	svc.InvalidateAvailableModelsCache(&groupID, PlatformOpenAI)
	repo.byGroup[groupID] = nil
	for i := 0; i < 2; i++ {
		require.Nil(t, svc.GetAvailableModels(context.Background(), &groupID, PlatformOpenAI), "unknown catalog keeps ordinary fallback semantics")
	}
}

func TestGPT55RetirementCompositeRouteAliases(t *testing.T) {
	groupID := int64(87)
	routes := []CompositeModelRoute{
		{GroupID: groupID, PublicModel: "legacy", MatchType: CompositeRouteMatchExact, TargetPlatform: PlatformOpenAI, UpstreamModel: "gpt-5.5", Enabled: true},
		{GroupID: groupID, PublicModel: "replacement", MatchType: CompositeRouteMatchExact, TargetPlatform: PlatformOpenAI, UpstreamModel: "gpt-6.1-sol", Enabled: true},
		{GroupID: groupID, PublicModel: "native", MatchType: CompositeRouteMatchExact, TargetPlatform: PlatformAnthropic, UpstreamModel: "claude-sonnet-4-6", Enabled: true},
	}
	for _, typ := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
		svc := &GatewayService{accountRepo: &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: {{ID: 1, Platform: PlatformOpenAI, Type: typ}}}}, compositeResolver: NewCompositeRouteResolver(compositeRouteRepoStub{routes: routes})}
		models, err := svc.GetCompositeRouteModels(context.Background(), &groupID, "")
		require.NoError(t, err)
		require.Contains(t, models, "replacement")
		require.Contains(t, models, "native")
		if typ == AccountTypeAPIKey {
			require.Contains(t, models, "legacy")
		} else {
			require.NotContains(t, models, "legacy")
		}
	}
}

func TestGPT55RetirementCompactColdSelection(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		for _, batch := range []bool{false, true} {
			for _, passthrough := range []bool{false, true} {
				resetOpenAIAdvancedSchedulerSettingCacheForTest()
				account := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"compact_model_mapping": map[string]any{"gpt-5.5": "gpt-6.1-sol"}}, Extra: map[string]any{"openai_passthrough": passthrough}}
				svc := newOpenAICompactionSchedulerTestService([]Account{account}, advanced)
				svc.cfg.Gateway.Scheduling.LoadBatchEnabled = batch
				groupID := int64(87)
				selection, _, err := svc.SelectAccountWithScheduler(context.Background(), &groupID, "", "", "gpt-5.5", nil, OpenAIUpstreamTransportAny, true)
				require.NoError(t, err, "advanced=%v batch=%v passthrough=%v", advanced, batch, passthrough)
				require.NotNil(t, selection)
				require.Equal(t, account.ID, selection.Account.ID)
				if selection.ReleaseFunc != nil {
					selection.ReleaseFunc()
				}
				selection, _, err = svc.SelectAccountWithScheduler(context.Background(), &groupID, "", "", "gpt-5.5", nil, OpenAIUpstreamTransportAny, false)
				require.Error(t, err)
				require.Nil(t, selection)
			}
		}
	}
}

func TestGPT55RetirementSnapshotDiagnosis(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
	snapshot := NewSchedulerSnapshotService(&openAISnapshotCacheStub{snapshotAccounts: []*Account{account}}, nil, nil, nil, &config.Config{})
	groupID := int64(87)
	diag := diagnoseModelAvailabilityFromSchedulerSnapshot(snapshot, context.Background(), &groupID, "gpt-5.5", PlatformOpenAI)
	require.True(t, diag.HasAccountsInPool)
	require.False(t, diag.HasModelSupport)
	require.True(t, diag.HasRetiredSubscriptionModel)
}

func TestGPT55RetirementCompactFallbackSelection(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		for _, wait := range []bool{false, true} {
			t.Run(fmt.Sprintf("advanced=%v/wait=%v", advanced, wait), func(t *testing.T) {
				resetOpenAIAdvancedSchedulerSettingCacheForTest()
				t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
				account := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"compact_model_mapping": map[string]any{"gpt-5.5": "gpt-6.1-sol"}}}
				svc := newOpenAICompactionSchedulerTestService([]Account{account}, advanced)
				svc.cfg.Gateway.Scheduling.LoadBatchEnabled = true
				cache := schedulerTestConcurrencyCache{loadBatchErr: errors.New("load query unavailable")}
				if wait {
					cache = schedulerTestConcurrencyCache{acquireResults: map[int64]bool{account.ID: false}}
				}
				svc.concurrencyService = NewConcurrencyService(cache)
				groupID := int64(87)
				selection, _, err := svc.SelectAccountWithScheduler(context.Background(), &groupID, "", "", "gpt-5.5", nil, OpenAIUpstreamTransportAny, true)
				require.NoError(t, err)
				require.NotNil(t, selection)
				require.Equal(t, account.ID, selection.Account.ID)
				if wait {
					require.False(t, selection.Acquired)
					require.NotNil(t, selection.WaitPlan)
				} else {
					require.True(t, selection.Acquired)
				}
				if selection.ReleaseFunc != nil {
					selection.ReleaseFunc()
				}
			})
		}
	}
}
