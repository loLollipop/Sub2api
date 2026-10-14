package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestScopeSelectionCapabilityPlaceholder(t *testing.T) {
	for _, scope := range []string{ModelScopeOpenAI, ModelScopeOther, ModelScopeCN, "invalid"} {
		t.Run(scope, func(t *testing.T) {
			gid := int64(1)
			group := &Group{ID: gid, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true}
			key := &APIKey{ModelScope: scope, GroupID: &gid, Group: group, User: &User{ID: 1}}
			routes := newAPIKeyRouteIterator(context.Background(), key, []int64{gid}, "",
				func(context.Context, *APIKey, int64) (*APIKey, error) { return key, nil }, nil,
				func(context.Context, *int64) groupModelsCatalog { return groupModelsCatalog{} },
				func(context.Context, *int64, string) (ChannelMappingResult, bool) {
					return ChannelMappingResult{}, false
				}, nil)
			require.Equal(t, scope != "invalid", routes.loadNext())
			if scope == "invalid" {
				require.ErrorIs(t, routes.err, ErrModelScopeDenied)
			} else {
				require.NoError(t, routes.err)
				require.Empty(t, routes.candidates[0].model)
			}
		})
	}
}

func TestScopeSelectionMixedChoosesSafeMappedAccount(t *testing.T) {
	accounts := []Account{
		{ID: 1, Platform: PlatformMixed, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Priority: 1, Credentials: map[string]any{"model_mapping": map[string]any{"qwen3": "gpt-5"}}},
		{ID: 2, Platform: PlatformMixed, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Priority: 2, Credentials: map[string]any{"model_mapping": map[string]any{"qwen3": "qwen3"}}},
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{RunMode: config.RunModeSimple}}
	ctx := ContextWithModelScope(context.Background(), ModelScopeCN)
	selected, _, _ := svc.selectBestAccount(ctx, nil, PlatformMixed, accounts, "qwen3", nil, false, "", false)
	require.NotNil(t, selected)
	require.Equal(t, int64(2), selected.ID)
	scheduler := &defaultOpenAIAccountScheduler{service: svc}
	req := OpenAIAccountScheduleRequest{Platform: PlatformMixed, RequestedModel: "qwen3"}
	allowed, reason := scheduler.isAccountRequestCompatibleReason(ctx, &accounts[0], req)
	require.False(t, allowed)
	require.Equal(t, "model_scope_denied", reason)
	require.True(t, scheduler.isAccountRequestCompatible(ctx, &accounts[1], req))
	require.Nil(t, svc.recheckSelectedOpenAIAccountFromDBBeforeProfit(ctx, &accounts[0], nil, PlatformMixed, "qwen3", false, ""))
	selected, _, _ = svc.selectBestAccount(context.Background(), nil, PlatformMixed, accounts, "qwen3", nil, false, "", false)
	require.NotNil(t, selected)
	require.Equal(t, int64(1), selected.ID, "legacy all keeps original account priority")
}

func TestScopeSelectionUsesCompactAndPassthroughMapping(t *testing.T) {
	ctx := ContextWithModelScope(context.Background(), ModelScopeOpenAI)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Extra: map[string]any{"openai_passthrough": true},
		Credentials: map[string]any{
			"model_mapping":         map[string]any{"gpt-5": "qwen3"},
			"compact_model_mapping": map[string]any{"gpt-5": "qwen3"},
		}}
	require.True(t, openAIAccountAllowsModelScope(ctx, account, "gpt-5", false), "passthrough ignores ordinary model mapping")
	require.False(t, openAIAccountAllowsModelScope(ctx, account, "gpt-5", true), "compact passthrough still applies compact mapping")
	account.Extra["openai_passthrough"] = false
	require.False(t, openAIAccountAllowsModelScope(ctx, account, "gpt-5", false))
	account.Credentials["compact_model_mapping"] = map[string]any{"gpt-5": "gpt-5"}
	require.True(t, openAIAccountAllowsModelScope(ctx, account, "gpt-5", true), "exact compact mapping precedes normal mapping")
}

type scopeNativeSelectionRepo struct {
	AccountRepository
	accounts []Account
}

func (r *scopeNativeSelectionRepo) ListSchedulableByPlatforms(context.Context, []string) ([]Account, error) {
	return r.accounts, nil
}

func TestScopeSelectionNativeAnthropicChoosesSafeMappedAccount(t *testing.T) {
	accounts := []Account{
		{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Priority: 1,
			Credentials: map[string]any{"model_mapping": map[string]any{"claude-sonnet-4-5": "qwen3"},
				"compact_model_mapping": map[string]any{"claude-sonnet-4-5": "claude-sonnet-4-5"}}},
		{ID: 2, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Priority: 2,
			Credentials: map[string]any{"model_mapping": map[string]any{"claude-sonnet-4-5": "claude-sonnet-4-5"},
				"compact_model_mapping": map[string]any{"claude-sonnet-4-5": "qwen3"}}},
	}
	svc := &GatewayService{accountRepo: &scopeNativeSelectionRepo{accounts: accounts}, cfg: &config.Config{RunMode: config.RunModeSimple}}
	for _, mode := range []string{"native", "mixed"} {
		t.Run(mode, func(t *testing.T) {
			selectAccount := func(ctx context.Context) (*Account, error) {
				if mode == "mixed" {
					return svc.selectAccountWithMixedScheduling(ctx, nil, "", "claude-sonnet-4-5", nil, PlatformAnthropic)
				}
				return svc.selectAccountForModelWithPlatform(ctx, nil, "", "claude-sonnet-4-5", nil, PlatformAnthropic)
			}
			selected, err := selectAccount(ContextWithModelScope(context.Background(), ModelScopeAnthropic))
			require.NoError(t, err)
			require.NotNil(t, selected)
			require.Equal(t, int64(2), selected.ID, "an invalid priority-1 account must not block the legal candidate")
			selected, err = selectAccount(context.Background())
			require.NoError(t, err)
			require.Equal(t, int64(1), selected.ID, "legacy all preserves existing priority")
		})
	}
}

func TestScopeSelectionNativeUsesOrdinaryProtocolMapping(t *testing.T) {
	for _, tc := range []struct {
		name, scope, platform, accountType, model, upstream string
		passthrough, allowed                                bool
	}{
		{"anthropic API-key mapped across scope", ModelScopeAnthropic, PlatformAnthropic, AccountTypeAPIKey, "claude-sonnet-4-5", "qwen3", false, false},
		{"anthropic OAuth ignores account mapping", ModelScopeAnthropic, PlatformAnthropic, AccountTypeOAuth, "claude-sonnet-4-5", "qwen3", false, true},
		{"anthropic Vertex applies explicit mapping", ModelScopeAnthropic, PlatformAnthropic, AccountTypeServiceAccount, "claude-sonnet-4-5", "qwen3", false, false},
		{"anthropic Bedrock applies ordinary mapping", ModelScopeAnthropic, PlatformAnthropic, AccountTypeBedrock, "claude-sonnet-4-5", "qwen3", false, false},
		{"antigravity applies ordinary mapping", ModelScopeAnthropic, PlatformAntigravity, AccountTypeOAuth, "claude-sonnet-4-5", "qwen3", false, false},
		{"openai passthrough ignores ordinary mapping", ModelScopeOpenAI, PlatformOpenAI, AccountTypeOAuth, "gpt-5", "qwen3", true, true},
		{"openai ordinary mapping is checked", ModelScopeOpenAI, PlatformOpenAI, AccountTypeOAuth, "gpt-5", "qwen3", false, false},
		{"gemini ordinary protocol ignores compact mapping", ModelScopeOther, PlatformGemini, AccountTypeAPIKey, "gemini-2.5-pro", "gemini-2.5-pro", false, true},
		{"mixed ordinary protocol ignores compact mapping", ModelScopeCN, PlatformMixed, AccountTypeAPIKey, "qwen3", "qwen3", false, true},
		{"requested model is checked before safe mapping", ModelScopeAnthropic, PlatformAnthropic, AccountTypeAPIKey, "qwen3", "claude-sonnet-4-5", false, false},
		{"unknown scope fails closed", "invalid", PlatformAnthropic, AccountTypeAPIKey, "claude-sonnet-4-5", "claude-sonnet-4-5", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := &Account{Platform: tc.platform, Type: tc.accountType, Status: StatusActive, Schedulable: true,
				Credentials: map[string]any{"model_mapping": map[string]any{tc.model: tc.upstream},
					"compact_model_mapping": map[string]any{tc.model: "gpt-5"}},
				Extra: map[string]any{"openai_passthrough": tc.passthrough}}
			ctx := ContextWithModelScope(context.Background(), tc.scope)
			require.Equal(t, tc.allowed, (&GatewayService{}).isAccountSchedulableForModelSelection(ctx, account, tc.model))
		})
	}
}

func TestScopeSelectionLiveActualModelAndDefault(t *testing.T) {
	for _, tc := range []struct {
		scope, session string
		allowed        bool
	}{
		{ModelScopeOpenAI, `{}`, true},
		{ModelScopeCN, `{}`, false},
		{ModelScopeOpenAI, `{"model":"gpt-live"}`, true},
		{ModelScopeOpenAI, `{"model":"qwen3"}`, false},
		{ModelScopeOpenAI, `{"model":"gpt-live","MODEL":"qwen3"}`, false},
	} {
		ctx := ContextWithModelScope(context.Background(), tc.scope)
		err := checkLiveCallModelScope(ctx, &LiveCallRequest{Session: json.RawMessage(tc.session)})
		if tc.allowed {
			require.NoError(t, err)
		} else {
			require.ErrorIs(t, err, ErrModelScopeDenied)
		}
	}
	svc := &OpenAIGatewayService{}
	require.False(t, svc.shouldFailoverLiveCreateError(nil, fmt.Errorf("wrapped: %w", ErrModelScopeDenied)))
}

func TestScopeSelectionLocalDenialPreservesAffinityAndScheduling(t *testing.T) {
	ctx := withUpstreamAttempts(context.Background())
	cache := &failedAffinityCacheStub{}
	account := &Account{ID: 1}
	err := fmt.Errorf("wrapped: %w", ErrModelScopeDenied)
	invalidateUpstreamAffinity(ctx, cache, nil, []string{"session"}, account, err)
	require.Empty(t, cache.invalidated)
	state, ok := ctx.Value(upstreamAttemptContextKey{}).(*upstreamAttempts)
	require.True(t, ok)
	require.Empty(t, state.failed)
	// A nil service would panic if the policy error reached any scheduler or
	// account health operation; the refusal must return before those hooks.
	var svc *OpenAIGatewayService
	require.False(t, svc.ReportOpenAIAccountScheduleResult(account, "qwen3", false, nil, err))
	require.False(t, svc.ObserveOpenAIAccountHealthFailure(ctx, account, err))
}

func TestScopeSelectionMixedDenialDoesNotStartProtocolFallback(t *testing.T) {
	account := &Account{Platform: PlatformMixed, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"model_mapping": map[string]any{"qwen3": "gpt-5"}}}
	ctx := ContextWithModelScope(context.Background(), ModelScopeCN)
	called := false
	_, err := (&OpenAIGatewayService{}).forwardWithProtocolFallback(ctx, &gin.Context{}, account,
		[]byte(`{"model":"qwen3"}`), APIProtocolResponses, "", "",
		func(context.Context) (*OpenAIForwardResult, error) { called = true; return nil, nil })
	require.ErrorIs(t, err, ErrModelScopeDenied)
	require.False(t, called)
}
