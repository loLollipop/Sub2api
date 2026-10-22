package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func retiredFeatureRouteEnvironment(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	h := &handler.Handlers{Admin: &handler.AdminHandlers{}}
	pass := func(c *gin.Context) { c.Next() }
	audit := middleware.AuditLogMiddleware(pass)
	RegisterUserRoutes(v1, h, middleware.JWTAuthMiddleware(pass), audit, nil, nil)
	RegisterAdminRoutes(v1, h, middleware.AdminAuthMiddleware(pass), audit, middleware.StepUpAuthMiddleware(pass), nil, nil)
	return router
}

func retiredFeatureRouteRequest(router *gin.Engine, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

func TestSupportTicketRoutesRemoved(t *testing.T) {
	router := retiredFeatureRouteEnvironment(t)
	for _, prefix := range []string{"/api/v1/tickets", "/api/v1/admin/tickets"} {
		for _, suffix := range []string{"", "/stats", "/1", "/1/read", "/1/replies"} {
			for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPatch} {
				t.Run(method+prefix+suffix, func(t *testing.T) {
					require.Equal(t, http.StatusNotFound, retiredFeatureRouteRequest(router, method, prefix+suffix).Code)
				})
			}
		}
	}
}
