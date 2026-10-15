//go:build unit

package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestProviderCatalogPreservesMixedIdentity(t *testing.T) {
	spec, ok := domain.LookupPlatform(PlatformMixed)
	require.True(t, ok)
	require.Equal(t, domain.PlatformGatewayOpenAI, spec.Gateway)
	require.False(t, spec.CNProvider)
	require.Empty(t, spec.LiteLLMProvider)
	require.Nil(t, LookupProviderProfile(PlatformMixed))
	require.False(t, IsMultiProtocolAPIKeyProvider(PlatformMixed))
	for _, entry := range BuildPlatformCatalog().Platforms {
		if entry.ID == PlatformMixed {
			require.Nil(t, entry.MultiProtocol)
		}
		require.NotEqual(t, "opencode_go", entry.ID)
		if entry.ID == PlatformTypeSafe {
			require.Equal(t, domain.PlatformGatewaySystemOne, entry.Gateway)
			require.Nil(t, entry.MultiProtocol)
		}
	}
	account := &Account{Platform: PlatformMixed, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test", "base_url": DefaultDeepseekBaseURL}}
	require.Equal(t, PlatformDeepseek, account.providerProfile().Platform)
	require.Equal(t, PlatformMixed, account.Platform)
	account.Credentials["base_url"] = "https://relay.example/v1"
	require.Nil(t, account.providerProfile())
}

func TestProviderAccountProbesRejectUntrustedOrigins(t *testing.T) {
	for _, platform := range []string{PlatformCline, PlatformCommandCode} {
		host := "api.cline.bot"
		if platform == PlatformCommandCode {
			host = "api.commandcode.ai"
		}
		for _, base := range []string{"http://" + host, "https://" + host + ":444/v1", "https://user:pass@" + host, "https://" + host + "?tenant=x", "https://" + host + "#secret", "https://" + host + ".evil.example"} {
			account := &Account{Platform: platform, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test", "base_url": base}}
			require.False(t, account.clineAccountAPISupported(), base)
			require.False(t, account.commandCodeUsageSupported(), base)
			require.Error(t, validatePayGAccount(account), base)
			require.Error(t, validateCodingPlanAccount(account), base)
		}
		account := &Account{Platform: platform, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test", "api_protocol": APIProtocolAdaptive, "api_base_urls": map[string]any{APIProtocolAnthropic: "https://relay.example/v1"}}}
		require.False(t, account.providerAccountAPIOriginAllowed(host))
	}
}

func TestCommandCodeRelayUsesConfiguredOriginForNativeAnthropic(t *testing.T) {
	account := commandCodeTestAccount(981)
	account.Credentials["base_url"] = "https://relay.example/provider/v1"
	target, err := (&OpenAIGatewayService{}).nativeAnthropicTargetURL(account)
	require.NoError(t, err)
	require.Equal(t, "https://relay.example/provider/v1/messages", target)
	require.False(t, account.commandCodeUsageSupported())
}

func TestCommandCodeGatewayRoutesAfterAccountModelMapping(t *testing.T) {
	for _, ingress := range routingMatrixIngresses() {
		account := commandCodeTestAccount(991)
		account.Credentials["model_mapping"] = map[string]any{"public-alias": "claude-sonnet-4"}
		// Avoid catalog I/O while exercising model rules through real entry points.
		catalogURL := buildOpenAIModelsURL(DefaultCommandCodeBaseURL)
		upstreamModelProtocols.store(modelProtocolCatalogKey(account, catalogURL), map[string][]string{"claude-sonnet-4": {APIProtocolAnthropic}}, nil, time.Now())
		upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
		svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
		body := routingMatrixCase{ingress: ingress, model: "public-alias"}.body()
		_ = ingress.forward(svc, adaptiveProtocolTestContext(ingress.path, body), account, body)
		require.NotEmpty(t, upstream.requests, ingress.name)
		req := upstream.requests[len(upstream.requests)-1]
		require.True(t, strings.HasSuffix(req.URL.String(), "/provider/v1/messages"), req.URL)
		require.Equal(t, "claude-sonnet-4", gjson.GetBytes(upstream.lastBody, "model").String())
		require.Equal(t, "user_test_key", req.Header.Get("x-api-key"))
	}
}

func TestProviderWalletScopeUsesMappedModel(t *testing.T) {
	account := clineTestAccount(992)
	account.Credentials["model_mapping"] = map[string]any{"public-alias": "cline-pass/glm-5.3-flash"}
	require.Contains(t, account.modelRateLimitKeysForRequest(context.Background(), "public-alias"), clinePassRateLimitKey)
	require.NotContains(t, account.modelRateLimitKeysForRequest(context.Background(), "public-alias"), clineCreditsRateLimitKey)
}
