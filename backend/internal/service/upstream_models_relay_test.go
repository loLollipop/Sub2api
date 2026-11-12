package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilterUpstreamModelsPreservesOfficialDefaultsAndRegionalHosts(t *testing.T) {
	models := []string{"deepseek-chat", "glm-5", "kimi-k2", "MiniMax-M2", "grok-4", "qwen3"}
	for _, tc := range []struct {
		platform string
		base     string
		want     string
	}{
		{PlatformDeepseek, "", "deepseek-chat"},
		{PlatformDeepseek, "https://api.deepseek.com/v1", "deepseek-chat"},
		{PlatformKimi, "", "kimi-k2"},
		{PlatformKimi, "https://api.moonshot.ai/v1", "kimi-k2"},
		{PlatformKimi, "https://api.kimi.com/coding", "kimi-k2"},
		{PlatformKimi, "https://api.kimi.ai/coding/v1", "kimi-k2"},
		{PlatformKimi, "https://API.MOONSHOT.CN./v1", "kimi-k2"},
		{PlatformZhipu, "", "glm-5"},
		{PlatformZhipu, "https://open.bigmodel.cn/api/paas/v4", "glm-5"},
		{PlatformZhipu, "https://api.z.ai/api/paas/v4", "glm-5"},
		{PlatformMiniMax, "", "MiniMax-M2"},
		{PlatformMiniMax, "https://api.minimaxi.com/v1", "MiniMax-M2"},
		{PlatformMiniMax, "https://api.minimax.io/v1", "MiniMax-M2"},
		{PlatformMiniMax, "https://api.minimax.com/v1", "MiniMax-M2"},
		{PlatformGrok, "", "grok-4"},
		{PlatformGrok, "https://api.x.ai/v1", "grok-4"},
		{PlatformGrok, "https://cli-chat-proxy.grok.com/v1", "grok-4"},
	} {
		t.Run(tc.platform+"/"+tc.base, func(t *testing.T) {
			account := &Account{Platform: tc.platform, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": tc.base}}
			require.Equal(t, []string{tc.want}, filterUpstreamModelsByAccountPlatform(account, models))
		})
	}
	for _, base := range []string{"", "https://cli-chat-proxy.grok.com/v1"} {
		account := grokOAuthModelSyncTestAccount(base)
		require.Equal(t, []string{"grok-4"}, filterUpstreamModelsByAccountPlatform(account, models))
	}
	for _, base := range []string{"https://relay.example/v1", "https://api.moonshot.cn.relay.example/v1"} {
		account := &Account{Platform: PlatformKimi, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": base}}
		require.Equal(t, models, filterUpstreamModelsByAccountPlatform(account, models))
	}
	// Adaptive discovery takes its Chat URL from api_base_urls before base_url.
	adaptive := &Account{Platform: PlatformKimi, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_protocol": APIProtocolAdaptive,
		"base_url":     "https://api.moonshot.cn/v1",
		"api_base_urls": map[string]any{
			APIProtocolChatCompletions: "https://relay.example/v1",
		},
	}}
	require.Equal(t, models, filterUpstreamModelsByAccountPlatform(adaptive, models))
	adaptive.Credentials["base_url"] = "https://relay.example/v1"
	adaptive.Credentials["api_base_urls"] = map[string]any{APIProtocolChatCompletions: "https://api.moonshot.cn/v1"}
	require.Equal(t, []string{"kimi-k2"}, filterUpstreamModelsByAccountPlatform(adaptive, models))
}

func TestSyncUpstreamModelCatalogKeepsRelayVendorModels(t *testing.T) {
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"deepseek-chat"},{"id":"kimi-k2"},{"id":"qwen3"}]}`))},
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`))},
	}}
	repo := &upstreamModelMetadataRepoStub{}
	svc := &AccountTestService{validateTestTarget: allowFakeAccountTestTarget, accountRepo: repo, httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}
	account := &Account{
		ID: 517, Platform: PlatformKimi, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "synthetic-test-key", "base_url": "https://relay.example/v1"},
	}

	catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, []string{"deepseek-chat", "kimi-k2", "qwen3"}, catalog.Models)
	require.Equal(t, "relay.example", upstream.requests[0].URL.Hostname())
}
