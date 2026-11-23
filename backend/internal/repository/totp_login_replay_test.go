//go:build unit

package repository

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestTotpLoginReplayRedisAtomicConsumptionAndExpiry(t *testing.T) {
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer rdb.Close()
	cache := NewTotpCache(rdb)
	ctx := context.Background()
	var successes atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Go(func() {
			ok, err := cache.MarkLoginCodeUsed(ctx, 7, "fingerprint", 5*time.Minute)
			require.NoError(t, err)
			if ok {
				successes.Add(1)
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 1, successes.Load())
	ok, err := cache.MarkLoginCodeUsed(ctx, 8, "fingerprint", 5*time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	server.FastForward(5 * time.Minute)
	ok, err = cache.MarkLoginCodeUsed(ctx, 7, "fingerprint", 5*time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, cache.SetLoginSession(ctx, "challenge", &service.TotpLoginSession{UserID: 7}, time.Minute))
	successes.Store(0)
	for i := 0; i < 16; i++ {
		wg.Go(func() {
			session, err := cache.ConsumeLoginSession(ctx, "challenge")
			require.NoError(t, err)
			if session != nil {
				successes.Add(1)
				require.EqualValues(t, 7, session.UserID)
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 1, successes.Load())
	server.Close()
	_, err = cache.MarkLoginCodeUsed(ctx, 7, "other", time.Minute)
	require.Error(t, err)
}
