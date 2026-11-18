package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayRoutesRequestBodyMemoryBudget(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{
		MaxBodySize: 1024, TextMaxBodySize: 512, RequestBodyMemoryBudgetBytes: 8,
	}}
	for _, tc := range []struct{ path, platform string }{
		{"/v1/responses", service.PlatformOpenAI},
		{"/responses", service.PlatformOpenAI},
		{"/backend-api/codex/responses", service.PlatformOpenAI},
		{"/v1/messages", service.PlatformAnthropic},
		{"/v1/embeddings", service.PlatformOpenAI},
		{"/v1/systemone", service.PlatformTypeSafe},
	} {
		t.Run(tc.path, func(t *testing.T) {
			router := newGatewayRoutesTestRouterWithConfig(cfg, tc.platform)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(`{"model":"example"}`))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
			require.Equal(t, "1", rec.Header().Get("Retry-After"))
		})
	}
}

func TestGrokVideosGateRequestBodyMemoryBudget(t *testing.T) {
	router := gin.New()
	router.POST("/v1/videos", middleware.RequestBodyLimit(1024, pkghttputil.NewBodyMemoryBudget(8)), func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
			Group: &service.Group{Platform: service.PlatformOpenAI}, RouteGroupIDs: []int64{1, 2},
		})
		require.False(t, grokVideosAPIAllowed(c))
		require.True(t, c.IsAborted(), "caller must preserve the body-read response")
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"grok-imagine-video"}`)))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
	require.Equal(t, "1", rec.Header().Get("Retry-After"))
}
