package service

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func TestAnthropicCacheSnapshotsAcrossParsers(t *testing.T) {
	for _, parser := range []string{"typed", "generic", "passthrough", "antigravity"} {
		t.Run(parser, func(t *testing.T) {
			var usage ClaudeUsage
			for i, tc := range []struct {
				raw                   string
				input, write, oneHour int
			}{
				{`{"input_tokens":12000,"prompt_tokens":12000,"cache_creation_input_tokens":11000,"cache_creation":{"ephemeral_1h_input_tokens":3000}}`, 1000, 11000, 3000},
				{`{"cache_creation":{"ephemeral_1h_input_tokens":4000}}`, 1000, 11000, 4000},
				{`{"cache_read_input_tokens":500}`, 500, 11000, 4000},
				{`{"cache_creation_input_tokens":0,"cache_read_input_tokens":11500}`, 500, 0, 0},
				{`{"cache_creation":{"ephemeral_1h_input_tokens":4000}}`, 500, 0, 0},
				{`{"input_tokens":0,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}`, 0, 0, 0},
				{`{"output_tokens":4}`, 0, 0, 0},
			} {
				event := `{"type":"message_delta","usage":` + tc.raw + `}`
				if i == 0 {
					event = `{"type":"message_start","message":{"usage":` + tc.raw + `}}`
				}
				switch parser {
				case "typed":
					var patch apicompat.AnthropicUsage
					require.NoError(t, json.Unmarshal([]byte(tc.raw), &patch))
					mergeAnthropicUsage(&usage, patch)
				case "generic":
					(&GatewayService{}).parseSSEUsage(event, &usage)
				case "passthrough":
					parseSSEUsagePassthrough(event, &usage)
				case "antigravity":
					(&AntigravityGatewayService{}).extractSSEUsage("data: "+event, &usage)
				}
				require.Equal(t, tc.input, usage.InputTokens, tc.raw)
				require.Equal(t, tc.write, usage.CacheCreationInputTokens, tc.raw)
				require.Equal(t, tc.oneHour, usage.CacheCreation1hTokens, tc.raw)
			}
		})
	}
}

func TestAnthropicCacheSnapshotNonStreaming(t *testing.T) {
	require.Zero(t, (&AntigravityGatewayService{}).extractClaudeUsage([]byte(`{"usage":{"cache_creation_input_tokens":30}`)).CacheCreationInputTokens)
	for _, tc := range []struct {
		raw            string
		write, oneHour int
	}{
		{`{"cache_creation":{"ephemeral_1h_input_tokens":3000}}`, 3000, 3000},
		{`{"cache_creation_input_tokens":11000,"cache_creation":{"ephemeral_1h_input_tokens":3000}}`, 11000, 3000},
		{`{"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_1h_input_tokens":3000}}`, 0, 0},
	} {
		for _, usage := range []*ClaudeUsage{parseClaudeUsageFromResponseBody([]byte(`{"usage":` + tc.raw + `}`)), (&AntigravityGatewayService{}).extractClaudeUsage([]byte(`{"usage":` + tc.raw + `}`))} {
			require.Equal(t, tc.write, usage.CacheCreationInputTokens)
			require.Equal(t, tc.oneHour, usage.CacheCreation1hTokens)
		}
	}
}

func TestCacheTTLOverrideUsesAggregateAndExplicitZero(t *testing.T) {
	for _, tc := range []struct {
		aggregate int
		present   bool
		want      int
	}{{11000, true, 11000}, {0, true, 0}, {0, false, 3000}} {
		for _, target := range []string{"5m", "1h"} {
			usage := ClaudeUsage{CacheCreationInputTokens: tc.aggregate, cacheCreationInputTokensPresent: tc.present, CacheCreation1hTokens: 3000}
			applyCacheTTLOverride(&usage, target)
			require.Equal(t, tc.want, usage.CacheCreation5mTokens+usage.CacheCreation1hTokens)
			require.False(t, applyCacheTTLOverride(&usage, target), "repeated override must be idempotent")
			raw := map[string]any{"cache_creation": map[string]any{"ephemeral_1h_input_tokens": float64(3000)}}
			if tc.present {
				raw["cache_creation_input_tokens"] = float64(tc.aggregate)
			}
			rewriteCacheCreationJSON(raw, target)
			data, err := json.Marshal(map[string]any{"usage": raw})
			require.NoError(t, err)
			parsed := parseClaudeUsageFromResponseBody(data)
			require.Equal(t, tc.want, parsed.CacheCreation5mTokens+parsed.CacheCreation1hTokens)
		}
	}
}

func TestCacheCreationUnclassifiedRemainderBilling(t *testing.T) {
	for _, tc := range []struct {
		name             string
		total, five, one int
		cost             float64
	}{
		{"partial 1h", 11000, 0, 3000, 28},
		{"partial mixed", 11000, 1000, 2000, 26},
		{"no details", 11000, 0, 0, 22},
		{"oversized details", 100, 50, 200, .36},
		{"zero", 0, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tokens := UsageTokens{InputTokens: 10, CacheCreationTokens: tc.total, CacheCreation5mTokens: tc.five, CacheCreation1hTokens: tc.one}
			pricing := &ModelPricing{SupportsCacheBreakdown: true, CacheCreation5mPrice: .002, CacheCreation1hPrice: .004}
			require.InDelta(t, tc.cost, (&BillingService{}).computeCacheCreationCost(pricing, tokens, .002, 1), 1e-12)
			input, five, one := .001, .002, .004
			stats := calculateTokenStatsCost(&ChannelModelPricing{InputPrice: &input, CacheWritePrice: &five, CacheWrite1hPrice: &one}, tokens)
			require.NotNil(t, stats)
			require.InDelta(t, tc.cost+.01, *stats, 1e-12)
		})
	}
}
