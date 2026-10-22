//go:build unit

package service

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidateCreateAPIKeyRequestNumericLimits(t *testing.T) {
	positiveExpiry := 1
	require.NoError(t, validateCreateAPIKeyRequest(CreateAPIKeyRequest{
		Quota: 1e100, RateLimit5h: 1e100, ExpiresInDays: &positiveExpiry,
	}))
	require.NoError(t, validateCreateAPIKeyRequest(CreateAPIKeyRequest{}))

	invalidExpiry := 0
	tests := []CreateAPIKeyRequest{
		{Quota: -1},
		{Quota: math.NaN()},
		{Quota: math.Inf(1)},
		{RateLimit5h: -1},
		{RateLimit1d: math.NaN()},
		{RateLimit7d: math.Inf(-1)},
		{ExpiresInDays: &invalidExpiry},
	}
	for _, req := range tests {
		require.Error(t, validateCreateAPIKeyRequest(req))
	}
}

func TestValidateUpdateAPIKeyRequestNumericLimits(t *testing.T) {
	zero, large, negative, nan, inf := 0.0, 1e100, -1.0, math.NaN(), math.Inf(1)
	require.NoError(t, validateUpdateAPIKeyRequest(UpdateAPIKeyRequest{Quota: &zero, RateLimit7d: &large}))

	for _, req := range []UpdateAPIKeyRequest{
		{Quota: &negative},
		{RateLimit5h: &nan},
		{RateLimit1d: &inf},
		{RateLimit7d: &negative},
	} {
		require.Error(t, validateUpdateAPIKeyRequest(req))
	}
}

func TestAPIKeyServiceCreateAbsoluteExpiry(t *testing.T) {
	expiry := time.Now().Add(30 * time.Minute).Truncate(time.Millisecond)
	repo, cache := newCreateLimitStubs()
	svc := newCreateLimitService(repo, cache, 0, 0)
	key, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "short lived", ExpiresAt: &expiry})
	require.NoError(t, err)
	require.Equal(t, expiry, *key.ExpiresAt)
	require.Equal(t, expiry, *repo.created[0].ExpiresAt)
}

func TestAPIKeyServiceCreateRejectsInvalidAbsoluteExpiryBeforeWrite(t *testing.T) {
	future := time.Now().Add(30 * time.Minute)
	past := time.Now().Add(-time.Minute)
	zero := time.Time{}
	days := 1
	for _, req := range []CreateAPIKeyRequest{
		{ExpiresAt: &past}, {ExpiresAt: &zero}, {ExpiresAt: &future, ExpiresInDays: &days},
	} {
		repo, cache := newCreateLimitStubs()
		svc := newCreateLimitService(repo, cache, 0, 0)
		_, err := svc.Create(context.Background(), 7, req)
		require.Error(t, err)
		require.Empty(t, repo.created)
	}
}

func TestAPIKeyServiceCreateRetainsRelativeExpiry(t *testing.T) {
	repo, cache := newCreateLimitStubs()
	svc := newCreateLimitService(repo, cache, 0, 0)
	days := 7
	before := time.Now().AddDate(0, 0, days)
	key, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "legacy", ExpiresInDays: &days})
	require.NoError(t, err)
	require.WithinDuration(t, before, *key.ExpiresAt, time.Second)
}
