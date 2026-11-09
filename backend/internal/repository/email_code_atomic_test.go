package repository

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestEmailCodeAtomicConsumption(t *testing.T) {
	for _, notify := range []bool{false, true} {
		name := "registration"
		if notify {
			name = "notification"
		}
		t.Run(name, func(t *testing.T) {
			server := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: server.Addr()})
			t.Cleanup(func() { _ = client.Close() })
			cache := NewEmailCache(client)
			ctx := context.Background()
			email := "User@example.com"
			set, get, consume := cache.SetVerificationCode, cache.GetVerificationCode, cache.ConsumeVerificationCode
			key := verifyCodeKey(email)
			if notify {
				set, get, consume = cache.SetNotifyVerifyCode, cache.GetNotifyVerifyCode, cache.ConsumeNotifyVerifyCode
				key = notifyVerifyKey(email)
			}
			data := &service.VerificationCodeData{Code: "123456", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(15 * time.Minute)}
			require.NoError(t, set(ctx, email, data, 15*time.Minute))
			server.FastForward(4 * time.Minute)
			var invalid, limited atomic.Int32
			runConcurrentCodeChecks(40, func() {
				err := consume(ctx, "user@EXAMPLE.com", "wrong", 5)
				if errors.Is(err, service.ErrInvalidVerifyCode) {
					invalid.Add(1)
				} else if errors.Is(err, service.ErrVerifyCodeMaxAttempts) {
					limited.Add(1)
				} else {
					t.Errorf("unexpected verification error: %v", err)
				}
			})
			require.Equal(t, int32(4), invalid.Load())
			require.Equal(t, int32(36), limited.Load())
			stored, err := get(ctx, email)
			require.NoError(t, err)
			require.Equal(t, 5, stored.Attempts)
			require.Equal(t, 11*time.Minute, server.TTL(key), "attempt updates must preserve TTL")
			require.ErrorIs(t, consume(ctx, email, "123456", 5), service.ErrVerifyCodeMaxAttempts)

			require.NoError(t, set(ctx, email, data, time.Minute))
			var successes atomic.Int32
			runConcurrentCodeChecks(40, func() {
				err := consume(ctx, email, "123456", 5)
				if err == nil {
					successes.Add(1)
				} else if !errors.Is(err, service.ErrInvalidVerifyCode) {
					t.Errorf("unexpected verification error: %v", err)
				}
			})
			require.Equal(t, int32(1), successes.Load())
			require.False(t, server.Exists(key))

			require.NoError(t, set(ctx, email, data, time.Second))
			server.FastForward(time.Second)
			require.ErrorIs(t, consume(ctx, email, "123456", 5), service.ErrInvalidVerifyCode)
			server.SetError("ERR cache unavailable")
			require.Error(t, consume(ctx, email, "123456", 5))
		})
	}
}

func runConcurrentCodeChecks(count int, check func()) {
	var wg sync.WaitGroup
	wg.Add(count)
	start := make(chan struct{})
	for range count {
		go func() {
			defer wg.Done()
			<-start
			check()
		}()
	}
	close(start)
	wg.Wait()
}
