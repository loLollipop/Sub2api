package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type revocationUserRepo struct {
	service.UserRepository
	user *service.User
}

func (r *revocationUserRepo) GetByID(context.Context, int64) (*service.User, error) {
	return r.user, nil
}

type failingRevocationCleanup struct {
	service.RefreshTokenCache
	service.UserTokenRevocationCache
	cleanupCalled bool
	beforeStore   func()
}

func (c *failingRevocationCleanup) DeleteUserRefreshTokens(context.Context, int64) error {
	c.cleanupCalled = true
	return errors.New("cleanup failed")
}

func (c *failingRevocationCleanup) StoreRefreshToken(ctx context.Context, hash string, data *service.RefreshTokenData, ttl time.Duration) error {
	if c.beforeStore != nil {
		c.beforeStore()
	}
	return c.RefreshTokenCache.StoreRefreshToken(ctx, hash, data, ttl)
}

func TestAuthUserTokenCutoffPersistsAndRejectsStaleCredentials(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	cache := NewRefreshTokenCache(client)
	cutoffs, ok := cache.(service.UserTokenRevocationCache)
	require.True(t, ok, "production refresh cache must persist user revocation")
	failing := &failingRevocationCleanup{RefreshTokenCache: cache, UserTokenRevocationCache: cutoffs}
	user := &service.User{ID: 7, Email: "revoked@example.com", Role: service.RoleUser, Status: service.StatusActive}
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-revocation-secret", ExpireHour: 168, RefreshTokenExpireDays: 30}}
	auth := service.NewAuthService(nil, &revocationUserRepo{user: user}, nil, failing, cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	pair, err := auth.GenerateTokenPair(ctx, user, "")
	require.NoError(t, err)
	_, err = auth.ValidateToken(pair.AccessToken)
	require.NoError(t, err)
	require.NoError(t, auth.RevokeAllUserTokens(ctx, user.ID))
	require.True(t, failing.cleanupCalled)
	_, err = auth.ValidateToken(pair.AccessToken)
	require.ErrorIs(t, err, service.ErrTokenRevoked)
	_, err = auth.RefreshToken(ctx, pair.AccessToken)
	require.ErrorIs(t, err, service.ErrTokenRevoked)
	_, err = auth.RefreshTokenPair(ctx, pair.RefreshToken)
	require.ErrorIs(t, err, service.ErrTokenRevoked, "failed refresh-set deletion must not restore a revoked session")
	// Pre-upgrade refresh records have no epoch; their CreatedAt must still be
	// checked when the user-token set was missing or could not be deleted.
	legacyToken := "rt_legacy-before-revocation"
	legacyHash := sha256.Sum256([]byte(legacyToken))
	require.NoError(t, cache.StoreRefreshToken(ctx, hex.EncodeToString(legacyHash[:]), &service.RefreshTokenData{
		UserID: user.ID, CreatedAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour),
	}, time.Hour))
	_, err = auth.RefreshTokenPair(ctx, legacyToken)
	require.ErrorIs(t, err, service.ErrTokenRevoked)

	server.FastForward(49 * time.Hour)
	require.True(t, server.Exists(userTokenCutoffPrefix+"7"))
	require.Equal(t, time.Duration(0), server.TTL(userTokenCutoffPrefix+"7"), "cutoffs must not expire after a fixed 48h")
	_, err = auth.ValidateToken(pair.AccessToken)
	require.ErrorIs(t, err, service.ErrTokenRevoked)
	// Construct a new service to prove revocation does not depend on local state.
	auth = service.NewAuthService(nil, &revocationUserRepo{user: user}, nil, cache, cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	_, err = auth.ValidateToken(pair.AccessToken)
	require.ErrorIs(t, err, service.ErrTokenRevoked)

	newPair, err := auth.GenerateTokenPair(ctx, user, "")
	require.NoError(t, err)
	_, err = auth.ValidateToken(newPair.AccessToken)
	require.NoError(t, err)
	_, err = auth.RefreshTokenPair(ctx, newPair.RefreshToken)
	require.NoError(t, err)

	// An unavailable revocation cache must never authenticate a signed JWT.
	server.SetError("ERR cache unavailable")
	_, err = auth.ValidateToken(newPair.AccessToken)
	require.ErrorIs(t, err, service.ErrServiceUnavailable)
	require.Error(t, auth.RevokeAllUserTokens(ctx, user.ID))
}

func TestAuthUserTokenCutoffSameSecondAndMonotonic(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	cache := NewRefreshTokenCache(client)
	cutoffs, ok := cache.(service.UserTokenRevocationCache)
	require.True(t, ok, "production refresh cache must persist user revocation")
	base := time.Now().Truncate(time.Second)
	cutoff := base.Add(500 * time.Millisecond)
	require.NoError(t, cutoffs.SetUserTokenCutoff(ctx, 7, cutoff))
	require.NoError(t, cutoffs.SetUserTokenCutoff(ctx, 7, cutoff.Add(-time.Millisecond)))
	stored, err := cutoffs.GetUserTokenCutoff(ctx, 7)
	require.NoError(t, err)
	require.True(t, cutoff.Add(time.Nanosecond).Equal(stored))
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "same-second-secret", ExpireHour: 168}}
	auth := service.NewAuthService(nil, nil, nil, cache, cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	for _, tc := range []struct {
		name  string
		epoch *int64
		want  error
	}{
		{"legacy_second_precision", nil, service.ErrTokenRevoked},
		{"before_first_cutoff", new(int64), service.ErrTokenRevoked},
		{"previous_cutoff", new(cutoff.UnixNano()), service.ErrTokenRevoked},
		{"new_login_same_clock_tick", new(stored.UnixNano()), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := &service.JWTClaims{UserID: 7, RevocationCutoff: tc.epoch, RegisteredClaims: jwt.RegisteredClaims{
				IssuedAt: jwt.NewNumericDate(base), ExpiresAt: jwt.NewNumericDate(base.Add(168 * time.Hour)),
			}}
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(cfg.JWT.Secret))
			require.NoError(t, err)
			_, err = auth.ValidateToken(token)
			require.ErrorIs(t, err, tc.want)
		})
	}
	expiredClaims := &service.JWTClaims{UserID: 7, RegisteredClaims: jwt.RegisteredClaims{
		IssuedAt: jwt.NewNumericDate(base.Add(-2 * time.Hour)), ExpiresAt: jwt.NewNumericDate(base.Add(-time.Hour)),
	}}
	expiredToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, expiredClaims).SignedString([]byte(cfg.JWT.Secret))
	require.NoError(t, err)
	_, err = auth.RefreshToken(ctx, expiredToken)
	require.ErrorIs(t, err, service.ErrTokenRevoked, "expired legacy JWTs must not bypass the cutoff during refresh")
	user := &service.User{ID: 7, Email: "same-tick@example.com"}
	token, err := auth.GenerateToken(ctx, user)
	require.NoError(t, err)
	_, err = auth.ValidateToken(token)
	require.NoError(t, err)
	require.NoError(t, cutoffs.SetUserTokenCutoff(ctx, 7, stored))
	_, err = auth.ValidateToken(token)
	require.ErrorIs(t, err, service.ErrTokenRevoked, "repeated revocations at identical timestamps must advance the epoch")
	newToken, err := auth.GenerateToken(ctx, user)
	require.NoError(t, err)
	_, err = auth.ValidateToken(newToken)
	require.NoError(t, err)
}

func TestAuthUserTokenCutoffDuringRefreshRotation(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	cache := NewRefreshTokenCache(client)
	cutoffs, ok := cache.(service.UserTokenRevocationCache)
	require.True(t, ok, "production refresh cache must persist user revocation")
	failing := &failingRevocationCleanup{RefreshTokenCache: cache, UserTokenRevocationCache: cutoffs}
	user := &service.User{ID: 7, Email: "race@example.com", Role: service.RoleUser, Status: service.StatusActive}
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "refresh-race-secret", ExpireHour: 168, RefreshTokenExpireDays: 30}}
	auth := service.NewAuthService(nil, &revocationUserRepo{user: user}, nil, failing, cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	pair, err := auth.GenerateTokenPair(ctx, user, "")
	require.NoError(t, err)
	failing.beforeStore = func() { require.NoError(t, auth.RevokeAllUserTokens(ctx, user.ID)) }
	_, err = auth.RefreshTokenPair(ctx, pair.RefreshToken)
	require.ErrorIs(t, err, service.ErrTokenRevoked)
}
