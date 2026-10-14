package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyModelScopeIndependentOfGroupAllowlist(t *testing.T) {
	for _, scope := range []string{"openai", "anthropic", "cn", "other"} {
		for _, model := range []string{"gpt-5", "claude-4", "qwen3", "gemini-3", "grok-4"} {
			key := &service.APIKey{ModelScope: scope, Group: &service.Group{Platform: service.PlatformMixed}}
			router, calls := newGroupModelAllowlistTestRouter(key, "/v1")
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"`+model+`"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Upgrade", "websocket") // spoofed upgrade cannot bypass HTTP admission
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if service.ClassifyModelScope(model) == scope {
				require.Equal(t, http.StatusOK, response.Code)
				require.Len(t, *calls, 1)
			} else {
				require.Equal(t, http.StatusNotFound, response.Code)
				require.Empty(t, *calls)
			}
		}
	}
}

func TestAPIKeyModelScopeDuplicateAndLegacy(t *testing.T) {
	key := &service.APIKey{ModelScope: "cn", Group: &service.Group{Platform: service.PlatformMixed}}
	router, calls := newGroupModelAllowlistTestRouter(key, "/v1")
	response := doJSON(t, router, http.MethodPost, "/v1/messages", `{"model":"qwen3","Model":"gpt-5"}`)
	require.Equal(t, http.StatusNotFound, response.Code)
	require.Empty(t, *calls)
	key.ModelScope = ""
	response = doJSON(t, router, http.MethodPost, "/v1/messages", `{"model":"gpt-5"}`)
	require.Equal(t, http.StatusOK, response.Code)
}
