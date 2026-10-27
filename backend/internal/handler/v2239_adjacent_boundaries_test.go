//go:build unit

package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/util/clientprivacy"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestV2239AdjacentGeminiModelErrorsSurviveProjection(t *testing.T) {
	for _, fields := range []string{
		`"error":{},"error":null`, `"error":null,"error":{}`,
		`"status":"failed","status":"completed"`, `"status":"completed","status":"failed"`,
	} {
		for _, route := range []string{"native", "account-alias", "channel-alias"} {
			for _, detail := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/detail=%t", fields, route, detail), func(t *testing.T) {
					id := int64(45)
					public := "gemini-live"
					credentials := map[string]any{"api_key": "fixture-only"}
					if route == "account-alias" {
						public = "gemini-public"
						credentials["model_mapping"] = map[string]any{public: "gemini-live"}
					}
					metadata := `"models":[{"name":"models/gemini-live","custom":42}]`
					if detail {
						metadata = `"name":"models/gemini-live","custom":42`
					}
					body := "{" + metadata + "," + fields + `,"message":"https://private-provider.example","api_key":"synthetic-private-value"}`
					repo := &geminiAllowlistAccountRepoStub{gatewayModelsAccountRepoStub: gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{id: {
						{ID: 1, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey, Credentials: credentials},
					}}}}
					h := &GatewayHandler{}
					if route == "channel-alias" {
						public = "gemini-public"
						h, _ = newGeminiCatalogProjectionHandler(service.PlatformGemini, nil, map[string]map[string]string{service.PlatformGemini: {public: "gemini-live"}})
					}
					upstream := &geminiAdaptiveCatalogUpstream{status: http.StatusOK, body: body}
					h.geminiCompatService = service.NewGeminiMessagesCompatService(repo, nil, nil, nil, nil, nil, upstream, nil, &config.Config{})
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
					c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{GroupID: &id, Group: &service.Group{ID: id, Platform: service.PlatformGemini,
						ModelAllowlist: service.GroupModelAllowlist{Enabled: true, Models: []string{public}},
					}})
					if detail {
						c.Params = gin.Params{{Key: "model", Value: public}}
						h.GeminiV1BetaGetModel(c)
					} else {
						h.GeminiV1BetaListModels(c)
					}
					require.Equal(t, http.StatusOK, rec.Code, "the original HTTP status remains unchanged")
					require.Len(t, upstream.paths, 1)
					if detail {
						require.True(t, strings.HasSuffix(upstream.paths[0], "/models/gemini-live"))
					}
					require.Equal(t, string(clientprivacy.JSON([]byte(body))), rec.Body.String(), "errors must bypass all catalogue and alias projections")
					require.NotContains(t, rec.Body.String(), "private-provider")
					require.NotContains(t, rec.Body.String(), "synthetic-private-value")
				})
			}
		}
	}
}
