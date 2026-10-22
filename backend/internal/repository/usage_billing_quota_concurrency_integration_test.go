//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUsageBillingQuotaConcurrentCommandsRemainIdempotent(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)
	user := mustCreateUser(t, client, &service.User{Email: uuid.NewString() + "@example.com", PasswordHash: "hash", Balance: 100})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-test-" + uuid.NewString(), Name: "quota concurrency"})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "quota-concurrent-" + uuid.NewString(), Platform: service.PlatformGemini,
		Type: service.AccountTypeServiceAccount, Extra: map[string]any{"quota_limit": 1.0},
	})
	const commands = 16
	var wg sync.WaitGroup
	errors := make(chan error, commands*2)
	for i := 0; i < commands; i++ {
		cmd := service.UsageBillingCommand{
			RequestID: uuid.NewString(), APIKeyID: key.ID, UserID: user.ID,
			AccountID: account.ID, AccountType: service.AccountTypeServiceAccount,
			BalanceCost: 0.25, AccountQuotaCost: 0.25,
		}
		// Duplicate deliveries run concurrently with other debits for the same user.
		for j := 0; j < 2; j++ {
			wg.Add(1)
			go func(local service.UsageBillingCommand) {
				defer wg.Done()
				if _, err := repo.Apply(ctx, &local); err != nil {
					errors <- fmt.Errorf("apply concurrent debit: %w", err)
				}
			}(cmd)
		}
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	var balance, used float64
	var outbox, dedup int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT (extra->>'quota_used')::numeric FROM accounts WHERE id = $1", account.ID).Scan(&used))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE api_key_id = $1", key.ID).Scan(&dedup))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM scheduler_outbox WHERE event_type = $1 AND account_id = $2", service.SchedulerOutboxEventAccountChanged, account.ID).Scan(&outbox))
	require.InDelta(t, 96.0, balance, 1e-9)
	require.InDelta(t, 4.0, used, 1e-9)
	require.Equal(t, commands, dedup)
	require.Equal(t, 1, outbox)
}
