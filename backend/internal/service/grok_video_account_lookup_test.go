//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type grokVideoLookupAccountRepo struct {
	AccountRepository
	account *Account
	err     error
}

func (r grokVideoLookupAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, r.err
}

func TestGrokVideoDurableAccountLookupHonorsCurrentSourceOverCreateTimeCache(t *testing.T) {
	for index, scenario := range []string{"removed", "platform changed", "temporary failure", "rotated credentials"} {
		t.Run(scenario, func(t *testing.T) {
			id := int64(9823400 + index)
			stale := &Account{ID: id, Platform: PlatformGrok, Credentials: map[string]any{"api_key": "old-synthetic-key"}}
			rememberGrokVideoAccount(24, 41, 51, scenario, stale, time.Hour)
			t.Cleanup(func() { grokVideoAccountsByID.Delete(id) })
			current := *stale
			current.Credentials = map[string]any{"api_key": "current-synthetic-key"}
			repo := grokVideoLookupAccountRepo{account: &current}
			outage := errors.New("synthetic repository outage")
			switch scenario {
			case "removed":
				repo.account, repo.err = nil, ErrAccountNotFound
			case "platform changed":
				current.Platform = PlatformOpenAI
			case "temporary failure":
				repo.account, repo.err = nil, outage
			}
			// An invalidation may still be in flight on a warm instance. The
			// published scheduler account must not override current DB state.
			svc := &OpenAIGatewayService{
				accountRepo: repo,
				schedulerSnapshot: &SchedulerSnapshotService{cache: &snapshotHydrationCache{
					accounts: map[int64]*Account{id: stale},
				}},
			}
			account, err := svc.GetGrokMediaBoundAccount(context.Background(), id)
			switch scenario {
			case "removed", "platform changed":
				require.ErrorIs(t, err, ErrGrokVideoSourceUnavailable)
				require.Nil(t, account)
			case "temporary failure":
				require.ErrorIs(t, err, outage)
				require.NotErrorIs(t, err, ErrGrokVideoSourceUnavailable)
				require.Nil(t, account)
			default:
				require.NoError(t, err)
				require.Equal(t, "current-synthetic-key", account.Credentials["api_key"])
			}
		})
	}
}
