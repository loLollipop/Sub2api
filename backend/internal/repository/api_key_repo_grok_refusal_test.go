package repository

import (
	"context"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAPIKeyRepositoryGrokRefusalPriceColdAuthProjection(t *testing.T) {
	for _, price := range []*float64{nil, func() *float64 { v := 0.0; return &v }(), func() *float64 { v := .37; return &v }()} {
		t.Run(fmt.Sprint(price), func(t *testing.T) {
			repo, client := newAPIKeyRepoSQLite(t)
			ctx := context.Background()
			user := mustCreateAPIKeyRepoUser(t, ctx, client, "grok-price@test.com")
			group, err := client.Group.Create().SetName("grok").SetPlatform(service.PlatformGrok).SetNillableGrokModerationRefusalPrice(price).Save(ctx)
			require.NoError(t, err)
			key := &service.APIKey{UserID: user.ID, Key: "sk-refusal-projection", Name: "refusal", GroupID: &group.ID, Status: service.StatusActive}
			require.NoError(t, repo.Create(ctx, key))
			loaded, err := repo.GetByKeyForAuth(ctx, key.Key)
			require.NoError(t, err)
			require.NotNil(t, loaded.Group)
			require.Equal(t, price, loaded.Group.GrokModerationRefusalPrice)
		})
	}
}
