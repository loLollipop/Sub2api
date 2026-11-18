package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnthropicCacheSnapshotsPreserveReportedAggregate(t *testing.T) {
	for _, tc := range []struct {
		name      string
		snapshots []string
		want      []int
	}{
		{"reported total", []string{`{"cache_creation_input_tokens":11000,"cache_creation":{"ephemeral_1h_input_tokens":3000}}`, `{"cache_creation":{"ephemeral_1h_input_tokens":4000}}`}, []int{11000, 11000}},
		{"derived total", []string{`{"cache_creation":{"ephemeral_5m_input_tokens":20}}`, `{"cache_creation":{"ephemeral_1h_input_tokens":30}}`, `{"cache_creation":{"ephemeral_5m_input_tokens":0}}`}, []int{20, 50, 30}},
		{"zero clears writes", []string{`{"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_1h_input_tokens":100}}`, `{"cache_creation_input_tokens":0,"cache_read_input_tokens":100}`, `{"cache_creation":{"ephemeral_1h_input_tokens":200}}`}, []int{100, 0, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var usage AnthropicUsage
			state := NewAnthropicEventToResponsesState()
			for i, raw := range tc.snapshots {
				var patch AnthropicUsage
				require.NoError(t, json.Unmarshal([]byte(raw), &patch))
				usage.Merge(patch)
				state.mergeUsage(patch)
				require.Equal(t, tc.want[i], usage.CacheCreationInputTokens)
				require.Equal(t, tc.want[i], state.CacheCreationInputTokens)
			}
		})
	}
}

func TestAnthropicCacheSnapshotsRetainPromptTotal(t *testing.T) {
	var usage AnthropicUsage
	for _, tc := range []struct {
		raw   string
		input int
	}{
		{`{"input_tokens":200,"prompt_tokens":200}`, 200},
		{`{"cache_creation":{"ephemeral_1h_input_tokens":30}}`, 170},
		{`{"cache_read_input_tokens":100}`, 70},
		{`{"cache_creation_input_tokens":0,"cache_read_input_tokens":130}`, 70},
		{`{"input_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}`, 0},
		{`{"output_tokens":4}`, 0},
	} {
		var patch AnthropicUsage
		require.NoError(t, json.Unmarshal([]byte(tc.raw), &patch))
		usage.Merge(patch)
		require.Equal(t, tc.input, usage.InputTokens, tc.raw)
	}
}

func TestAnthropicNonStreamingCacheDetailsAndZero(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		write int
	}{
		{`{"input_tokens":10,"cache_creation":{"ephemeral_1h_input_tokens":30}}`, 30},
		{`{"input_tokens":10,"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_1h_input_tokens":30}}`, 0},
	} {
		var usage AnthropicUsage
		require.NoError(t, json.Unmarshal([]byte(tc.raw), &usage))
		wire, err := json.Marshal(usage)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(wire, &usage))
		out := AnthropicToResponsesResponse(&AnthropicResponse{Usage: usage})
		require.Equal(t, 10+tc.write, out.Usage.InputTokens)
		require.Equal(t, tc.write, out.Usage.CacheCreationInputTokens)
		if tc.write == 0 {
			require.Nil(t, out.Usage.CacheCreation)
		}
	}
}
