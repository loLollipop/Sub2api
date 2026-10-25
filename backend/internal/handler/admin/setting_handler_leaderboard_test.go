//go:build unit

package admin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	publichandler "github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestLeaderboardSettingsPartialUpdateAndPublicProjection(t *testing.T) {
	repo := &purchaseSettingsRepo{values: map[string]string{service.SettingKeyLeaderboardEnabled: "true", service.SettingKeyLeaderboardRequireAuth: "true"}}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	h := admin.NewSettingHandler(svc, nil, nil, nil, nil)
	rec := purchaseSettingsRequest(t, h.UpdateSettings, http.MethodPut, map[string]any{"leaderboard_enabled": false})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "false", repo.values[service.SettingKeyLeaderboardEnabled])
	require.Equal(t, "true", repo.values[service.SettingKeyLeaderboardRequireAuth])
	rec = purchaseSettingsRequest(t, h.UpdateSettings, http.MethodPut, map[string]any{"leaderboard_enabled": true, "leaderboard_require_auth": false})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "true", repo.values[service.SettingKeyLeaderboardEnabled])
	require.Equal(t, "false", repo.values[service.SettingKeyLeaderboardRequireAuth])
	for _, endpoint := range []gin.HandlerFunc{h.GetSettings, publichandler.NewSettingHandler(svc, "test").GetPublicSettings} {
		rec = purchaseSettingsRequest(t, endpoint, http.MethodGet, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var body struct {
			Data struct {
				Enabled     bool `json:"leaderboard_enabled"`
				RequireAuth bool `json:"leaderboard_require_auth"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		require.True(t, body.Data.Enabled)
		require.False(t, body.Data.RequireAuth)
	}
	injected, err := svc.GetPublicSettingsForInjection(context.Background())
	require.NoError(t, err)
	require.True(t, injected.(*service.PublicSettingsInjectionPayload).LeaderboardEnabled)
	require.False(t, injected.(*service.PublicSettingsInjectionPayload).LeaderboardRequireAuth)
}
