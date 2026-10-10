package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSTurnPricingCurrentOr(t *testing.T) {
	fallback := time.Date(2024, time.January, 2, 2, 0, 0, 0, time.UTC)

	t.Run("frozen time takes precedence", func(t *testing.T) {
		frozen := fallback.Add(time.Minute)
		var p openAIWSTurnPricing
		p.freeze(frozen)
		require.Equal(t, frozen, p.currentOr(fallback))
	})

	t.Run("zero value falls back to turn start", func(t *testing.T) {
		var p openAIWSTurnPricing
		require.Equal(t, fallback, p.currentOr(fallback))
	})
}

type wsTurnAuthRepo struct {
	service.APIKeyRepository
	key   *service.APIKey
	err   error
	calls int
}

func (r *wsTurnAuthRepo) GetByKeyForAuth(context.Context, string) (*service.APIKey, error) {
	r.calls++
	return r.key, r.err
}

func TestOpenAIWSTurnBillingRefreshPricesAndFallbacks(t *testing.T) {
	for _, scenario := range []string{"repriced", "lookup-error", "key-changed", "group-changed", "missing-group", "platform-changed", "subscription-changed"} {
		t.Run(scenario, func(t *testing.T) {
			original := &service.Group{ID: 9, Platform: service.PlatformOpenAI, SubscriptionType: "standard", RateMultiplier: 0.15, Hydrated: true, Status: service.StatusActive}
			conn := &service.APIKey{ID: 7, Key: "test-ws-key", UserID: 42, User: &service.User{ID: 42, Status: service.StatusActive}, Group: original, GroupID: &original.ID, Status: service.StatusActive}
			group := *original
			group.RateMultiplier = 0.3
			latest := *conn
			latest.Group = &group
			repo := &wsTurnAuthRepo{key: &latest}
			switch scenario {
			case "lookup-error":
				repo.err = errors.New("lookup unavailable")
			case "key-changed":
				latest.ID++
			case "group-changed":
				group.ID = 17
				latest.GroupID = &group.ID
			case "missing-group":
				latest.Group = nil
			case "platform-changed":
				group.Platform = service.PlatformGrok
			case "subscription-changed":
				group.SubscriptionType = "subscription"
			}
			auth := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, nil)
			ctx := context.WithValue(context.Background(), ctxkey.Group, original)
			var turns openAIWSTurnBillingAPIKeys
			firstCtx := turns.begin(ctx, auth, 1, conn)
			require.Zero(t, repo.calls, "first turn uses connection snapshot")
			secondCtx := turns.begin(firstCtx, auth, 2, conn)
			require.Equal(t, 1, repo.calls)
			firstKey, secondKey := turns.forTurn(1, conn), turns.forTurn(2, conn)
			gateway := &service.OpenAIGatewayService{}
			first := gateway.BalancePreauthorizationCostInput(firstCtx, firstKey, "gpt-5.1", time.Now(), "")
			second := gateway.BalancePreauthorizationCostInput(secondCtx, secondKey, "gpt-5.1", time.Now(), "")
			require.Equal(t, 0.15, first.RateMultiplier)
			if scenario == "repriced" {
				require.Equal(t, 0.3, second.RateMultiplier, "later turn uses refreshed pricing")
				currentGroup, ok := secondCtx.Value(ctxkey.Group).(*service.Group)
				require.True(t, ok, "turn context must contain its billing group")
				require.Equal(t, 0.3, currentGroup.RateMultiplier)
				require.NotSame(t, conn, secondKey)
			} else {
				require.Equal(t, 0.15, second.RateMultiplier)
				require.Same(t, conn, secondKey, "changed identity or unavailable lookup keeps connection billing")
			}
			require.Equal(t, 0.15, conn.Group.RateMultiplier, "connection/auth snapshot remains immutable")
		})
	}
}

// TestOpenAIWSTurnPricingFreezePerTurn 钉死每个 turn 的 BeforeTurn 都会覆盖
// 上一个 turn 的定价时刻：长连接跨峰谷时后续 turn 不得沿用旧时刻。
func TestOpenAIWSTurnPricingFreezePerTurn(t *testing.T) {
	var p openAIWSTurnPricing
	turn1 := time.Now().Add(-time.Hour)
	turn2 := time.Now()

	p.freeze(turn1)
	require.Equal(t, turn1, p.currentOr(time.Time{}))

	p.freeze(turn2)
	require.Equal(t, turn2, p.currentOr(time.Time{}), "后续 turn 必须使用自己的定价时刻")
}
