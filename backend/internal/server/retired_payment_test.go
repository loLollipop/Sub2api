package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestNativePaymentRoutesRemoved(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	pass := func(c *gin.Context) { c.Next() }
	h := &handler.Handlers{Admin: &handler.AdminHandlers{}}
	registerRoutes(router, h, middleware.JWTAuthMiddleware(pass), middleware.OptionalJWTAuthMiddleware(pass), middleware.AdminAuthMiddleware(pass), middleware.APIKeyAuthMiddleware(pass), middleware.AuditLogMiddleware(pass), middleware.StepUpAuthMiddleware(pass), nil, nil, nil, nil, nil, &config.Config{}, nil, nil)
	for _, route := range router.Routes() {
		require.False(t, strings.HasPrefix(route.Path, "/api/v1/payment/"), route.Path)
		require.False(t, strings.HasPrefix(route.Path, "/api/v1/admin/payment/"), route.Path)
		require.NotContains(t, route.Path, "/auth/oauth/wechat/payment/")
	}
	for _, path := range []string{
		"/api/v1/payment/config", "/api/v1/payment/checkout-info", "/api/v1/payment/orders",
		"/api/v1/payment/orders/my", "/api/v1/payment/public/orders/verify", "/api/v1/payment/public/orders/resolve",
		"/api/v1/payment/webhook/easypay", "/api/v1/payment/webhook/alipay", "/api/v1/payment/webhook/wxpay",
		"/api/v1/payment/webhook/stripe", "/api/v1/payment/webhook/airwallex", "/api/v1/payment/webhook/epusdt",
		"/api/v1/payments/callback", "/api/v1/admin/payment/config", "/api/v1/admin/payment/dashboard",
		"/api/v1/admin/payment/orders", "/api/v1/admin/payment/plans", "/api/v1/admin/payment/providers",
		"/api/v1/auth/oauth/wechat/payment/start", "/api/v1/auth/oauth/wechat/payment/callback",
	} {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
			t.Run(method+path, func(t *testing.T) {
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
				require.Equal(t, http.StatusNotFound, rec.Code)
			})
		}
	}
	// The retained shop fulfills purchases through ordinary code redemption.
	routes := router.Routes()
	for _, path := range []string{"/api/v1/redeem", "/api/v1/admin/redeem-codes/create-and-redeem", "/api/v1/subscriptions", "/api/v1/auth/oauth/wechat/start"} {
		found := false
		for _, route := range routes {
			if route.Path == path {
				found = true
				break
			}
		}
		require.True(t, found, path)
	}
}
