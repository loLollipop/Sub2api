package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestUsesNativeGeminiUpstream(t *testing.T) {
	require.False(t, (*Account)(nil).UsesNativeGeminiUpstream())
	for _, protocol := range []string{"", " adaptive ", "native", APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
		t.Run(protocol, func(t *testing.T) {
			account := geminiBridgeAccount(protocol)
			wantNative := protocol == "" || protocol == " adaptive " || protocol == "native"
			require.Equal(t, wantNative, account.UsesNativeGeminiUpstream())
			// Native routing must not change OpenAI/Anthropic compatibility detection.
			require.Equal(t, protocol != "native", account.IsGeminiOpenAIProtocol())
			require.Equal(t, account.IsGeminiOpenAIProtocol(), account.IsOpenAICompatible())
		})
	}
	require.True(t, (&Account{Platform: PlatformGemini, Type: AccountTypeOAuth}).UsesNativeGeminiUpstream())
	require.True(t, (&Account{Platform: PlatformGemini, Type: AccountTypeServiceAccount}).UsesNativeGeminiUpstream())
}

func TestGeminiNativeURLsPreserveVersionedRelayPrefix(t *testing.T) {
	svc := &AccountTestService{cfg: &config.Config{}}
	for _, prefix := range []string{"/relay/v1", "/relay/v1beta", "/relay/v1beta/models"} {
		t.Run(prefix, func(t *testing.T) {
			// Literal public IP avoids DNS in the account-test target guard;
			// these tests only build requests and never send them.
			base := "https://8.8.8.8" + prefix
			account := &Account{Type: AccountTypeAPIKey, Platform: PlatformGemini, Credentials: map[string]any{
				"api_key": "test", "base_url": "https://compat.example.com/v1",
				"api_base_urls": map[string]any{"gemini": base + "/v1beta"},
			}}
			nativeBase := account.GetGeminiBaseURL("")
			require.Equal(t, base, nativeBase)
			generated, err := buildGeminiAIStudioModelActionURL(nativeBase, "gemini-native", "countTokens", false)
			require.NoError(t, err)
			require.Equal(t, base+"/v1beta/models/gemini-native:countTokens", generated)
			require.Equal(t, base+"/v1beta/models", buildGeminiModelsURL(nativeBase))
			probe, err := svc.buildGeminiAPIKeyRequest(context.Background(), account, "gemini-native", []byte(`{}`))
			require.NoError(t, err)
			require.Equal(t, base+"/v1beta/models/gemini-native:streamGenerateContent?alt=sse", probe.URL.String())
			catalog, err := svc.buildGeminiUpstreamModelsRequest(context.Background(), account)
			require.NoError(t, err)
			require.Equal(t, base+"/v1beta/models", catalog.URL.String())
		})
	}
}

func TestGeminiNativeBaseURLPrecedence(t *testing.T) {
	const fallback = "https://generativelanguage.googleapis.com"
	for _, tc := range []struct {
		name        string
		accountType string
		platform    string
		base        string
		urls        any
		want        string
	}{
		{"dedicated URL", AccountTypeAPIKey, PlatformGemini, "https://compat.example.com/v1", map[string]any{"gemini": " https://native.example.com/relay/v1beta/models/ "}, "https://native.example.com/relay"},
		{"dedicated only", AccountTypeAPIKey, PlatformGemini, "", map[string]any{"gemini": "https://native.example.com/relay/v1beta"}, "https://native.example.com/relay"},
		{"blank dedicated", AccountTypeAPIKey, PlatformGemini, "https://relay.example.com/prefix/v1/responses/", map[string]any{"gemini": "  "}, "https://relay.example.com/prefix"},
		{"invalid dedicated", AccountTypeAPIKey, PlatformGemini, "https://relay.example.com/prefix/v1/chat/completions", map[string]any{"gemini": 42}, "https://relay.example.com/prefix"},
		{"other protocol only", AccountTypeAPIKey, PlatformGemini, "https://relay.example.com/prefix/v1", map[string]any{"responses": "https://responses.example.com/v1"}, "https://relay.example.com/prefix"},
		{"missing URL", AccountTypeAPIKey, PlatformGemini, "", nil, fallback},
		{"OAuth ignores dedicated", AccountTypeOAuth, PlatformGemini, "https://oauth.example.com/prefix/v1beta", map[string]any{"gemini": "https://native.example.com"}, "https://oauth.example.com/prefix"},
		{"Antigravity legacy suffix", AccountTypeAPIKey, PlatformAntigravity, "https://relay.example.com/v1/", nil, "https://relay.example.com/v1/antigravity"},
		{"Antigravity dedicated suffix", AccountTypeAPIKey, PlatformAntigravity, "https://compat.example.com", map[string]any{"gemini": "https://relay.example.com/v1/"}, "https://relay.example.com/v1/antigravity"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := &Account{Platform: tc.platform, Type: tc.accountType, Credentials: map[string]any{"base_url": tc.base, "api_base_urls": tc.urls}}
			require.Equal(t, tc.want, account.GetGeminiBaseURL(fallback))
		})
	}
}
