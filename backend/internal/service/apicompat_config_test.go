package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestConfiguredMinOutputTokensServiceConversions(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.MinOutputTokens = 256
	tokens := 32
	req, err := chatCompletionsToResponses(cfg, &apicompat.ChatCompletionsRequest{Model: "gpt-test", MaxTokens: &tokens})
	require.NoError(t, err)
	require.NotNil(t, req.MaxOutputTokens)
	require.Equal(t, 256, *req.MaxOutputTokens)
	converted, err := anthropicToResponses(cfg, &apicompat.AnthropicRequest{Model: "claude-test", MaxTokens: 32})
	require.NoError(t, err)
	require.NotNil(t, converted.MaxOutputTokens)
	require.Equal(t, 256, *converted.MaxOutputTokens)
	body, changed, err := normalizeOpenAIResponsesLegacyIngressWithConfig(cfg, []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}],"max_tokens":32}`))
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, int64(256), gjson.GetBytes(body, "max_output_tokens").Int())
	require.Equal(t, apicompat.DefaultMinMaxOutputTokens, configuredMinOutputTokens(nil))
	cfg.Gateway.MinOutputTokens = 1
	require.Equal(t, apicompat.DefaultMinMaxOutputTokens, configuredMinOutputTokens(cfg))
}
