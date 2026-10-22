//go:build unit

package handler

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidateAPIKeyCreateRequest(t *testing.T) {
	zero, large, negative, nan, inf := 0.0, 1e100, -1.0, math.NaN(), math.Inf(1)
	positiveDays, zeroDays, negativeDays := 1, 0, -1
	require.NoError(t, validateAPIKeyCreateRequest(CreateAPIKeyRequest{}))
	require.NoError(t, validateAPIKeyCreateRequest(CreateAPIKeyRequest{Quota: &zero, RateLimit5h: &large, ExpiresInDays: &positiveDays}))

	for _, req := range []CreateAPIKeyRequest{
		{Quota: &negative},
		{Quota: &nan},
		{RateLimit5h: &inf},
		{RateLimit1d: &negative},
		{RateLimit7d: &negative},
		{ExpiresInDays: &zeroDays},
		{ExpiresInDays: &negativeDays},
	} {
		require.Error(t, validateAPIKeyCreateRequest(req))
	}
}

func TestValidateAPIKeyUpdateRequest(t *testing.T) {
	zero, large, negative, nan, inf := 0.0, 1e100, -1.0, math.NaN(), math.Inf(-1)
	require.NoError(t, validateAPIKeyUpdateRequest(UpdateAPIKeyRequest{Quota: &zero, RateLimit7d: &large}))

	for _, req := range []UpdateAPIKeyRequest{
		{Quota: &negative},
		{RateLimit5h: &nan},
		{RateLimit1d: &inf},
		{RateLimit7d: &negative},
	} {
		require.Error(t, validateAPIKeyUpdateRequest(req))
	}
}

func TestValidateAPIKeyCreateRequestAbsoluteExpiry(t *testing.T) {
	future := time.Now().Add(30 * time.Minute)
	past := time.Now().Add(-time.Minute)
	zero := time.Time{}
	days := 1
	require.NoError(t, validateAPIKeyCreateRequest(CreateAPIKeyRequest{ExpiresAt: &future}))
	for _, req := range []CreateAPIKeyRequest{
		{ExpiresAt: &past}, {ExpiresAt: &zero}, {ExpiresAt: &future, ExpiresInDays: &days},
	} {
		require.Error(t, validateAPIKeyCreateRequest(req))
	}
}
