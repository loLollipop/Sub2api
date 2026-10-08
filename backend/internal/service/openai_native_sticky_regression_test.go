package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type nativeStickyListErrorRepo struct {
	schedulerTestOpenAIAccountRepo
}

func (r nativeStickyListErrorRepo) ListSchedulableByGroupIDAndPlatform(context.Context, int64, string) ([]Account, error) {
	return nil, errors.New("candidate lookup unavailable")
}

func TestNativeResponsesOrdinaryStickySelection(t *testing.T) {
	for _, route := range []struct {
		name            string
		advanced, batch bool
		cache           schedulerTestConcurrencyCache
	}{
		{name: "legacy_no_batch"},
		{name: "legacy_batch", batch: true},
		{name: "legacy_load_error", batch: true, cache: schedulerTestConcurrencyCache{loadBatchErr: errors.New("load unavailable")}},
		{name: "advanced", advanced: true},
	} {
		for _, tc := range []struct {
			name       string
			unmarked   bool
			nativeBind bool
			listError  bool
			dropNative bool
			compact    bool
			excluded   map[int64]struct{}
			mutate     func(*Account, *Account)
			wantID     int64
		}{
			{name: "eligible_native_replaces_converter", wantID: 2},
			{name: "no_native_keeps_original_converter", dropNative: true, wantID: 1},
			{name: "model_ineligible_native_keeps_original", mutate: func(_, native *Account) {
				native.Credentials = map[string]any{"model_mapping": map[string]any{"other-model": "other-model"}}
			}, wantID: 1},
			{name: "excluded_native_keeps_original", excluded: map[int64]struct{}{2: {}}, wantID: 1},
			{name: "wrong_group_native_keeps_original", mutate: func(_, native *Account) {
				native.GroupIDs = []int64{9}
			}, wantID: 1},
			{name: "unhealthy_native_keeps_original", mutate: func(_, native *Account) {
				native.Schedulable = false
			}, wantID: 1},
			{name: "capability_ineligible_native_keeps_original", mutate: func(_, native *Account) {
				native.Credentials = map[string]any{"openai_capabilities": []any{"embeddings"}}
			}, wantID: 1},
			{name: "compact_tier_precedes_native", compact: true, mutate: func(sticky, _ *Account) {
				sticky.Extra["openai_compact_supported"] = true
			}, wantID: 1},
			{name: "candidate_lookup_error_keeps_original", listError: true, wantID: 1},
			{name: "unmarked_keeps_original_converter", unmarked: true, wantID: 1},
			{name: "native_sticky_keeps_original", nativeBind: true, wantID: 2},
		} {
			t.Run(route.name+"/"+tc.name, func(t *testing.T) {
				sticky := nativeResponsesSchedulingTestAccount(1, 0, false)
				native := nativeResponsesSchedulingTestAccount(2, 10, true)
				if tc.mutate != nil {
					tc.mutate(&sticky, &native)
				}
				// This higher-priority converter makes accidental rebinding visible.
				accounts := []Account{sticky, nativeResponsesSchedulingTestAccount(3, -10, false)}
				if !tc.dropNative {
					accounts = append(accounts, native)
				}
				svc := nativeResponsesSchedulingTestService(t, accounts, route.advanced, route.batch, route.cache)
				if tc.listError {
					svc.accountRepo = nativeStickyListErrorRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}}
				}
				ctx := context.Background()
				if !tc.unmarked {
					ctx = WithOpenAIPreferNativeResponses(ctx)
				}
				groupID := int64(1)
				stickyID := int64(1)
				if tc.nativeBind {
					stickyID = 2
				}
				require.NoError(t, svc.setStickySessionAccountID(ctx, &groupID, "sticky", stickyID, openaiStickySessionTTL))
				var selection *AccountSelectionResult
				if route.advanced && len(tc.excluded) > 0 {
					// Exclusions also turn on fill scheduling at the public entry point.
					// Exercise the ordinary sticky selector with exclusions independently;
					// the existing strict-priority fill contract stays intact.
					scheduler := newDefaultOpenAIAccountScheduler(svc, newOpenAIAccountRuntimeStats())
					var decision OpenAIAccountScheduleDecision
					var err error
					selection, decision, err = scheduler.Select(ctx, OpenAIAccountScheduleRequest{
						GroupID: &groupID, Platform: PlatformOpenAI, SessionHash: "sticky",
						RequestedModel: "gpt-test", ExcludedIDs: tc.excluded,
						RequiredCapability: OpenAIEndpointCapabilityChatCompletions, PreferNativeResponses: true,
					})
					require.NoError(t, err)
					require.True(t, decision.StickySessionHit)
					require.NotNil(t, selection)
					if selection.ReleaseFunc != nil {
						t.Cleanup(selection.ReleaseFunc)
					}
				} else {
					selection = selectNativeResponsesSchedulingTestAccount(t, svc, ctx, PlatformOpenAI, "sticky", tc.excluded, tc.compact)
				}
				require.Equal(t, tc.wantID, selection.Account.ID)
				bound, err := svc.getStickySessionAccountID(ctx, &groupID, "sticky")
				require.NoError(t, err)
				require.Equal(t, tc.wantID, bound)
			})
		}
	}
}

func TestNativeResponsesStickyProbeHonorsTransport(t *testing.T) {
	sticky := nativeResponsesSchedulingTestAccount(1, 0, false)
	sticky.Extra["openai_apikey_responses_websockets_v2_enabled"] = true
	native := nativeResponsesSchedulingTestAccount(2, 10, true)
	for _, advanced := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "advanced"}[advanced], func(t *testing.T) {
			svc := nativeResponsesSchedulingTestService(t, []Account{sticky, native}, advanced, false, schedulerTestConcurrencyCache{})
			svc.cfg.Gateway.OpenAIWS.Enabled = true
			svc.cfg.Gateway.OpenAIWS.APIKeyEnabled = true
			svc.cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
			ctx := WithOpenAIPreferNativeResponses(context.Background())
			groupID := int64(1)
			require.NoError(t, svc.setStickySessionAccountID(ctx, &groupID, "sticky", 1, openaiStickySessionTTL))
			selection, _, err := svc.SelectAccountWithSchedulerForCapability(ctx, &groupID, "", "sticky", "gpt-test", nil,
				OpenAIUpstreamTransportResponsesWebsocketV2, OpenAIEndpointCapabilityChatCompletions, false, false, false)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, sticky.ID, selection.Account.ID)
			if selection.ReleaseFunc != nil {
				t.Cleanup(selection.ReleaseFunc)
			}
			bound, err := svc.getStickySessionAccountID(ctx, &groupID, "sticky")
			require.NoError(t, err)
			require.Equal(t, sticky.ID, bound)
		})
	}
}

func TestNativeResponsesPreferencePreservesHardPreviousResponse(t *testing.T) {
	sticky := nativeResponsesSchedulingTestAccount(1, 0, false)
	sticky.Extra["openai_apikey_responses_websockets_v2_enabled"] = true
	native := nativeResponsesSchedulingTestAccount(2, 10, true)
	svc := nativeResponsesSchedulingTestService(t, []Account{sticky, native}, true, false, schedulerTestConcurrencyCache{})
	svc.cfg.Gateway.OpenAIWS.Enabled = true
	svc.cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	svc.cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	svc.cfg.Gateway.OpenAIWS.StickyResponseIDTTLSeconds = 3600
	ctx := WithOpenAIPreferNativeResponses(context.Background())
	groupID := int64(1)
	require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(ctx, groupID, "resp_native_pref_hard", 1, time.Hour))
	selection, decision, err := svc.SelectAccountWithSchedulerForCapability(ctx, &groupID, "resp_native_pref_hard", "sticky", "gpt-test", nil,
		OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, false, false, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, sticky.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerPreviousResponse, decision.Layer)
	require.True(t, decision.StickyPreviousHit)
	if selection.ReleaseFunc != nil {
		t.Cleanup(selection.ReleaseFunc)
	}
}

func TestNativeResponsesPreferencePreservesGuardianParentSticky(t *testing.T) {
	accounts := []Account{nativeResponsesSchedulingTestAccount(1, 0, false), nativeResponsesSchedulingTestAccount(2, 10, true)}
	svc := nativeResponsesSchedulingTestService(t, accounts, true, false, schedulerTestConcurrencyCache{})
	ctx := WithOpenAIPreferNativeResponses(context.Background())
	groupID := int64(1)
	require.NoError(t, svc.setStickySessionAccountID(ctx, &groupID, "sticky", 2, openaiStickySessionTTL))
	scheduler := newDefaultOpenAIAccountScheduler(svc, newOpenAIAccountRuntimeStats())
	selection, decision, err := scheduler.Select(ctx, OpenAIAccountScheduleRequest{
		GroupID: &groupID, Platform: PlatformOpenAI, SessionHash: "sticky", GuardianParentAccountID: 1,
		RequestedModel: "gpt-test", RequiredCapability: OpenAIEndpointCapabilityChatCompletions, PreferNativeResponses: true,
	})
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(1), selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerGuardianParent, decision.Layer)
	bound, err := svc.getStickySessionAccountID(ctx, &groupID, "sticky")
	require.NoError(t, err)
	require.Equal(t, int64(2), bound)
	if selection.ReleaseFunc != nil {
		t.Cleanup(selection.ReleaseFunc)
	}
}
