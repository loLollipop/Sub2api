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

func TestSameMixedGroupFourAPIKeyCatalogues(t *testing.T) {
	gid := int64(218)
	h := newGatewayModelsHandlerForTest(&gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{gid: {
		{ID: 1, Platform: service.PlatformMixed, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": map[string]any{
			"gpt-5": "gpt-5", "claude-4": "claude-4", "qwen3": "qwen3", "gemini-3": "gemini-3", "grok-4": "grok-4", "gpt-cross": "qwen3", "claude-opaque": "raw-target",
		}}},
	}}})
	for scope, expected := range map[string][]string{"openai": {"gpt-5"}, "anthropic": {"claude-4"}, "cn": {"qwen3"}, "other": {"gemini-3", "grok-4"}} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ModelScope: scope, GroupID: &gid, Group: &service.Group{ID: gid, Platform: service.PlatformMixed}})
		h.Models(c)
		require.Equal(t, http.StatusOK, recorder.Code)
		var result gatewayModelsResponseForTest
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
		require.ElementsMatch(t, expected, modelIDsForTest(result.Data))
	}
}

func TestModelScopeCatalogueManifestETagAndIsolation(t *testing.T) {
	original := []byte(`{"models":[{"slug":"gpt-5"},{"slug":"qwen3"}],"metadata":"keep"}`)
	manifest := &service.OpenAIModelsResponse{Body: original, ETag: "unfiltered"}
	write := func(scope, etag string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/backend-api/codex/models", nil)
		c.Request.Header.Set("If-None-Match", etag)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ModelScope: scope})
		writeOpenAIModelsResponse(c, manifest)
		return recorder
	}
	openai := write("openai", "unfiltered")
	require.Equal(t, http.StatusOK, openai.Code)
	require.NotContains(t, openai.Body.String(), "qwen3")
	cn := write("cn", openai.Header().Get("ETag"))
	require.Equal(t, http.StatusOK, cn.Code)
	require.NotContains(t, cn.Body.String(), "gpt-5")
	require.Equal(t, http.StatusNotModified, write("cn", cn.Header().Get("ETag")).Code)
	require.Equal(t, original, manifest.Body)
	require.Equal(t, "unfiltered", manifest.ETag)
}

func TestWSKeyScopeCandidateInheritance(t *testing.T) {
	key := &service.APIKey{ModelScope: "cn"}
	require.Empty(t, blockedAPIKeyModelCandidate(key, []string{"qwen3"}))
	require.Equal(t, "gpt-5", blockedAPIKeyModelCandidate(key, []string{"qwen3", "gpt-5"}))
	key.ModelScope = "all"
	require.Empty(t, blockedAPIKeyModelCandidate(key, []string{"qwen3", "gpt-5"}))
}

func TestModelScopeManifestFiltersAccountWildcardWithoutMutatingSource(t *testing.T) {
	gid := int64(218)
	h := newGatewayModelsHandlerForTest(&gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{gid: {
		{Platform: service.PlatformMixed, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-*": "qwen3", "gpt-safe": "gpt-5"}}},
	}}})
	original := []byte(`{"models":[{"slug":"gpt-5","extra":true},{"slug":"gpt-safe","extra":true}],"metadata":"keep"}`)
	filtered, err := filterModelCatalogueForScope(original, "openai", func(models []string) []string {
		return h.gatewayService.FilterModelsForKeyScope(t.Context(), &gid, "openai", models)
	})
	require.NoError(t, err)
	require.NotContains(t, string(filtered), `"slug":"gpt-5"`)
	require.Contains(t, string(filtered), `"slug":"gpt-safe"`)
	require.Contains(t, string(filtered), `"extra":true`)
	require.Contains(t, string(filtered), `"metadata":"keep"`)
	require.Contains(t, string(original), `"slug":"gpt-5"`)
}
