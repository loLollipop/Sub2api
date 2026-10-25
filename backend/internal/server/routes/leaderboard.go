package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func RegisterLeaderboardRoutes(v1 *gin.RouterGroup, h *handler.Handlers, optionalJWT middleware.OptionalJWTAuthMiddleware, settings *service.SettingService, limiter *middleware.PanelRateLimiter) {
	leaderboard := v1.Group("/leaderboard")
	leaderboard.Use(limiter.PublicIP())
	leaderboard.Use(gin.HandlerFunc(optionalJWT))
	leaderboard.Use(middleware.BackendModeUserGuard(settings))
	leaderboard.GET("", h.Leaderboard.Get)
}
