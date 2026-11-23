//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"
)

type loginReplayCache struct {
	TotpCache
	mu          sync.Mutex
	used        map[string]bool
	markerErr   error
	attemptsErr error
	ttl         time.Duration
}

func (c *loginReplayCache) GetVerifyAttempts(context.Context, int64) (int, error) {
	return 0, c.attemptsErr
}
func (c *loginReplayCache) IncrementVerifyAttempts(context.Context, int64) (int, error) {
	return 1, nil
}
func (c *loginReplayCache) ClearVerifyAttempts(context.Context, int64) error { return nil }
func (c *loginReplayCache) MarkLoginCodeUsed(_ context.Context, _ int64, digest string, ttl time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.markerErr != nil {
		return false, c.markerErr
	}
	c.ttl = ttl
	if c.used == nil {
		c.used = map[string]bool{}
	}
	if c.used[digest] {
		return false, nil
	}
	c.used[digest] = true
	return true, nil
}

type loginReplayEncryptor struct{}

func (loginReplayEncryptor) Encrypt(s string) (string, error) { return s, nil }
func (loginReplayEncryptor) Decrypt(s string) (string, error) { return s, nil }

func newPlainLoginTOTPService() (*TotpService, *loginReplayCache) {
	secret := "JBSWY3DPEHPK3PXP"
	cache := &loginReplayCache{}
	svc := NewTotpService(&totpVMUserRepoStub{user: &User{ID: 7, TotpEnabled: true, TotpSecretEncrypted: &secret}}, loginReplayEncryptor{}, cache, nil, nil, nil)
	return svc, cache
}
func TestLoginTOTPCodeReplayRejectedWithoutPaymentSecurity(t *testing.T) {
	svc, cache := newPlainLoginTOTPService()
	ctx := context.Background()
	code, err := totp.GenerateCode("JBSWY3DPEHPK3PXP", time.Now())
	require.NoError(t, err)
	counter, err := svc.VerifyLoginCode(ctx, 7, code)
	require.NoError(t, err)
	require.NoError(t, svc.ConsumeLoginCode(ctx, 7, counter, code))
	require.ErrorIs(t, svc.ConsumeLoginCode(ctx, 7, counter, code), ErrTotpCodeReplayed)
	require.NoError(t, svc.ConsumeLoginCode(ctx, 8, counter, code))
	require.NoError(t, svc.ConsumeLoginCode(ctx, 7, counter+1, code))
	require.GreaterOrEqual(t, cache.ttl, 90*time.Second)
	// Setup, disable, and step-up verification retain their ordinary reusable semantics.
	require.NoError(t, svc.VerifyCode(ctx, 7, code))
	require.NoError(t, svc.VerifyCode(ctx, 7, code))
}
func TestLoginTOTPReplayConsumptionIsAtomic(t *testing.T) {
	svc, _ := newPlainLoginTOTPService()
	var successes atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Go(func() {
			err := svc.ConsumeLoginCode(context.Background(), 7, 123, "123456")
			if err == nil {
				successes.Add(1)
			} else {
				require.ErrorIs(t, err, ErrTotpCodeReplayed)
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 1, successes.Load())
}
func TestLoginTOTPVerificationAndMarkerFailuresFailClosed(t *testing.T) {
	svc, cache := newPlainLoginTOTPService()
	cache.markerErr = errors.New("redis unavailable")
	require.ErrorIs(t, svc.ConsumeLoginCode(context.Background(), 7, 123, "123456"), ErrTotpUnavailable)
	cache.attemptsErr = errors.New("redis unavailable")
	_, err := svc.VerifyLoginCode(context.Background(), 7, "123456")
	require.ErrorIs(t, err, ErrTotpUnavailable)
}
func TestLoginTOTPReplayUsesMatchedCounterAcrossClockBoundary(t *testing.T) {
	at := time.Unix(1800000029, 0)
	code, err := totp.GenerateCode("JBSWY3DPEHPK3PXP", at)
	require.NoError(t, err)
	counter, valid, err := matchedLoginTotpCounter(code, "JBSWY3DPEHPK3PXP", at.Add(2*time.Second))
	require.NoError(t, err)
	require.True(t, valid)
	require.Equal(t, at.Unix()/30, counter)
	svc, _ := newPlainLoginTOTPService()
	require.NoError(t, svc.ConsumeLoginCode(context.Background(), 7, counter, code))
	same, valid, err := matchedLoginTotpCounter(code, "JBSWY3DPEHPK3PXP", at)
	require.NoError(t, err)
	require.True(t, valid)
	require.ErrorIs(t, svc.ConsumeLoginCode(context.Background(), 7, same, code), ErrTotpCodeReplayed)
}
