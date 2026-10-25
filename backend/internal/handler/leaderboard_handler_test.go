//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type leaderboardSettingRepo struct {
	service.SettingRepository
	values map[string]string
	err    error
	reads  int
}

func (r *leaderboardSettingRepo) GetLeaderboardAccessSettings(context.Context) (map[string]string, error) {
	r.reads++
	return r.values, r.err
}

type leaderboardUsageReader struct{ calls int }

func (r *leaderboardUsageReader) GetLeaderboardUsage(_ context.Context, start, end time.Time, granularity string) ([]service.LeaderboardUsageBucket, error) {
	r.calls++
	return []service.LeaderboardUsageBucket{{Date: start, Model: "public-requested-model", TotalTokens: 42, Requests: 2}}, nil
}

func leaderboardRequest(h *LeaderboardHandler, query string, authenticated bool) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/leaderboard"+query, nil)
	if authenticated {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
	}
	h.Get(c)
	return w
}

func TestLeaderboardHandlerAccessCheckedBeforeEveryCachedResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &leaderboardSettingRepo{values: map[string]string{service.SettingKeyLeaderboardEnabled: "true"}}
	reader := &leaderboardUsageReader{}
	h := NewLeaderboardHandler(service.NewLeaderboardService(reader), service.NewSettingService(repo, &config.Config{}))
	w := leaderboardRequest(h, "?period=week&timezone=Pacific/Honolulu", false)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var body struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data, 12)
	require.Contains(t, string(body.Data["models"]), `"model":"public-requested-model"`)
	require.NotContains(t, w.Body.String(), "user_id")
	require.NotContains(t, w.Body.String(), "upstream_model")
	require.NotContains(t, w.Body.String(), "actual_cost")
	require.Equal(t, 2, reader.calls)
	repo.values[service.SettingKeyLeaderboardRequireAuth] = "true"
	require.Equal(t, http.StatusUnauthorized, leaderboardRequest(h, "", false).Code)
	require.Equal(t, http.StatusOK, leaderboardRequest(h, "", true).Code)
	require.Equal(t, 2, reader.calls)
	repo.values[service.SettingKeyLeaderboardEnabled] = "false"
	require.Equal(t, http.StatusNotFound, leaderboardRequest(h, "", true).Code)
	repo.values[service.SettingKeyLeaderboardEnabled] = "true"
	repo.err = errors.New("settings offline")
	require.Equal(t, http.StatusNotFound, leaderboardRequest(h, "", true).Code)
	require.Equal(t, 5, repo.reads)
	require.Equal(t, 2, reader.calls)
}

func TestLeaderboardHandlerDisabledMissingAndInvalidPeriod(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.Equal(t, http.StatusNotFound, leaderboardRequest(&LeaderboardHandler{}, "", false).Code)
	repo := &leaderboardSettingRepo{values: map[string]string{}}
	h := NewLeaderboardHandler(service.NewLeaderboardService(nil), service.NewSettingService(repo, &config.Config{}))
	require.Equal(t, http.StatusNotFound, leaderboardRequest(h, "?period=invalid", false).Code)
	repo.values[service.SettingKeyLeaderboardEnabled] = "true"
	require.Equal(t, http.StatusBadRequest, leaderboardRequest(h, "?period=invalid", false).Code)
}
