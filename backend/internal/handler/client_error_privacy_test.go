package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type clientPrivacyRulesRepo struct {
	service.ErrorPassthroughRepository
	rules []*model.ErrorPassthroughRule
}

type clientPrivacyModelsUpstream struct {
	service.HTTPUpstream
	status int
	body   string
}

func (u clientPrivacyModelsUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return &http.Response{StatusCode: u.status, Header: http.Header{
		"Content-Type":     {`application/json; upstream="https://private-provider.example"; api_key="synthetic-private-value"`},
		"Www-Authenticate": {`Bearer realm="https://private-provider.example"`},
		"X-Request-Id":     {"safe-correlation"},
	}, Body: io.NopCloser(strings.NewReader(u.body))}, nil
}

func TestClientErrorPrivacyGeminiModelListAndDetail(t *testing.T) {
	for _, status := range []int{200, 401, 503} {
		for _, detail := range []bool{false, true} {
			t.Run(http.StatusText(status)+"/detail="+boolLabel(detail), func(t *testing.T) {
				id := int64(46)
				repo := &geminiAllowlistAccountRepoStub{gatewayModelsAccountRepoStub: gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{id: {
					{ID: 1, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture-only", "model_mapping": map[string]any{"gemini-native": "gemini-native"}}},
				}}}}
				body := `{"models":[{"name":"models/gemini-native","description":"https://public.example"}]}`
				if detail {
					body = `{"name":"models/gemini-native","description":"https://public.example"}`
				}
				if status >= 400 {
					body = `{"error":{"message":"https://private-provider.example failed","api_key":"synthetic-private-value"}}`
				}
				upstream := clientPrivacyModelsUpstream{status: status, body: body}
				h := &GatewayHandler{geminiCompatService: service.NewGeminiMessagesCompatService(repo, nil, nil, nil, nil, nil, upstream, nil, &config.Config{})}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
				c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{GroupID: &id, Group: &service.Group{ID: id, Platform: service.PlatformGemini}})
				if detail {
					c.Params = gin.Params{{Key: "model", Value: "gemini-native"}}
					h.GeminiV1BetaGetModel(c)
				} else {
					h.GeminiV1BetaListModels(c)
				}
				require.Equal(t, status, rec.Code, rec.Body.String())
				require.Empty(t, rec.Header().Get("WWW-Authenticate"))
				require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
				require.Equal(t, "safe-correlation", rec.Header().Get("X-Request-Id"))
				require.NotContains(t, rec.Body.String(), "private-provider")
				require.NotContains(t, rec.Body.String(), "synthetic-private-value")
				if status == 200 {
					require.Contains(t, rec.Body.String(), "https://public.example")
				}
			})
		}
	}
}

func (r clientPrivacyRulesRepo) List(context.Context) ([]*model.ErrorPassthroughRule, error) {
	return r.rules, nil
}

func TestClientErrorPrivacyFailoverRulesAcrossProtocols(t *testing.T) {
	const message = "provider https://private-provider.example rejected api_key=synthetic-opaque-value"
	const body = `{"error":{"type":"invalid_request_error","message":"` + message + `"}}`
	for _, rule := range []bool{false, true} {
		for _, protocol := range []string{"anthropic", "openai", "gemini"} {
			for _, stream := range []bool{false, true} {
				if protocol == "gemini" && stream {
					continue
				}
				t.Run(protocol+"/rule="+boolLabel(rule)+"/stream="+boolLabel(stream), func(t *testing.T) {
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
					var rules *service.ErrorPassthroughService
					if rule {
						rules = service.NewErrorPassthroughService(clientPrivacyRulesRepo{rules: []*model.ErrorPassthroughRule{{
							ID: 1, Name: "fixture", Enabled: true, MatchMode: model.MatchModeAny,
							ErrorCodes: []int{422}, PassthroughBody: true, PassthroughCode: true,
						}}}, nil)
					}
					service.SetOpsUpstreamError(c, 422, message, "")
					failure := &service.UpstreamFailoverError{StatusCode: 422, ResponseBody: []byte(body)}
					switch protocol {
					case "anthropic":
						(&GatewayHandler{errorPassthroughService: rules}).handleFailoverExhausted(c, failure, service.PlatformAnthropic, stream)
					case "openai":
						(&OpenAIGatewayHandler{errorPassthroughService: rules}).handleFailoverExhausted(c, failure, stream)
					case "gemini":
						(&GatewayHandler{errorPassthroughService: rules}).handleGeminiFailoverExhausted(c, failure)
					}
					require.NotContains(t, rec.Body.String(), "private-provider")
					require.NotContains(t, rec.Body.String(), "synthetic-opaque-value")
					require.Equal(t, body, string(failure.ResponseBody), "retry and classification evidence stays intact")
					require.Equal(t, message, c.GetString(service.OpsUpstreamErrorMessageKey))
				})
			}
		}
	}
}

func boolLabel(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
