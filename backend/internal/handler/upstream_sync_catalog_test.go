package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayModels_GPT55RetirementFallbackAndCache(t *testing.T) {
	for _, tc := range []struct {
		name     string
		platform string
		typ      string
		mapping  map[string]any
		empty    bool
		retired  bool
	}{
		{"oauth defaults", service.PlatformOpenAI, service.AccountTypeOAuth, nil, false, false},
		{"api defaults", service.PlatformOpenAI, service.AccountTypeAPIKey, nil, false, true},
		{"retired only", service.PlatformOpenAI, service.AccountTypeOAuth, map[string]any{"gpt-5.5": "gpt-5.5"}, true, false},
		{"composite retired only", service.PlatformComposite, service.AccountTypeOAuth, map[string]any{"gpt-5.5": "gpt-5.5"}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gid := int64(88)
			h := newGatewayModelsHandlerForTest(&gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{gid: {{
				ID: 1, Platform: service.PlatformOpenAI, Type: tc.typ, Credentials: map[string]any{"model_mapping": tc.mapping},
			}}}})
			for i := 0; i < 2; i++ {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
				c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{Group: &service.Group{ID: gid, Platform: tc.platform}})
				h.Models(c)
				require.Equal(t, http.StatusOK, rec.Code)
				var got gatewayModelsResponseForTest
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
				ids := modelIDsForTest(got.Data)
				require.Equal(t, tc.empty, len(ids) == 0, rec.Body.String())
				if tc.retired {
					require.Contains(t, ids, "gpt-5.5")
				} else {
					require.NotContains(t, ids, "gpt-5.5")
				}
			}
		})
	}
}

func TestGatewayModels_GPT55RetirementAllowlistedMultiGroupEmpty(t *testing.T) {
	primaryID, secondID := int64(88), int64(89)
	for _, platform := range []string{service.PlatformOpenAI, service.PlatformComposite} {
		t.Run(platform, func(t *testing.T) {
			h := newGatewayModelsHandlerForTest(&gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{
				primaryID: {{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.5": "gpt-5.5"}}}},
				secondID:  {{ID: 2, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.5": "gpt-5.5"}}}},
			}})
			primary := &service.Group{ID: primaryID, Platform: platform, ModelAllowlist: service.GroupModelAllowlist{Enabled: true, Models: []string{"gpt-*"}}}
			second := &service.Group{ID: secondID, Platform: platform, Hydrated: true, Status: service.StatusActive, ModelAllowlist: service.GroupModelAllowlist{Enabled: true, Models: []string{"gpt-*"}}}
			apiKey := &service.APIKey{GroupID: &primaryID, Group: primary, RouteGroupIDs: []int64{primaryID, secondID}}
			ctx := context.WithValue(context.Background(), ctxkey.Group, second)
			for range 2 {
				models := h.availableModelsForAPIKey(ctx, apiKey, "")
				require.NotNil(t, models, "allowlisting must retain authoritative empty catalogs")
				require.Empty(t, models)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil).WithContext(ctx)
				c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
				h.Models(c)
				require.Equal(t, http.StatusOK, rec.Code)
				var got gatewayModelsResponseForTest
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
				require.Empty(t, got.Data)
			}
		})
	}
}
