//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIResponsesWebSocket_RejectsCompositeOAuthAmbiguity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newOpenAIHandlerForPreviousResponseIDValidation(t, &concurrencyCacheMock{})
	refusalHandler, _, _, _ := newRefusalHandler(t, nil)
	h.gatewayService = refusalHandler.gatewayService
	h.compositeResolver = service.NewCompositeRouteResolver(nil)
	h.compositeResolver.SetModelOwnershipResolver(func(context.Context, int64, string) (service.CompositeModelOwnership, error) {
		return service.CompositeModelOwnership{Ambiguous: true, OpenAIGatewayOAuthClaimed: true}, nil
	})
	groupID := int64(2)
	router := gin.New()
	router.GET("/v1/responses", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 101, GroupID: &groupID,
			Group: &service.Group{ID: groupID, Platform: service.PlatformComposite}, User: &service.User{ID: 1}})
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1, Concurrency: 1})
		h.ResponsesWebSocket(c)
	})
	server := httptest.NewServer(router)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, resp, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/v1/responses", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	defer conn.CloseNow()
	require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-6-astra","input":"hello"}`)))
	_, _, err = conn.Read(ctx)
	require.Equal(t, coderws.StatusPolicyViolation, coderws.CloseStatus(err))
	require.Contains(t, err.Error(), "configure an explicit composite route")
}
