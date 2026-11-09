//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestJWTRevocationProtectsUserAdminAndWebSocket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	user := &service.User{ID: 7, Email: "admin@example.com", Role: service.RoleAdmin, Status: service.StatusActive}
	repo := &stubJWTUserRepo{users: map[int64]*service.User{user.ID: user}}
	userService := service.NewUserService(repo, nil, nil, nil)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "middleware-revocation-secret", ExpireHour: 168}}
	authService := service.NewAuthService(nil, repo, nil, repository.NewRefreshTokenCache(client), cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	oldToken, err := authService.GenerateToken(context.Background(), user)
	require.NoError(t, err)
	router := gin.New()
	router.GET("/user", gin.HandlerFunc(NewJWTAuthMiddleware(authService, userService, nil, nil)), func(c *gin.Context) { c.Status(http.StatusOK) })
	router.GET("/admin", gin.HandlerFunc(NewAdminAuthMiddleware(authService, userService, nil, nil, cfg)), func(c *gin.Context) { c.Status(http.StatusOK) })
	check := func(token string, want int) {
		t.Helper()
		for _, tc := range []struct {
			path string
			ws   bool
		}{{"/user", false}, {"/admin", false}, {"/admin", true}} {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.ws {
				req.Header.Set("Connection", "Upgrade")
				req.Header.Set("Upgrade", "websocket")
				req.Header.Set("Sec-WebSocket-Protocol", "sub2api-admin, jwt."+token)
			} else {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			router.ServeHTTP(w, req)
			require.Equal(t, want, w.Code, "%s websocket=%v: %s", tc.path, tc.ws, w.Body.String())
		}
	}
	check(oldToken, http.StatusOK)
	require.NoError(t, authService.RevokeAllUserTokens(context.Background(), user.ID))
	check(oldToken, http.StatusUnauthorized)
	newToken, err := authService.GenerateToken(context.Background(), user)
	require.NoError(t, err)
	check(newToken, http.StatusOK)
	server.SetError("ERR cache unavailable")
	check(newToken, http.StatusUnauthorized)
}
