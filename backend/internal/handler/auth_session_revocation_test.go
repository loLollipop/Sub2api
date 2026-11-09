//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestAuthHandlerRevokeAllSessionsInvalidatesAccessTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &userHandlerRepoStub{
		user: &service.User{
			ID:           29,
			Email:        "session@example.com",
			Username:     "session-user",
			Role:         service.RoleUser,
			Status:       service.StatusActive,
			TokenVersion: 7,
		},
	}
	refreshTokenCache := &userHandlerRefreshTokenCacheStub{}
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:     "test-secret",
			ExpireHour: 1,
		},
	}
	authService := service.NewAuthService(nil, repo, nil, refreshTokenCache, cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	handler := &AuthHandler{authService: authService}
	oldToken, err := authService.GenerateToken(context.Background(), repo.user)
	require.NoError(t, err)
	_, err = authService.ValidateToken(oldToken)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/revoke-all-sessions", nil)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 29})

	handler.RevokeAllSessions(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, []int64{29}, refreshTokenCache.revokedUserIDs)
	_, err = authService.ValidateToken(oldToken)
	require.ErrorIs(t, err, service.ErrTokenRevoked)
	require.Equal(t, int64(7), repo.user.TokenVersion)
	newToken, err := authService.GenerateToken(context.Background(), repo.user)
	require.NoError(t, err)
	_, err = authService.ValidateToken(newToken)
	require.NoError(t, err)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, "All sessions have been revoked. Please log in again.", resp.Data.Message)
}

func TestOAuthBindCookieRejectsRevokedAccessToken(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	repo := &userHandlerRepoStub{user: &service.User{ID: 29, Email: "bind@example.com", Role: service.RoleUser, Status: service.StatusActive}}
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "cookie-revocation-secret", ExpireHour: 168}}
	authService := service.NewAuthService(nil, repo, nil, repository.NewRefreshTokenCache(client), cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	h := &AuthHandler{authService: authService, userService: service.NewUserService(repo, nil, nil, nil)}
	token, err := authService.GenerateToken(context.Background(), repo.user)
	require.NoError(t, err)
	resolve := func(value string) (*int64, error) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/oauth/bind", nil)
		c.Request.AddCookie(&http.Cookie{Name: oauthBindAccessTokenCookieName, Value: url.QueryEscape(value)})
		return h.resolveOAuthBindTargetUserID(c)
	}
	id, err := resolve(token)
	require.NoError(t, err)
	require.Equal(t, repo.user.ID, *id)
	require.NoError(t, authService.RevokeAllUserTokens(context.Background(), repo.user.ID))
	_, err = resolve(token)
	require.ErrorIs(t, err, service.ErrTokenRevoked)
	newToken, err := authService.GenerateToken(context.Background(), repo.user)
	require.NoError(t, err)
	_, err = resolve(newToken)
	require.NoError(t, err)
}
