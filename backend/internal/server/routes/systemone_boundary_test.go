package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSystemOneForeignProtocolBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, composite := range []bool{false, true} {
		for _, path := range []string{"/v1/messages", "/v1/messages/count_tokens", "/v1/chat/completions", "/v1/responses", "/responses", "/v1/images/generations", "/v1/audio/transcriptions", "/v1beta/models/jev-latest:generateContent"} {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", path, nil)
			platform := service.PlatformTypeSafe
			if composite {
				platform = service.PlatformComposite
				c.Request = c.Request.WithContext(service.WithResolvedTargetPlatform(context.Background(), service.PlatformTypeSafe))
			}
			require.True(t, rejectNonNativeSystemOne(c, &service.APIKey{Group: &service.Group{Platform: platform}}), path)
			require.Equal(t, 404, w.Code)
		}
	}
}

func TestSystemOneGeminiMiddlewareBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name     string
		platform string
		smart    bool
		model    string
		route    *service.CompositeModelRoute
		wantCode int
	}{
		{name: "direct-native", platform: service.PlatformTypeSafe, model: "jev-latest", wantCode: http.StatusNotFound},
		{name: "composite-detector", platform: service.PlatformComposite, model: "jev-latest", wantCode: http.StatusNotFound},
		{name: "composite-native-alias", platform: service.PlatformComposite, model: "native-alias", route: &service.CompositeModelRoute{TargetPlatform: service.PlatformTypeSafe, UpstreamModel: "jev-latest"}, wantCode: http.StatusNotFound},
		{name: "smart-native", platform: service.PlatformAnthropic, smart: true, model: "jev-latest", wantCode: http.StatusNotFound},
		{name: "smart-prefixed-native", platform: service.PlatformOpenAI, smart: true, model: "models/jev-latest", wantCode: http.StatusNotFound},
		{name: "smart-gemini", platform: service.PlatformAnthropic, smart: true, model: "gemini-2.5-pro", wantCode: http.StatusNoContent},
		{name: "smart-native-primary-gemini", platform: service.PlatformTypeSafe, smart: true, model: "gemini-2.5-pro", wantCode: http.StatusNoContent},
		{name: "smart-native-primary-native", platform: service.PlatformTypeSafe, smart: true, model: "jev-latest", wantCode: http.StatusNotFound},
		{name: "composite-gemini", platform: service.PlatformComposite, model: "gemini-2.5-pro", wantCode: http.StatusNoContent},
		{name: "explicit-gemini-alias", platform: service.PlatformComposite, model: "jev-latest", route: &service.CompositeModelRoute{TargetPlatform: service.PlatformGemini, UpstreamModel: "gemini-2.5-pro"}, wantCode: http.StatusNoContent},
	} {
		for _, detail := range []bool{false, true} {
			name := tc.name + "/generate"
			if detail {
				name = tc.name + "/detail"
			}
			t.Run(name, func(t *testing.T) {
				groupID := int64(1)
				key := &service.APIKey{GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: tc.platform}}
				if tc.smart {
					key.RouteGroupIDs = []int64{1, 2}
				}
				var routes []service.CompositeModelRoute
				if tc.route != nil {
					route := *tc.route
					route.ID, route.GroupID = 1, groupID
					route.PublicModel, route.MatchType = tc.model, service.CompositeRouteMatchExact
					route.Endpoint, route.Enabled = service.CompositeRouteEndpointGemini, true
					routes = append(routes, route)
				}
				router := gin.New()
				router.Use(func(c *gin.Context) {
					c.Set(string(servermiddleware.ContextKeyAPIKey), key)
					c.Next()
				})
				router.Use(compositeGeminiTargetPlatformMiddleware(service.NewCompositeRouteResolver(compositeRouteRepoStub{routes: routes})))
				reachedHandler := false
				handler := func(c *gin.Context) {
					reachedHandler = true
					platform, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context())
					require.True(t, ok)
					require.Equal(t, service.PlatformGemini, platform)
					if tc.route != nil {
						upstream, ok := service.ResolvedUpstreamModelFromContext(c.Request.Context())
						require.True(t, ok)
						require.Equal(t, tc.route.UpstreamModel, upstream)
					}
					c.Status(http.StatusNoContent)
				}
				method, path := http.MethodPost, "/v1beta/models/"+tc.model+":generateContent"
				if detail {
					method, path = http.MethodGet, "/v1beta/models/"+tc.model
					router.GET("/v1beta/models/*modelAction", handler)
				} else {
					router.POST("/v1beta/models/*modelAction", handler)
				}
				w := httptest.NewRecorder()
				req := httptest.NewRequest(method, path, strings.NewReader(`{"contents":[]}`))
				req.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(w, req)
				require.Equal(t, tc.wantCode, w.Code, w.Body.String())
				require.Equal(t, tc.wantCode == http.StatusNoContent, reachedHandler)
			})
		}
	}
}

func TestSystemOneSmartPrimaryWaitsForResolvedTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name     string
		smart    bool
		platform string
		resolved bool
		wantCode int
	}{
		{name: "smart-openai", smart: true, platform: service.PlatformOpenAI, resolved: true, wantCode: http.StatusNoContent},
		{name: "smart-native", smart: true, platform: service.PlatformTypeSafe, resolved: true, wantCode: http.StatusNotFound},
		{name: "smart-unresolved", smart: true, wantCode: http.StatusNotFound},
		{name: "single-native", platform: service.PlatformOpenAI, resolved: true, wantCode: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			groupID := int64(1)
			key := &service.APIKey{GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: service.PlatformTypeSafe}}
			if tc.smart {
				key.RouteGroupIDs = []int64{1, 2}
			}
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set(string(servermiddleware.ContextKeyAPIKey), key); c.Next() })
			router.Use(compositeTargetPlatformMiddlewareResolved(nil, stubUpstreamResolver{platform: tc.platform, ok: tc.resolved}))
			reached := false
			router.POST("/v1/chat/completions", func(c *gin.Context) {
				reached = true
				require.Equal(t, service.PlatformOpenAI, getGroupPlatform(c))
				c.Status(http.StatusNoContent)
			})
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.4","messages":[]}`)))
			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			require.Equal(t, tc.wantCode == http.StatusNoContent, reached)
		})
	}
}
