package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayModelsDomesticCatalogAliasesRespectGroupAllowlist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, selected := range [][]string{nil, {"kimi-k2.6"}, {"moonshotai/kimi-k2.6"}, {"unlisted-model"}} {
		groupID := int64(218)
		h := newGatewayModelsHandlerForTest(&gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{
			groupID: {{ID: 1271, Platform: service.PlatformMixed, Type: service.AccountTypeAPIKey,
				Credentials: map[string]any{"model_mapping": map[string]any{"moonshotai/kimi-k2.6": "raw-upstream", "Agents-A1": "Agents-A1"}}}},
		}})
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{GroupID: &groupID, Group: &service.Group{
			ID: groupID, Platform: service.PlatformMixed, ModelAllowlist: service.GroupModelAllowlist{Enabled: selected != nil, Models: selected},
		}})
		h.Models(c)
		require.Equal(t, http.StatusOK, recorder.Code)
		var result gatewayModelsResponseForTest
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
		got := modelIDsForTest(result.Data)
		switch {
		case selected == nil:
			require.ElementsMatch(t, []string{"moonshotai/kimi-k2.6", "kimi-k2.6", "Agents-A1"}, got)
		case selected[0] == "unlisted-model":
			require.Empty(t, got)
		default:
			require.Equal(t, selected, got)
		}
	}
}
