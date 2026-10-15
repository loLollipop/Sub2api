package migrations

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestTypeSafePlatformMigrationPreservesLocalPlatforms(t *testing.T) {
	raw, err := FS.ReadFile("266_add_typesafe_platform.sql")
	require.NoError(t, err)
	sql := string(raw)
	for _, platform := range domain.ConcretePlatformIDs() {
		require.Equal(t, 2, strings.Count(sql, "'"+platform+"'"), platform)
	}
	require.Contains(t, sql, "user_platform_quotas_platform_check")
	require.Contains(t, sql, "composite_model_routes_target_platform_check")
	require.NotContains(t, strings.ToUpper(sql), "NOT VALID")
	require.NotContains(t, strings.ToUpper(sql), "DELETE FROM")
}
