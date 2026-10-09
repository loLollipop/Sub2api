package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIHandlers_RejectConflictingModelsWithMultipartContentType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []struct {
		path   string
		handle func(*OpenAIGatewayHandler, *gin.Context)
	}{
		{"/v1/responses", (*OpenAIGatewayHandler).Responses},
		{"/v1/chat/completions", (*OpenAIGatewayHandler).ChatCompletions},
	} {
		t.Run(endpoint.path, func(t *testing.T) {
			for _, body := range []string{
				`{"model":"cheap-model","model":"gpt-6-astra","input":"hello","messages":[{"role":"user","content":"hello"}]}`,
				`{"model":"cheap-model","Model":"gpt-6-astra","input":"hello","messages":[{"role":"user","content":"hello"}]}`,
			} {
				t.Run(body, func(t *testing.T) {
					cfg := &config.Config{RunMode: config.RunModeSimple}
					cfg.Default.RateMultiplier = 1
					accountRepo := &openAIWSUsageHandlerAccountRepoStub{account: service.Account{
						ID: 9901, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
						Status: service.StatusActive, Schedulable: true,
						Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://api.example.test"},
						Extra:       map[string]any{"openai_passthrough": true},
					}}
					usageRepo := &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 1)}
					upstream := &openAIHTTPPassthroughFailoverUpstream{}
					billingCacheSvc := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
					t.Cleanup(billingCacheSvc.Stop)
					gatewaySvc := service.NewOpenAIGatewayService(
						accountRepo, usageRepo, nil, nil, nil, nil, nil, cfg, nil, nil,
						service.NewBillingService(cfg, nil), nil, billingCacheSvc, upstream,
						&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
					)
					h := NewOpenAIGatewayHandler(
						gatewaySvc, service.NewConcurrencyService(nil), billingCacheSvc,
						service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
						nil, nil, nil, nil, cfg,
					)

					groupID := int64(4201)
					group := wsAllowlistGroup(false, "cheap-model")
					group.ID = groupID
					group.Hydrated = true
					group.Status = service.StatusActive
					router := gin.New()
					router.Use(func(c *gin.Context) {
						c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
							ID: 1801, GroupID: &groupID, Group: group,
							User: &service.User{ID: 1701, Status: service.StatusActive},
						})
						c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1701})
						c.Next()
					})
					router.POST(endpoint.path, func(c *gin.Context) { endpoint.handle(h, c) })
					req := httptest.NewRequest(http.MethodPost, endpoint.path, strings.NewReader(body))
					req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
					w := httptest.NewRecorder()
					router.ServeHTTP(w, req)

					require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
					require.Contains(t, w.Body.String(), service.ErrDuplicateModelField.Error())
					require.Empty(t, upstream.calls(), "conflicting models must not be forwarded")
					select {
					case log := <-usageRepo.created:
						t.Fatalf("rejected request must not record usage: %+v", log)
					default:
					}
				})
			}
		})
	}
}

func TestOpenAIResponsesWebSocket_ConflictingModelsRejectedWithAllowlistDisabled(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModePassthrough, service.OpenAIWSIngressModeDedicated} {
		t.Run(mode, func(t *testing.T) {
			for _, stage := range []string{"first frame", "second turn"} {
				t.Run(stage, func(t *testing.T) {
					tc := openAIResponsesWSUsageLogCase{
						firstPayload:        `{"type":"response.create","model":"gpt-5.4","stream":false}`,
						group:               wsAllowlistGroup(false, "gpt-5.4"),
						ingressMode:         mode,
						expectedCloseReason: service.ErrDuplicateModelField.Error(),
					}
					conflict := `{"type":"response.create","model":"gpt-5.4","model":"gpt-4.1","stream":false}`
					if stage == "first frame" {
						tc.firstPayload = conflict
						tc.firstFrameCloseExpected = true
					} else {
						tc.secondPayload = conflict
						tc.secondTurnCloseExpected = true
					}
					runOpenAIResponsesWebSocketUsageLogCase(t, tc)
				})
			}
		})
	}
}

func TestOpenAIResponsesWebSocket_DeepSeekDedupeDoesNotHideConflictingModels(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModePassthrough, service.OpenAIWSIngressModeDedicated} {
		t.Run(mode, func(t *testing.T) {
			runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
				firstPayload:            `{"type":"response.create","model":"gpt-5.4","stream":false}`,
				secondPayload:           `{"type":"response.create","model":"deepseek-v4","model":"deepseek-v3","stream":false,"input":[{"type":"function_call","call_id":"call_a","name":"do","arguments":"{}"},{"type":"function_call","call_id":"call_a","name":"do","arguments":"{}"}]}`,
				group:                   wsAllowlistGroup(false, "gpt-5.4"),
				ingressMode:             mode,
				expectedCloseReason:     service.ErrDuplicateModelField.Error(),
				secondTurnCloseExpected: true,
			})
		})
	}
}
