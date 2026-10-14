//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestMixedProviderRateValidationAndIdentity(t *testing.T) {
	for _, body := range []string{`{"kimi":null}`, `{"kimi":"0.5"}`, `{"typo":1}`, `{"kimi":-1}`} {
		var rates MixedProviderRateMultipliers
		require.Error(t, json.Unmarshal([]byte(body), &rates))
	}
	for model, vendor := range map[string]string{
		"deepseek-ai/deepseek-v4-pro": "deepseek", "moonshotai/kimi-k2.5": "kimi", "K3": "kimi",
		"z-ai/glm-5.3": "zhipu", "Qwen/Qwen3.6": "qwen", "MiniMaxAI/MiniMax-M2.5": "minimax",
		"xiaomimimo/mimo-v2-pro": "mimo", "stepfun-ai/step-3.5-flash": "stepfun",
		"openai/gpt-5.5": "openai", "us.anthropic.claude-sonnet-4": "anthropic",
		"google/gemini-3-pro": "gemini", "x-ai/grok-4": "grok", "untrusted/kimi-k2": "other",
		"openai/glm-5": "other", "gpt-unknown/claude-sonnet-4": "other", "gemini-fake/anything": "other", "google/gemini-3/fake": "other",
		"chatglm_turbo": "zhipu", "zhipuai/cogview-3": "zhipu", "thudm/cogvideo": "zhipu", "doubao-embedding-vision": "doubao",
		"01-ai/yi-large": "yi", "baidu/ernie-4.0": "baidu", "spark-pro": "spark", "tencent/hunyuan-code": "hunyuan", "k2-thinking": "kimi", "k1.5": "kimi", "m2.7": "minimax",
	} {
		require.Equal(t, vendor, ModelVendor(model), model)
	}
	for _, rate := range []float64{-1, math.NaN(), math.Inf(1), 1000000} {
		require.Error(t, ValidateMixedProviderRateMultipliers(map[string]float64{"kimi": rate}))
	}
	require.Error(t, ValidateMixedProviderRateMultipliers(map[string]float64{"unknown": 1}))
	require.NoError(t, ValidateMixedProviderRateMultipliers(map[string]float64{"kimi": 0}))
	group := &Group{Platform: PlatformMixed, RateMultiplier: 3, MixedProviderRateMultipliers: map[string]float64{"kimi": 0}}
	require.Equal(t, 0.0, group.RateMultiplierForModel("kimi-k2"))
	require.Equal(t, 1.0, group.RateMultiplierForModel("glm-5"))
	group.Platform = PlatformOpenAI
	require.Equal(t, 3.0, group.RateMultiplierForModel("kimi-k2"))
}

func TestMixedProviderRateOverrideCache(t *testing.T) {
	ctx := context.Background()
	for _, override := range []*float64{nil, ptrFloat(0), ptrFloat(2)} {
		t.Run(fmt.Sprint(override), func(t *testing.T) {
			repo := &userGroupRateRepoHotpathStub{rate: override}
			r := newUserGroupRateResolver(repo, nil, time.Minute, nil, "test")
			const n = 32
			var wg sync.WaitGroup
			got := make([]float64, n)
			for i := range got {
				wg.Add(1)
				go func(i int) { defer wg.Done(); got[i] = r.Resolve(ctx, 1, 2, float64(i+1)) }(i)
			}
			wg.Wait()
			for i, value := range got {
				expected := float64(i + 1)
				if override != nil {
					expected = *override
				}
				require.Equal(t, expected, value)
				require.Equal(t, expected, r.Resolve(ctx, 1, 2, float64(i+1)))
				require.Equal(t, expected, r.ResolveSnapshotOnly(1, 2, float64(i+1)))
			}
			require.Equal(t, int64(1), repo.calls.Load())
			r.cache.Delete("1:2")
			require.Equal(t, 99.0, r.ResolveSnapshotOnly(1, 2, 99))
			require.Equal(t, int64(1), repo.calls.Load(), "snapshot must not query repository")
			r.Resolve(ctx, 1, 2, 99)
			require.Equal(t, int64(2), repo.calls.Load())
		})
	}
	repo := &userGroupRateRepoHotpathStub{err: errors.New("offline")}
	r := newUserGroupRateResolver(repo, nil, time.Minute, nil, "test")
	r.cache.Set("1:2", 77.0, time.Minute) // legacy resolved values have unknown override provenance
	require.Equal(t, 2.0, r.ResolveSnapshotOnly(1, 2, 2))
	require.Equal(t, 2.0, r.Resolve(ctx, 1, 2, 2))
	require.Equal(t, 3.0, r.Resolve(ctx, 1, 2, 3))
}

func TestMixedProviderRateAdminStorageSemantics(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}
	input := &CreateGroupInput{Name: "Mixed", Platform: PlatformMixed, RateMultiplier: 2, MixedProviderRateMultipliers: map[string]float64{"kimi": 0, "deepseek": 0.5}}
	created, err := svc.CreateGroup(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 0.5, created.MixedProviderRateMultipliers["deepseek"])
	input.MixedProviderRateMultipliers["deepseek"] = 7
	require.Equal(t, 0.5, created.MixedProviderRateMultipliers["deepseek"])
	repo.getByID = created
	for _, body := range []string{`{}`, `{"mixed_provider_rate_multipliers":null}`} {
		var update UpdateGroupInput
		// Service input uses Go names. Handler JSON map omission/null both become nil.
		var wire struct {
			Rates map[string]float64 `json:"mixed_provider_rate_multipliers"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &wire))
		update.MixedProviderRateMultipliers = wire.Rates
		got, err := svc.UpdateGroup(context.Background(), 1, &update)
		require.NoError(t, err)
		require.Equal(t, 0.5, got.MixedProviderRateMultipliers["deepseek"])
	}
	got, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{MixedProviderRateMultipliers: map[string]float64{}})
	require.NoError(t, err)
	require.Empty(t, got.MixedProviderRateMultipliers)
	require.Equal(t, 1.0, got.RateMultiplierForModel("deepseek-v4-pro"), "clear returns to standard rate, never free")
	_, err = svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{MixedProviderRateMultipliers: map[string]float64{"typo": 1}})
	require.Error(t, err)
	svc.cfg = &config.Config{RunMode: config.RunModeSimple}
	_, err = svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{MixedProviderRateMultipliers: map[string]float64{"kimi": 10}})
	require.NoError(t, err)
	require.Empty(t, repo.updated.MixedProviderRateMultipliers)
}

func TestMixedProviderRateSnapshotsAndClones(t *testing.T) {
	groupID := int64(2)
	group := &Group{ID: groupID, Platform: PlatformMixed, Status: StatusActive, RateMultiplier: 2, MixedProviderRateMultipliers: map[string]float64{"kimi": 0, "deepseek": 0.4}}
	key := &APIKey{ID: 3, UserID: 1, GroupID: &groupID, Group: group, User: &User{ID: 1, Status: StatusActive}, Status: StatusActive}
	svc := &APIKeyService{}
	snapshot := svc.snapshotFromAPIKey(context.Background(), key)
	data, err := json.Marshal(snapshot)
	require.NoError(t, err)
	var restored APIKeyAuthSnapshot
	require.NoError(t, json.Unmarshal(data, &restored))
	materialized, used, err := svc.applyAuthCacheEntry("k", &APIKeyAuthCacheEntry{Snapshot: &restored})
	require.NoError(t, err)
	require.True(t, used)
	require.Equal(t, group.MixedProviderRateMultipliers, materialized.Group.MixedProviderRateMultipliers)
	for _, clone := range []*Group{cloneSchedulerGroup(group), cloneGroupForDuplicate(group, "op"), materialized.Group} {
		clone.MixedProviderRateMultipliers["deepseek"] = 9
		require.Equal(t, 0.4, group.MixedProviderRateMultipliers["deepseek"])
	}
	snapshot.Group.MixedProviderRateMultipliers["kimi"] = 8
	require.Equal(t, 0.0, group.MixedProviderRateMultipliers["kimi"])
	_, used, err = svc.applyAuthCacheEntry("k", &APIKeyAuthCacheEntry{Snapshot: &APIKeyAuthSnapshot{Version: 26}})
	require.NoError(t, err)
	require.False(t, used)
}

func TestMixedProviderRateCatalogue(t *testing.T) {
	repo := &modelScopeAccountRepo{accounts: []Account{{Platform: PlatformMixed, Credentials: map[string]any{"model_mapping": map[string]any{
		"kimi-k2": "deepseek-v4-pro", "gpt-5.5": "gpt-5.5", "gemini-3-pro": "gemini-3-pro", "blocked": "glm-5",
	}}}}}
	svc := &GatewayService{accountRepo: repo}
	group := &Group{ID: 10, Platform: PlatformMixed}
	require.Equal(t, []string{"openai", "cn", "other"}, svc.SupportedKeyProviders(context.Background(), group))
	require.Equal(t, []string{"deepseek", "zhipu", "openai", "gemini"}, group.SupportedModelVendors)
	require.Equal(t, 1, repo.calls)
}

func TestMixedProviderRateCatalogueAllScopeCrossCategoryAlias(t *testing.T) {
	repo := &modelScopeAccountRepo{accounts: []Account{{Platform: PlatformMixed, Credentials: map[string]any{"model_mapping": map[string]any{
		"gpt-5.1": "deepseek-v4-flash",
	}}}}}
	svc := &GatewayService{accountRepo: repo}
	group := &Group{ID: 10, Platform: PlatformMixed}
	require.Empty(t, svc.SupportedKeyProviders(context.Background(), group), "restricted categories must still reject cross-category mappings")
	require.Equal(t, []string{"deepseek"}, group.SupportedModelVendors, "all-scope keys still use the target vendor rate")
	require.Equal(t, 1, repo.calls, "vendor projection must share the original catalogue read")
}

func TestMixedProviderRatePreauthorization(t *testing.T) {
	groupID := int64(2)
	key := &APIKey{UserID: 1, GroupID: &groupID, Group: &Group{Platform: PlatformMixed, RateMultiplier: 7, MixedProviderRateMultipliers: map[string]float64{"kimi": 0, "deepseek": 0.4}}}
	r := newUserGroupRateResolver(&userGroupRateRepoHotpathStub{}, nil, time.Minute, nil, "test")
	gateway := &GatewayService{userGroupRateResolver: r}
	openai := &OpenAIGatewayService{userGroupRateResolver: r}
	for model, rate := range map[string]float64{"kimi-k2": 0, "deepseek-v4-pro": 0.4, "gpt-5.5": 1} {
		require.Equal(t, rate, gateway.BalancePreauthorizationCostInput(context.Background(), key, model, time.Now(), "").RateMultiplier)
		require.Equal(t, rate, openai.BalancePreauthorizationCostInput(context.Background(), key, model, time.Now(), "").RateMultiplier)
	}
}

func mixedRateTestPricing() map[string]*ModelPricing {
	return map[string]*ModelPricing{
		"kimi-k2":         {InputPricePerToken: 1e-6, OutputPricePerToken: 2e-6},
		"glm-5":           {InputPricePerToken: 2e-6, OutputPricePerToken: 4e-6},
		"gpt-5.4":         {InputPricePerToken: 3e-6, OutputPricePerToken: 6e-6},
		"claude-sonnet-4": {InputPricePerToken: 4e-6, OutputPricePerToken: 8e-6},
	}
}

func TestMixedProviderRateBothBillingPaths(t *testing.T) {
	for _, gateway := range []string{"openai", "anthropic"} {
		for _, source := range []string{BillingModelSourceUpstream, BillingModelSourceRequested, BillingModelSourceChannelMapped, "fallback"} {
			for _, override := range []*float64{nil, ptrFloat(0), ptrFloat(9)} {
				t.Run(fmt.Sprintf("%s/%s/%v", gateway, source, override), func(t *testing.T) {
					ctx := context.Background()
					logs := &openAIRecordUsageLogRepoStub{inserted: true}
					users := &openAIRecordUsageUserRepoStub{}
					id := int64(2)
					group := &Group{ID: id, Platform: PlatformMixed, RateMultiplier: 9, MixedProviderRateMultipliers: map[string]float64{"kimi": 0.2, "zhipu": 0.8, "openai": 2}}
					key := &APIKey{ID: 3, UserID: 1, User: &User{ID: 1}, GroupID: &id, Group: group}
					fields := ChannelUsageFields{OriginalModel: "kimi-k2", ChannelMappedModel: "gpt-5.4", BillingModelSource: source}
					model := "glm-5"
					switch source {
					case BillingModelSourceRequested:
						model = fields.OriginalModel
					case BillingModelSourceChannelMapped:
						model = fields.ChannelMappedModel
					case "fallback":
						fields.BillingModelSource = BillingModelSourceRequested
						fields.OriginalModel = "unknown-rate-test"
						fields.ChannelMappedModel = ""
					}
					rate := group.RateMultiplierForModel(model)
					if override != nil {
						rate = *override
					}
					r := newUserGroupRateResolver(&userGroupRateRepoHotpathStub{rate: override}, nil, time.Minute, nil, "test")
					account := &Account{ID: 4, Platform: PlatformMixed, Type: AccountTypeAPIKey}
					var billing *BillingService
					if gateway == "openai" {
						svc := newOpenAIRecordUsageServiceForTest(logs, users, &openAIRecordUsageSubRepoStub{}, nil)
						svc.userGroupRateResolver = r
						svc.billingService.fallbackPrices = mixedRateTestPricing()
						billing = svc.billingService
						require.NoError(t, svc.RecordUsage(ctx, &OpenAIRecordUsageInput{
							APIKey: key, User: key.User, Account: account, ChannelUsageFields: fields, InboundEndpoint: "/v1/messages",
							Result: &OpenAIForwardResult{RequestID: "mixed-rate", Model: fields.OriginalModel, UpstreamModel: "glm-5", Usage: OpenAIUsage{InputTokens: 100, OutputTokens: 50}},
						}))
					} else {
						svc := newGatewayRecordUsageServiceForTest(logs, users, &openAIRecordUsageSubRepoStub{})
						svc.userGroupRateResolver = r
						svc.billingService.fallbackPrices = mixedRateTestPricing()
						billing = svc.billingService
						require.NoError(t, svc.RecordUsage(ctx, &RecordUsageInput{
							APIKey: key, User: key.User, Account: account, ChannelUsageFields: fields,
							// The original Messages forwarder puts its concrete billing model in Model.
							Result: &ForwardResult{RequestID: "mixed-rate", Model: "glm-5", UpstreamModel: "glm-5", Usage: ClaudeUsage{InputTokens: 100, OutputTokens: 50}},
						}))
					}
					expected, err := billing.CalculateCost(model, UsageTokens{InputTokens: 100, OutputTokens: 50}, rate)
					require.NoError(t, err)
					require.NotNil(t, logs.lastLog)
					require.InDelta(t, expected.TotalCost, logs.lastLog.TotalCost, 1e-12)
					require.InDelta(t, expected.ActualCost, logs.lastLog.ActualCost, 1e-12)
					require.InDelta(t, expected.ActualCost, users.lastAmount, 1e-12)
					require.Equal(t, rate, logs.lastLog.RateMultiplier)
				})
			}
		}
	}
}

func TestMixedProviderRateResponseModelGuards(t *testing.T) {
	for _, gateway := range []string{"openai", "anthropic"} {
		for _, responseRate := range []float64{0, 0.25, 8} {
			t.Run(fmt.Sprintf("%s/%g", gateway, responseRate), func(t *testing.T) {
				logs := &openAIRecordUsageLogRepoStub{inserted: true}
				users := &openAIRecordUsageUserRepoStub{}
				id := int64(2)
				group := &Group{ID: id, Platform: PlatformMixed, RateMultiplier: 9, MixedProviderRateMultipliers: map[string]float64{"kimi": responseRate, "zhipu": 0.5}}
				key := &APIKey{ID: 3, UserID: 1, User: &User{ID: 1}, GroupID: &id, Group: group}
				fields := ChannelUsageFields{OriginalModel: "glm-5", BillingModelSource: BillingModelSourceResponse}
				account := &Account{ID: 4, Platform: PlatformMixed, Type: AccountTypeAPIKey}
				var billing *BillingService
				if gateway == "openai" {
					svc := newOpenAIRecordUsageServiceForTest(logs, users, &openAIRecordUsageSubRepoStub{}, nil)
					svc.billingService.fallbackPrices = mixedRateTestPricing()
					billing = svc.billingService
					require.NoError(t, svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{APIKey: key, User: key.User, Account: account, ChannelUsageFields: fields,
						Result: &OpenAIForwardResult{RequestID: "response-rate", Model: "glm-5", UpstreamResponseModel: "kimi-k2", Usage: OpenAIUsage{InputTokens: 100, OutputTokens: 50}},
					}))
				} else {
					svc := newGatewayRecordUsageServiceForTest(logs, users, &openAIRecordUsageSubRepoStub{})
					svc.billingService.fallbackPrices = mixedRateTestPricing()
					billing = svc.billingService
					require.NoError(t, svc.RecordUsage(context.Background(), &RecordUsageInput{APIKey: key, User: key.User, Account: account, ChannelUsageFields: fields,
						Result: &ForwardResult{RequestID: "response-rate", Model: "glm-5", UpstreamResponseModel: "kimi-k2", Usage: ClaudeUsage{InputTokens: 100, OutputTokens: 50}},
					}))
				}
				model, rate := "glm-5", 0.5
				if responseRate == 0.25 {
					model, rate = "kimi-k2", responseRate
				}
				expected, err := billing.CalculateCost(model, UsageTokens{InputTokens: 100, OutputTokens: 50}, rate)
				require.NoError(t, err)
				require.InDelta(t, expected.TotalCost, logs.lastLog.TotalCost, 1e-12)
				require.InDelta(t, expected.ActualCost, logs.lastLog.ActualCost, 1e-12)
				require.Equal(t, rate, logs.lastLog.RateMultiplier)
			})
		}
	}
}
