//go:build unit

package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type mixedHandlerUpstream struct {
	service.HTTPUpstream
	calls   int
	groupID int64
}

func (u *mixedHandlerUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.calls++
	if group, ok := req.Context().Value(ctxkey.Group).(*service.Group); ok {
		u.groupID = group.ID
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"cc_mixed","model":"gpt-5.1","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2}}`))}, nil
}

func newMixedSafetyHandler(t *testing.T, primary, backup *service.Group, platform string) (*OpenAIGatewayHandler, *mixedHandlerUpstream, *service.APIKey) {
	t.Helper()
	account := service.Account{ID: 1, Platform: platform, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, GroupIDs: []int64{backup.ID}, Credentials: map[string]any{"api_key": "test", "base_url": "https://upstream.invalid/v1", "model_mapping": map[string]any{"gpt-5.1": "gpt-5.1"}}}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	cfg.Default.RateMultiplier = 1
	accountRepo := openAIImagesFailoverAccountRepo{accounts: []service.Account{account}}
	snapshot := service.NewSchedulerSnapshotService(&smartRouteHandlerSnapshot{fakeSchedulerCache{accounts: []*service.Account{&account}}}, nil, accountRepo, smartRouteHandlerGroups{groups: map[int64]*service.Group{primary.ID: primary, backup.ID: backup}}, cfg)
	for _, id := range []int64{primary.ID, backup.ID} {
		_, err := snapshot.GetGroupByIDLite(context.Background(), id)
		require.NoError(t, err)
	}
	billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, &config.Config{RunMode: config.RunModeSimple}, nil)
	t.Cleanup(billingCache.Stop)
	upstream := &mixedHandlerUpstream{}
	usage := smartRouteHandlerUsage{logs: make(chan *service.UsageLog, 5)}
	gateway := service.NewOpenAIGatewayService(accountRepo, usage, smartRouteHandlerBilling{}, nil, nil, nil, nil, cfg, snapshot, nil, service.NewBillingService(cfg, nil), nil, billingCache, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
	h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(nil), billingCache, &service.APIKeyService{}, nil, nil, nil, nil, cfg)
	key := &service.APIKey{ID: 7, UserID: 42, User: &service.User{ID: 42, Status: service.StatusActive}, GroupID: &primary.ID, Group: primary, RouteGroupIDs: []int64{primary.ID, backup.ID}}
	return h, upstream, key
}

func mixedHandlerContext(key *service.APIKey, endpoint, body string) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	ctx := context.WithValue(context.Background(), ctxkey.Group, key.Group)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+endpoint, strings.NewReader(body)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyAPIKey), key)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42, Concurrency: 0})
	return c, rec
}

func TestMixedMessagesSmartKeyUsesCandidatePermissions(t *testing.T) {
	for _, platform := range []string{service.PlatformMixed, service.PlatformOpenAI, service.PlatformComposite} {
		t.Run(platform, func(t *testing.T) {
			primary := &service.Group{ID: 9, Platform: service.PlatformOpenAI, Status: service.StatusActive, Hydrated: true, RateMultiplier: 1}
			backup := &service.Group{ID: 17, Platform: platform, Status: service.StatusActive, Hydrated: true, RateMultiplier: 1, CrossProtocolConversionEnabled: true}
			accountPlatform := platform
			if platform == service.PlatformComposite {
				accountPlatform = service.PlatformMixed
			}
			h, upstream, key := newMixedSafetyHandler(t, primary, backup, accountPlatform)
			c, rec := mixedHandlerContext(key, "messages", `{"model":"gpt-5.1","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`)
			c.Request = c.Request.WithContext(service.WithResolvedTargetPlatform(c.Request.Context(), service.PlatformMixed))
			require.True(t, allowInitialOpenAICompatibleMessagesDispatch(c, key))
			h.Messages(c)
			if platform == service.PlatformMixed {
				require.Equal(t, 200, rec.Code, rec.Body.String())
				require.Contains(t, rec.Body.String(), "hello")
				require.Equal(t, 1, upstream.calls)
				require.Equal(t, backup.ID, upstream.groupID)
			} else {
				require.GreaterOrEqual(t, rec.Code, 400, rec.Body.String())
				require.Zero(t, upstream.calls, "resolved Mixed must not bypass an old candidate's switch")
			}
		})
	}
}

func TestMixedLocalLimitationsPreserveHandler400(t *testing.T) {
	for _, endpoint := range []string{"responses", "chat/completions", "messages"} {
		t.Run(endpoint, func(t *testing.T) {
			group := &service.Group{ID: 9, Platform: service.PlatformMixed, Status: service.StatusActive, Hydrated: true, RateMultiplier: 1}
			h, upstream, key := newMixedSafetyHandler(t, group, group, service.PlatformMixed)
			key.RouteGroupIDs = []int64{group.ID}
			body := `{"model":"gpt-5.1","audio":{},"max_tokens":64,"messages":[{"role":"user","content":"hello"}],"input":"hello"}`
			c, rec := mixedHandlerContext(key, endpoint, body)
			switch endpoint {
			case "responses":
				h.Responses(c)
			case "chat/completions":
				h.ChatCompletions(c)
			case "messages":
				h.Messages(c)
			}
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), `"invalid_request_error"`)
			require.Zero(t, upstream.calls)
		})
	}
}
