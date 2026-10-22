//go:build unit

package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// singleAccountProbeUpstream returns a non-retryable response without network I/O.
type singleAccountProbeUpstream struct {
	service.HTTPUpstream
	calls atomic.Int32
}

func (u *singleAccountProbeUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.calls.Add(1)
	return &http.Response{StatusCode: http.StatusBadRequest, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"invalid request","status":"INVALID_ARGUMENT"}}`))}, nil
}

type singleAccountProbeFixture struct {
	handler        *GatewayHandler
	group          *service.Group
	apiKey         *service.APIKey
	upstream       *singleAccountProbeUpstream
	schedulerCache *bucketRecordingSchedulerCache
	ctx            context.Context
}

func newSingleAccountProbeFixture(t *testing.T) *singleAccountProbeFixture {
	t.Helper()
	groupID := int64(9200)
	accountID := int64(9201)
	group := &service.Group{ID: groupID, Hydrated: true, Platform: service.PlatformGemini, Status: service.StatusActive}
	account := &service.Account{
		ID:            accountID,
		Platform:      service.PlatformGemini,
		Type:          service.AccountTypeAPIKey,
		Credentials:   map[string]any{"api_key": "test-key"},
		Status:        service.StatusActive,
		Schedulable:   true,
		Concurrency:   1,
		Priority:      1,
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
	}
	schedulerCache := &bucketRecordingSchedulerCache{fakeSchedulerCache: &fakeSchedulerCache{accounts: []*service.Account{account}}}
	h, cleanup := newTestGatewayHandlerWithSchedulerCache(t, group, schedulerCache)
	t.Cleanup(cleanup)

	ctx := context.Background()
	upstream := &singleAccountProbeUpstream{}
	h.geminiCompatService = service.NewGeminiMessagesCompatService(nil, nil, nil, nil, nil, nil, upstream, nil, &config.Config{})

	apiKey := &service.APIKey{
		ID: 9202, UserID: 9203, GroupID: &groupID, Group: group, Status: service.StatusActive,
		User: &service.User{ID: 9203, Concurrency: 10, Balance: 100},
	}
	return &singleAccountProbeFixture{handler: h, group: group, apiKey: apiKey, upstream: upstream, schedulerCache: schedulerCache, ctx: ctx}
}

// serve 经 ops 错误日志中间件执行请求，返回响应与 ops 队列中的条目数。
func (f *singleAccountProbeFixture) serve(t *testing.T, route, path, body string, call func(*gin.Context)) (*httptest.ResponseRecorder, int64) {
	t.Helper()
	setupOpsErrorLogTestQueue(t, 4)
	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.POST(route, func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), f.apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: f.apiKey.UserID, Concurrency: 10})
		call(c)
	})

	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req = req.WithContext(context.WithValue(f.ctx, ctxkey.Group, f.group))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec, OpsErrorLogQueueLength()
}
