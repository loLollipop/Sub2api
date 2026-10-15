package service

import (
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"testing"
)

func TestBedrockSonnet55GlobalProfileSurvivesRegionMapping(t *testing.T) {
	for _, model := range []string{"claude-sonnet-5-5", "global.anthropic.claude-sonnet-5-5", "public-alias"} {
		a := &Account{Credentials: map[string]any{"aws_region": "eu-west-1", "model_mapping": map[string]any{"public-alias": "claude-sonnet-5-5"}}}
		resolved, ok := ResolveBedrockModelID(a, model)
		require.True(t, ok)
		require.Equal(t, "global.anthropic.claude-sonnet-5-5", resolved)
	}
}

func TestBedrockSonnet55EffortStructuredOutputAndThinking(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-5-5","messages":[{"role":"user","content":"hi"}],"output_config":{"effort":"medium","format":{"type":"json_schema","schema":{"type":"object"}}},"thinking":{"type":"enabled","budget_tokens":1000}}`)
	got, err := PrepareBedrockRequestBodyWithTokens(body, "global.anthropic.claude-sonnet-5-5", nil, true)
	require.NoError(t, err)
	require.Equal(t, "medium", gjson.GetBytes(got, "output_config.effort").String())
	require.False(t, gjson.GetBytes(got, "output_config.format").Exists())
	require.Equal(t, `{"type":"object"}`, gjson.GetBytes(got, "messages.0.content.1.text").String())
	require.Equal(t, "adaptive", gjson.GetBytes(got, "thinking.type").String())
	require.False(t, gjson.GetBytes(got, "thinking.budget_tokens").Exists())
	got = sanitizeBedrockThinking([]byte(`{"thinking":{"type":"disabled"}}`), "global.anthropic.claude-sonnet-5-5")
	require.Equal(t, "between_tools", gjson.GetBytes(got, "thinking.type").String())
}
