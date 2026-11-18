package migrations

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestOpenAIUltrafastAndCacheTTLMigrations(t *testing.T) {
	group, err := FS.ReadFile("271_group_force_openai_ultrafast.sql")
	require.NoError(t, err)
	require.Contains(t, string(group), "ADD COLUMN IF NOT EXISTS force_openai_ultrafast BOOLEAN NOT NULL DEFAULT FALSE")
	ultra, err := FS.ReadFile("272_channel_pricing_ultrafast_multiplier.sql")
	require.NoError(t, err)
	require.Contains(t, string(ultra), "ADD COLUMN IF NOT EXISTS ultrafast_multiplier NUMERIC(12,6)")
	require.Contains(t, string(ultra), "IF NOT EXISTS")
	require.Contains(t, string(ultra), "ultrafast_multiplier IS NULL OR ultrafast_multiplier > 0")
	ttl, err := FS.ReadFile("273_openai_cache_write_ttl.sql")
	require.NoError(t, err)
	require.Equal(t, 4, strings.Count(string(ttl), "ADD COLUMN IF NOT EXISTS cache_write_12h_price"))
	for _, name := range []string{"cache_creation_30m_tokens", "cache_creation_12h_tokens"} {
		require.Contains(t, string(ttl), "ADD COLUMN IF NOT EXISTS "+name+" INTEGER NOT NULL DEFAULT 0")
	}
}
