package handler

import (
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type LeaderboardHandler struct {
	leaderboardService *service.LeaderboardService
	settingService     *service.SettingService
}

func NewLeaderboardHandler(leaderboardService *service.LeaderboardService, settingService *service.SettingService) *LeaderboardHandler {
	return &LeaderboardHandler{leaderboardService: leaderboardService, settingService: settingService}
}

func (h *LeaderboardHandler) Get(c *gin.Context) {
	// Public proxies must not retain data after a feature/auth switch changes.
	c.Header("Cache-Control", "no-store")
	if h.settingService == nil {
		response.NotFound(c, "Leaderboard is not enabled")
		return
	}
	runtime := h.settingService.GetLeaderboardRuntime(c.Request.Context())
	if !runtime.Enabled {
		response.NotFound(c, "Leaderboard is not enabled")
		return
	}
	if _, authed := middleware.GetAuthSubjectFromContext(c); runtime.RequireAuth && !authed {
		response.Unauthorized(c, "Authentication required")
		return
	}
	data, err := h.leaderboardService.Get(c.Request.Context(), c.Query("period"))
	if errors.Is(err, service.ErrLeaderboardPeriod) {
		response.BadRequest(c, err.Error())
		return
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, data)
}
