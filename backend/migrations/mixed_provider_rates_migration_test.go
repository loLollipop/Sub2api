package migrations

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMixedProviderRatesMigration(t *testing.T) {
	body, err := FS.ReadFile("257_mixed_provider_rate_multipliers.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(body)), " ")
	require.Contains(t, sql, "SET LOCAL lock_timeout = '5s'")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS mixed_provider_rate_multipliers JSONB")
	require.Contains(t, sql, "OLD.mixed_provider_rate_multipliers IS NOT DISTINCT FROM NEW.mixed_provider_rate_multipliers")
	require.Contains(t, sql, "INSERT INTO auth_cache_invalidation_outbox")
	require.NotContains(t, sql, "UPDATE groups")
	require.NotContains(t, sql, "UPDATE accounts")
	previous, err := FS.ReadFile("193_group_profit_control_auth_cache_invalidation.sql")
	require.NoError(t, err)
	for _, field := range regexp.MustCompile(`OLD\.[a-z_]+ IS NOT DISTINCT FROM NEW\.[a-z_]+`).FindAllString(string(previous), -1) {
		require.Contains(t, sql, field, "preserve every existing invalidation predicate")
	}
}
