package service

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func nativeResponsesSchedulingTestAccount(id int64, priority int, native bool) Account {
	account := Account{
		ID: id, Priority: priority, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		GroupIDs: []int64{1},
	}
	if !native {
		account.Extra = map[string]any{
			openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
		}
	}
	return account
}

func nativeResponsesSchedulingTestService(t *testing.T, accounts []Account, advanced, batch bool, concurrencyCache schedulerTestConcurrencyCache) *OpenAIGatewayService {
	t.Helper()
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = batch
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
	svc := &OpenAIGatewayService{
		accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:       &schedulerTestGatewayCache{}, cfg: cfg,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}
	if advanced {
		svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("true")
	}
	return svc
}

func selectNativeResponsesSchedulingTestAccount(t *testing.T, svc *OpenAIGatewayService, ctx context.Context, platform, session string, excluded map[int64]struct{}, compact bool) *AccountSelectionResult {
	t.Helper()
	groupID := int64(1)
	selection, _, err := svc.SelectAccountWithSchedulerForCapability(
		ctx, &groupID, "", session, "gpt-test", excluded,
		OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, compact, false, false, platform,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	if selection.ReleaseFunc != nil {
		t.Cleanup(selection.ReleaseFunc)
	}
	return selection
}

func TestNativeResponsesLegacySelectionMappingAndCompactOrder(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		preferNative, compact      bool
		chatCompact, nativeCompact bool
		wantID                     int64
	}{
		{name: "unmarked_keeps_mapping_first", wantID: 1},
		{name: "native_precedes_mapping", preferNative: true, wantID: 2},
		{name: "compact_precedes_native_and_mapping", preferNative: true, compact: true, chatCompact: true, wantID: 1},
		{name: "supported_native_precedes_mapped_unknown", preferNative: true, compact: true, nativeCompact: true, wantID: 2},
		{name: "unmarked_compact_keeps_mapping_first", compact: true, nativeCompact: true, wantID: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chat := nativeResponsesSchedulingTestAccount(1, 0, false)
			chat.Credentials = map[string]any{"model_mapping": map[string]any{"gpt-test": "gpt-upstream"}}
			native := nativeResponsesSchedulingTestAccount(2, 10, true)
			if tc.chatCompact {
				chat.Extra["openai_compact_supported"] = true
			}
			if tc.nativeCompact {
				native.Extra = map[string]any{"openai_compact_supported": true}
			}
			svc := nativeResponsesSchedulingTestService(t, []Account{chat, native}, false, false, schedulerTestConcurrencyCache{})
			ctx := context.Background()
			if tc.preferNative {
				ctx = WithOpenAIPreferNativeResponses(ctx)
			}
			selection := selectNativeResponsesSchedulingTestAccount(t, svc, ctx, PlatformOpenAI, "", nil, tc.compact)
			require.Equal(t, tc.wantID, selection.Account.ID)
		})
	}
}

func TestNativeResponsesSelectionDoesNotPromoteAnthropic(t *testing.T) {
	for _, tc := range []struct {
		name            string
		advanced, batch bool
		cache           schedulerTestConcurrencyCache
	}{
		{name: "legacy_no_batch"},
		{name: "legacy_batch", batch: true},
		{name: "legacy_batch_error", batch: true, cache: schedulerTestConcurrencyCache{loadBatchErr: errors.New("load unavailable")}},
		{name: "legacy_wait", batch: true, cache: schedulerTestConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 100}, 2: {AccountID: 2, LoadRate: 100},
		}}},
		{name: "advanced", advanced: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chat := nativeResponsesSchedulingTestAccount(1, 0, false)
			chat.Platform = PlatformDeepseek
			chat.Credentials = map[string]any{"api_protocol": APIProtocolChatCompletions, "model_mapping": map[string]any{"gpt-test": "deepseek-chat"}}
			anthropic := nativeResponsesSchedulingTestAccount(2, 10, true)
			anthropic.Platform = PlatformDeepseek
			anthropic.Credentials = map[string]any{"api_protocol": APIProtocolAnthropic, "model_mapping": map[string]any{"gpt-test": "deepseek-chat"}}
			svc := nativeResponsesSchedulingTestService(t, []Account{chat, anthropic}, tc.advanced, tc.batch, tc.cache)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			c.Set("api_key", &APIKey{Group: &Group{ID: 1, Platform: PlatformDeepseek, CrossProtocolConversionEnabled: false}})
			require.False(t, crossProtocolConversionAllowedFromContext(c))
			selection := selectNativeResponsesSchedulingTestAccount(t, svc, WithOpenAIPreferNativeResponses(c), PlatformDeepseek, "", nil, false)
			require.Equal(t, chat.ID, selection.Account.ID, "preference must not promote an account that needs an Anthropic bridge")
			if tc.name == "legacy_wait" {
				require.False(t, selection.Acquired)
				require.NotNil(t, selection.WaitPlan)
			}
		})
	}
}

func TestNativeResponsesAdvancedFillRespectsPriorityBeforeNative(t *testing.T) {
	for _, tc := range []struct {
		name         string
		accounts     []Account
		rejected     map[int64]bool
		wantAcquired []int64
	}{
		{name: "converter_in_higher_priority_layer", accounts: []Account{
			nativeResponsesSchedulingTestAccount(1, 0, false), nativeResponsesSchedulingTestAccount(2, 1, true),
		}, wantAcquired: []int64{1}},
		{name: "native_first_within_layer", accounts: []Account{
			nativeResponsesSchedulingTestAccount(1, 0, false), nativeResponsesSchedulingTestAccount(2, 0, true),
		}, wantAcquired: []int64{2}},
		{name: "exhaust_layer_before_next_native", accounts: []Account{
			nativeResponsesSchedulingTestAccount(1, 0, true), nativeResponsesSchedulingTestAccount(2, 0, false),
			nativeResponsesSchedulingTestAccount(3, 1, true), nativeResponsesSchedulingTestAccount(4, 1, false),
		}, rejected: map[int64]bool{1: false, 2: false}, wantAcquired: []int64{1, 2, 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			accounts := append([]Account{nativeResponsesSchedulingTestAccount(100, -1, true)}, tc.accounts...)
			var acquired []int64
			svc := nativeResponsesSchedulingTestService(t, accounts, true, false, schedulerTestConcurrencyCache{acquireResults: tc.rejected, acquiredIDs: &acquired})
			selection := selectNativeResponsesSchedulingTestAccount(t, svc, WithOpenAIPreferNativeResponses(context.Background()), PlatformOpenAI, "", map[int64]struct{}{100: {}}, false)
			require.True(t, selection.Acquired)
			require.Equal(t, tc.wantAcquired[len(tc.wantAcquired)-1], selection.Account.ID)
			require.Equal(t, tc.wantAcquired, acquired)
		})
	}
}

func TestNativeResponsesSelectionPreservesInitialPreferenceStickyAndExclusions(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		for _, tc := range []struct {
			name, session string
			excluded      map[int64]struct{}
			wantID        int64
		}{
			{name: "initial_native_before_priority", wantID: 2},
			{name: "sticky_converter", session: "sticky", wantID: 1},
			{name: "excluded_native", excluded: map[int64]struct{}{2: {}}, wantID: 1},
		} {
			name := "legacy/" + tc.name
			if advanced {
				name = "advanced/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				accounts := []Account{nativeResponsesSchedulingTestAccount(1, 0, false), nativeResponsesSchedulingTestAccount(2, 10, true)}
				svc := nativeResponsesSchedulingTestService(t, accounts, advanced, false, schedulerTestConcurrencyCache{})
				if tc.session != "" {
					groupID := int64(1)
					require.NoError(t, svc.setStickySessionAccountID(context.Background(), &groupID, tc.session, 1, openaiStickySessionTTL))
				}
				selection := selectNativeResponsesSchedulingTestAccount(t, svc, WithOpenAIPreferNativeResponses(context.Background()), PlatformOpenAI, tc.session, tc.excluded, false)
				require.Equal(t, tc.wantID, selection.Account.ID)
			})
		}
	}
}

func TestNativeResponsesAdvancedSelectionKeepsCompactFirst(t *testing.T) {
	for _, fill := range []bool{false, true} {
		name := "initial"
		if fill {
			name = "fill"
		}
		t.Run(name, func(t *testing.T) {
			chat := nativeResponsesSchedulingTestAccount(1, 1, false)
			chat.Extra["openai_compact_supported"] = true
			native := nativeResponsesSchedulingTestAccount(2, 0, true)
			svc := nativeResponsesSchedulingTestService(t, []Account{chat, native}, true, false, schedulerTestConcurrencyCache{})
			var excluded map[int64]struct{}
			if fill {
				excluded = map[int64]struct{}{100: {}}
			}
			selection := selectNativeResponsesSchedulingTestAccount(t, svc, WithOpenAIPreferNativeResponses(context.Background()), PlatformOpenAI, "", excluded, true)
			require.Equal(t, chat.ID, selection.Account.ID)
		})
	}
}
