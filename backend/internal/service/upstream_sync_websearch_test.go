//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/websearch"
	"github.com/stretchr/testify/require"
)

func TestOpenAIMessagesWebSearchRespectsNativeProtocol(t *testing.T) {
	previous := getWebSearchManager()
	SetWebSearchManager(websearch.NewManager(nil, nil))
	t.Cleanup(func() { SetWebSearchManager(previous) })
	setGlobalWebSearchConfig(&WebSearchEmulationConfig{Enabled: true, Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "test"}}})
	t.Cleanup(clearGlobalWebSearchConfig)
	// No user query ensures emulation returns before making any search call.
	body := []byte(`{"model":"gpt-6.1-sol","max_tokens":32,"tools":[{"type":"web_search"}],"messages":[]}`)
	for _, protocol := range []string{APIProtocolAnthropic, APIProtocolChatCompletions, APIProtocolResponses} {
		t.Run(protocol, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{err: errors.New("native capture")}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream, settingService: newSettingServiceForWebSearchTest(true)}
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test", "base_url": "http://native.example", "api_protocol": protocol}, Extra: map[string]any{"web_search_emulation": WebSearchModeEnabled}}
			if protocol == APIProtocolAnthropic {
				account = adaptiveProtocolTestAccount(PlatformZhipu, map[string]any{APIProtocolAnthropic: "http://native.example"})
				account.Extra = map[string]any{"web_search_emulation": WebSearchModeEnabled}
			}
			c := adaptiveProtocolTestContext("/v1/messages", body)
			c.Set("api_key", &APIKey{Group: &Group{ID: 1, Platform: account.Platform, CrossProtocolConversionEnabled: false}})
			_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
			require.Error(t, err)
			if protocol == APIProtocolAnthropic {
				require.NotNil(t, upstream.lastReq, "native request bypasses emulation")
				require.Equal(t, "/v1/messages", upstream.lastReq.URL.Path)
			} else {
				require.Nil(t, upstream.lastReq)
				require.Contains(t, err.Error(), "no query found", "emulation runs before the disabled cross-protocol gate")
			}
		})
	}
}
