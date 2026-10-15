//go:build unit

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/ent/compositemodelroute"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/ent/userplatformquota"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestProviderCatalogRuntimePlatformValidation(t *testing.T) {
	for _, platform := range domain.ConcretePlatformIDs() {
		require.NoError(t, userplatformquota.PlatformValidator(platform), platform)
		require.NoError(t, compositemodelroute.TargetPlatformValidator(platform), platform)
		require.NoError(t, validateQuotaPlatforms([]UserPlatformQuotaRecord{{Platform: platform}}))
	}
	for _, platform := range []string{"", "composite", "TypeSafe", "typesafe ", "opencode_go", "Mixed", "cline "} {
		require.Error(t, userplatformquota.PlatformValidator(platform), platform)
		require.Error(t, compositemodelroute.TargetPlatformValidator(platform), platform)
		// Validation must run before acquiring a transaction/database client.
		repo := &userPlatformQuotaRepository{}
		require.Error(t, repo.BulkInsertInitial(context.Background(), []UserPlatformQuotaRecord{{Platform: platform}}))
		require.Error(t, repo.UpsertForUser(context.Background(), 1, []UserPlatformQuotaRecord{{Platform: platform}}))
	}
}
