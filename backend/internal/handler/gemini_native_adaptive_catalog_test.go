//go:build unit

package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type geminiAdaptiveCatalogUpstream struct {
	service.HTTPUpstream
	paths  []string
	status int
	body   string
}

func (u *geminiAdaptiveCatalogUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.paths = append(u.paths, req.URL.Path)
	if u.status != 0 {
		return (&geminiMixedModelsUpstream{status: u.status, body: u.body}).Do(req, "", 0, 0)
	}
	body := `{"name":"models/gemini-visible","inputTokenLimit":123,"custom":{"retained":true}}`
	if req.URL.Path == "/relay/v1beta/models" {
		body = `{"models":[` + body + `,{"name":"models/gemini-hidden","inputTokenLimit":456}],"nextPageToken":"next","customEnvelope":42}`
	}
	return (&geminiMixedModelsUpstream{status: http.StatusOK, body: body}).Do(req, "", 0, 0)
}

func TestGeminiNativeAdaptiveMappedCatalogUsesRealUpstream(t *testing.T) {
	for _, protocol := range []string{"", service.APIProtocolAdaptive} {
		for _, status := range []int{http.StatusOK, http.StatusForbidden, http.StatusNotFound} {
			t.Run(protocol+"/"+http.StatusText(status), func(t *testing.T) {
				id := int64(46)
				repo := &geminiAllowlistAccountRepoStub{gatewayModelsAccountRepoStub: gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{id: {
					{ID: 1, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey, Credentials: map[string]any{
						"api_key": "test", "base_url": "https://relay.example.com/relay/v1", "api_protocol": protocol,
						"model_mapping": map[string]any{"gemini-public": "gemini-live", "gemini-absent": "gemini-missing"},
					}},
				}}}}
				body := `{"name":"models/gemini-live","inputTokenLimit":123,"custom":{"retained":true}}`
				if status != http.StatusOK {
					body = `{"error":{"message":"Native model unavailable","status":"PERMISSION_DENIED"}}`
				}
				upstream := &geminiAdaptiveCatalogUpstream{status: status, body: body}
				h := &GatewayHandler{geminiCompatService: service.NewGeminiMessagesCompatService(repo, nil, nil, nil, nil, nil, upstream, nil, &config.Config{})}
				apiKey := &service.APIKey{GroupID: &id, Group: &service.Group{ID: id, Platform: service.PlatformGemini}}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models/gemini-public", nil)
				c.Params = gin.Params{{Key: "model", Value: "gemini-public"}}
				c.Set(string(middleware.ContextKeyAPIKey), apiKey)
				h.GeminiV1BetaGetModel(c)
				require.Equal(t, []string{"/relay/v1beta/models/gemini-live"}, upstream.paths)
				require.Equal(t, status, rec.Code)
				if status != http.StatusOK {
					require.JSONEq(t, body, rec.Body.String())
					return
				}
				require.Contains(t, rec.Body.String(), `"name":"models/gemini-public"`)
				require.Contains(t, rec.Body.String(), `"inputTokenLimit":123`)
				require.Contains(t, rec.Body.String(), `"custom":{"retained":true}`)
				upstream.paths = nil
				upstream.body = `{"models":[` + body + `],"customEnvelope":42}`
				rec = httptest.NewRecorder()
				c, _ = gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
				c.Set(string(middleware.ContextKeyAPIKey), apiKey)
				h.GeminiV1BetaListModels(c)
				require.Equal(t, []string{"/relay/v1beta/models"}, upstream.paths)
				require.Equal(t, http.StatusOK, rec.Code)
				require.Contains(t, rec.Body.String(), `"name":"models/gemini-public"`)
				require.Contains(t, rec.Body.String(), `"custom":{"retained":true}`)
				require.NotContains(t, rec.Body.String(), "gemini-absent")
				require.NotContains(t, rec.Body.String(), "gemini-missing")
			})
		}
	}
}

func TestGeminiNativeAdaptiveCatalogPreservesMetadataAndAuthorization(t *testing.T) {
	for _, protocol := range []string{"", service.APIProtocolAdaptive} {
		for _, restrict := range []bool{false, true} {
			name := protocol + "/all"
			if restrict {
				name = protocol + "/allowlist"
			}
			t.Run(name, func(t *testing.T) {
				id := int64(46)
				repo := &geminiAllowlistAccountRepoStub{gatewayModelsAccountRepoStub: gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{id: {
					{ID: 1, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test", "base_url": "https://relay.example.com/relay/v1", "api_protocol": protocol}},
				}}}}
				upstream := &geminiAdaptiveCatalogUpstream{}
				h := &GatewayHandler{geminiCompatService: service.NewGeminiMessagesCompatService(repo, nil, nil, nil, nil, nil, upstream, nil, &config.Config{})}
				apiKey := &service.APIKey{GroupID: &id, Group: &service.Group{ID: id, Platform: service.PlatformGemini,
					ModelAllowlist: service.GroupModelAllowlist{Enabled: restrict, Models: []string{"gemini-visible"}},
				}}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
				c.Set(string(middleware.ContextKeyAPIKey), apiKey)
				h.GeminiV1BetaListModels(c)
				require.Equal(t, http.StatusOK, rec.Code)
				require.Equal(t, []string{"/relay/v1beta/models"}, upstream.paths)
				require.Contains(t, rec.Body.String(), `"inputTokenLimit":123`)
				require.Contains(t, rec.Body.String(), `"custom":{"retained":true}`)
				require.Contains(t, rec.Body.String(), `"nextPageToken":"next"`)
				require.Contains(t, rec.Body.String(), `"customEnvelope":42`)
				if restrict {
					require.NotContains(t, rec.Body.String(), "gemini-hidden")
				} else {
					require.Contains(t, rec.Body.String(), "gemini-hidden")
				}
				// Model details must respect the same group allowlist before forwarding.
				rec = httptest.NewRecorder()
				c, _ = gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models/gemini-hidden", nil)
				c.Params = gin.Params{{Key: "model", Value: "gemini-hidden"}}
				c.Set(string(middleware.ContextKeyAPIKey), apiKey)
				if restrict {
					h.GeminiV1BetaGetModel(c)
					require.Equal(t, http.StatusNotFound, rec.Code)
					require.Len(t, upstream.paths, 1)
				}
			})
		}
	}
}
