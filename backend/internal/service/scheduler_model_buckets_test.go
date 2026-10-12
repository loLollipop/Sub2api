package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountsSupportingRequestedModel_EmptyMappingDoesNotStealMappedSibling(t *testing.T) {
	empty := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{}}
	mapped := Account{
		ID:       2,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"gemini-3.8-flash": "gemini-3.8-flash"},
		},
	}

	got := accountsSupportingRequestedModel([]Account{empty, mapped}, "gemini-3.8-flash")
	require.Len(t, got, 1)
	require.Equal(t, int64(2), got[0].ID)
}

func TestAccountsSupportingRequestedModel_EmptyMappingFallbackWhenNobodyMapped(t *testing.T) {
	empty := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{}}
	got := accountsSupportingRequestedModel([]Account{empty}, "gemini-3.8-flash")
	require.Len(t, got, 1)
	require.Equal(t, int64(1), got[0].ID)
}

func TestAccountsSupportingRequestedModel_NormalizesAnthropicOAuthModelID(t *testing.T) {
	account := Account{
		ID:       1,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"claude-sonnet-4-5-20250929": "claude-sonnet-4-5-20250929",
			},
		},
	}

	got := accountsSupportingRequestedModel([]Account{account}, "claude-sonnet-4-5")
	require.Len(t, got, 1)
	require.Equal(t, int64(1), got[0].ID)
}

func TestAccountsSupportingRequestedModel_PassthroughRemainsInMappedPool(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
		for _, flag := range []string{"openai_passthrough", "openai_oauth_passthrough"} {
			t.Run(accountType+"/"+flag, func(t *testing.T) {
				accounts := []Account{
					{ID: 1, Platform: PlatformOpenAI, Type: accountType, Credentials: map[string]any{}, Extra: map[string]any{flag: true}},
					{ID: 2, Platform: PlatformOpenAI, Type: accountType, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-6.1-sol": "gpt-6.1-sol"}}},
					{ID: 3, Platform: PlatformOpenAI, Type: accountType, Credentials: map[string]any{}},
					{ID: 4, Platform: PlatformOpenAI, Type: accountType, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6": "gpt-5.6"}}},
				}
				got, unsupported := filterAccountsSupportingRequestedModel(accounts, "gpt-6.1-sol")
				var ids []int64
				for _, account := range got {
					ids = append(ids, account.ID)
				}
				require.Equal(t, []int64{1, 2}, ids)
				require.Equal(t, 2, unsupported)
			})
		}
	}
}

func TestAccountsSupportingRequestedModel_PassthroughWithLeftoverMapping(t *testing.T) {
	accounts := []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"openai_passthrough": true}},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"openai_passthrough": true}, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6": "gpt-5.6"}}},
	}
	got, unsupported := filterAccountsSupportingRequestedModel(accounts, "gpt-6.1-sol")
	require.Len(t, got, 2)
	require.Zero(t, unsupported)
}

func TestAccountsSupportingRequestedModel_PassthroughDoesNotHideOrdinaryFallback(t *testing.T) {
	accounts := []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"openai_passthrough": true}},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
	}
	got, unsupported := filterAccountsSupportingRequestedModel(accounts, "gpt-6.1-sol")
	require.Len(t, got, 2)
	require.Equal(t, int64(1), got[0].ID)
	require.Equal(t, int64(2), got[1].ID)
	require.Zero(t, unsupported)
}

func TestAccountsSupportingRequestedModel_PassthroughFlagValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		extra map[string]any
	}{
		{"disabled_overrides_legacy", map[string]any{"openai_passthrough": false, "openai_oauth_passthrough": true}},
		{"string_is_not_enabled", map[string]any{"openai_passthrough": "true"}},
		{"legacy_string_is_not_enabled", map[string]any{"openai_oauth_passthrough": "true"}},
		{"missing_flag", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			accounts := []Account{
				{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: tc.extra},
				{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-6.1-sol": "gpt-6.1-sol"}}},
			}
			got, unsupported := filterAccountsSupportingRequestedModel(accounts, "gpt-6.1-sol")
			require.Len(t, got, 1)
			require.Equal(t, int64(2), got[0].ID)
			require.Equal(t, 1, unsupported)
		})
	}
}

func TestAccountsSupportingRequestedModel_PassthroughFlagDoesNotApplyToOtherPlatforms(t *testing.T) {
	accounts := []Account{
		{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Extra: map[string]any{"openai_passthrough": true}},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-6.1-sol": "gpt-6.1-sol"}}},
	}
	got, unsupported := filterAccountsSupportingRequestedModel(accounts, "gpt-6.1-sol")
	require.Len(t, got, 1)
	require.Equal(t, int64(2), got[0].ID)
	require.Equal(t, 1, unsupported)
}
