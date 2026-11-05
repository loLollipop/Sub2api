package repository

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestRedeemCodeRepositoryCreateTransaction(t *testing.T) {
	for _, commit := range []bool{false, true} {
		name := "rollback"
		if commit {
			name = "commit"
		}
		t.Run(name, func(t *testing.T) {
			_, client := newUserEntRepo(t)
			repo := NewRedeemCodeRepository(client)
			ctx := context.Background()
			user, err := client.User.Create().SetEmail("transaction-source@example.test").SetPasswordHash("test-hash").Save(ctx)
			require.NoError(t, err)
			tx, err := client.Tx(ctx)
			require.NoError(t, err)
			t.Cleanup(func() { _ = tx.Rollback() })
			usedAt := time.Now()
			code := &service.RedeemCode{
				Code: "transaction-source", Type: service.AdjustmentTypeAdminBalance,
				Value: 5, Status: service.StatusUsed, UsedBy: &user.ID, UsedAt: &usedAt,
			}
			require.NoError(t, repo.Create(dbent.NewTxContext(ctx, tx), code))
			if commit {
				require.NoError(t, tx.Commit())
			} else {
				require.NoError(t, tx.Rollback())
			}
			count, err := client.RedeemCode.Query().Where(redeemcode.CodeEQ(code.Code)).Count(ctx)
			require.NoError(t, err)
			if commit {
				require.Equal(t, 1, count)
			} else {
				require.Zero(t, count, "source record must roll back with its balance transaction")
			}
		})
	}
}
