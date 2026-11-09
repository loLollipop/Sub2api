package repository

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type notifyVerificationUserRepo struct {
	service.UserRepository
	reads   atomic.Int32
	updates atomic.Int32
}

func (r *notifyVerificationUserRepo) GetByID(context.Context, int64) (*service.User, error) {
	r.reads.Add(1)
	return &service.User{ID: 7, Email: "primary@example.com"}, nil
}

func (r *notifyVerificationUserRepo) Update(_ context.Context, user *service.User, fields service.UserUpdateFields) error {
	r.updates.Add(1)
	return nil
}

func TestEmailCodeServicesUseAtomicConsumption(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewEmailCache(client)
	ctx := context.Background()
	email := "verified@example.com"
	data := &service.VerificationCodeData{Code: "123456", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute)}
	emailService := service.NewEmailService(nil, cache)
	require.NoError(t, cache.SetVerificationCode(ctx, email, data, time.Minute))
	var successes atomic.Int32
	runConcurrentCodeChecks(30, func() {
		if emailService.VerifyCode(ctx, email, "123456") == nil {
			successes.Add(1)
		}
	})
	require.Equal(t, int32(1), successes.Load())

	repo := &notifyVerificationUserRepo{}
	userService := service.NewUserService(repo, nil, nil, nil)
	require.NoError(t, cache.SetNotifyVerifyCode(ctx, email, data, time.Minute))
	successes.Store(0)
	runConcurrentCodeChecks(30, func() {
		if userService.VerifyAndAddNotifyEmail(ctx, 7, email, "123456", cache) == nil {
			successes.Add(1)
		}
	})
	require.Equal(t, int32(1), successes.Load())
	require.Equal(t, int32(1), repo.reads.Load(), "reused codes must not reach user mutation")
	require.Equal(t, int32(1), repo.updates.Load())

	server.SetError("ERR cache unavailable")
	require.Error(t, emailService.VerifyCode(ctx, email, "123456"))
	require.Error(t, userService.VerifyAndAddNotifyEmail(ctx, 7, email, "123456", cache))
	require.Equal(t, int32(1), repo.updates.Load(), "cache errors must fail closed")
}
