//go:build unit

package handler

import (
	"context"
	"errors"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/testutil"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type refusalHandlerRepo struct {
	*durableMediaHandlerRepo
	blocks               map[int64]map[string]string
	lookupErr, commitErr error
	allowOrdinaryUsage   bool
}

func (r *refusalHandlerRepo) Apply(ctx context.Context, cmd *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	if !r.allowOrdinaryUsage {
		return r.durableMediaHandlerRepo.Apply(ctx, cmd)
	}
	r.mu.Lock()
	r.directApply++
	r.mu.Unlock()
	return &service.UsageBillingApplyResult{}, nil
}

func (r *refusalHandlerRepo) FindGrokRefusal(_ context.Context, user int64, keys []string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lookupErr != nil {
		return false, r.lookupErr
	}
	for _, k := range keys {
		if r.blocks[user][k] != "" {
			return true, nil
		}
	}
	return false, nil
}
func (r *refusalHandlerRepo) CommitGrokRefusal(_ context.Context, job *service.MediaBillingJob, lookup, write []string) (*service.MediaBillingJob, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.commitErr != nil {
		return nil, false, r.commitErr
	}
	for _, k := range lookup {
		if id := r.blocks[job.UserID][k]; id != "" {
			return cloneDurableMediaHandlerJob(r.jobs[id]), false, nil
		}
	}
	if r.blocks == nil {
		r.blocks = map[int64]map[string]string{}
	}
	if r.blocks[job.UserID] == nil {
		r.blocks[job.UserID] = map[string]string{}
	}
	stored := cloneDurableMediaHandlerJob(job)
	stored.Status = service.MediaBillingBilled
	r.jobs[job.ID] = stored
	r.applyCount++
	r.balance -= job.Intent.Command.BalanceCost
	for _, k := range write {
		r.blocks[job.UserID][k] = job.ID
	}
	return cloneDurableMediaHandlerJob(stored), true, nil
}

type refusalHandlerUpstream struct {
	service.HTTPUpstream
	calls           atomic.Int32
	body            string
	status          int
	contentType     string
	responseForCall func(int32) (int, string, string)
}

type refusalHandlerAccountRepo struct {
	openAIImagesFailoverAccountRepo
}

func (refusalHandlerAccountRepo) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	return nil
}

func (u *refusalHandlerUpstream) Do(*http.Request, string, int64, int) (*http.Response, error) {
	call := u.calls.Add(1)
	if u.responseForCall != nil {
		status, contentType, body := u.responseForCall(call)
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}
	status := u.status
	if status == 0 {
		status = http.StatusForbidden
	}
	contentType := u.contentType
	if contentType == "" {
		contentType = "application/json"
	}
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(u.body))}, nil
}

func newRefusalHandler(t *testing.T, price *float64) (*OpenAIGatewayHandler, *refusalHandlerRepo, *refusalHandlerUpstream, *service.APIKey) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	primaryPrice := 4.0
	primary := &service.Group{ID: 9, Platform: service.PlatformGrok, Status: service.StatusActive, Hydrated: true, CrossProtocolConversionEnabled: true, RateMultiplier: 13, GrokModerationRefusalPrice: &primaryPrice}
	selected := &service.Group{ID: 17, Platform: service.PlatformGrok, Status: service.StatusActive, Hydrated: true, CrossProtocolConversionEnabled: true, RateMultiplier: 7, GrokModerationRefusalPrice: price}
	accounts := []service.Account{{ID: 17001, Platform: service.PlatformGrok, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, GroupIDs: []int64{17}, Concurrency: 10, Credentials: map[string]any{"api_key": "synthetic", "model_mapping": map[string]any{"grok-4": "grok-4", "grok-4-fast": "grok-4-fast"}}}}
	accountRepo := refusalHandlerAccountRepo{openAIImagesFailoverAccountRepo{accounts: accounts}}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	cfg.Default.RateMultiplier = 3
	cfg.Billing.BalancePreauthorizationEnabled = false
	repo := &refusalHandlerRepo{durableMediaHandlerRepo: &durableMediaHandlerRepo{balance: 10, jobs: map[string]*service.MediaBillingJob{}}}
	cache := &durableMediaBalanceCache{repo: repo.durableMediaHandlerRepo}
	usage := &durableMediaUsageRepo{}
	upstream := &refusalHandlerUpstream{body: `{"error":{"message":"I'm sorry, I can't help with that request. Failed check: SAFETY_CHECK_TYPE"}}`}
	key := &service.APIKey{ID: 71, UserID: 42, User: &service.User{ID: 42, Status: service.StatusActive, Balance: 10}, GroupID: &primary.ID, Group: primary, RouteGroupIDs: []int64{9, 17}}
	snapshot := service.NewSchedulerSnapshotService(&smartRouteHandlerSnapshot{fakeSchedulerCache{accounts: []*service.Account{&accounts[0]}}}, nil, accountRepo, smartRouteHandlerGroups{groups: map[int64]*service.Group{9: primary, 17: selected}}, cfg)
	for _, id := range []int64{9, 17} {
		_, err := snapshot.GetGroupByIDLite(context.Background(), id)
		require.NoError(t, err)
	}
	billingCache := service.NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCache.Stop)
	gateway := service.NewOpenAIGatewayService(accountRepo, usage, repo, nil, nil, nil, &videoRouteBillingCache{bindings: map[string]int64{}, pending: map[string][]byte{}, billed: map[string]bool{}}, cfg, snapshot, nil, service.NewBillingService(cfg, nil), nil, billingCache, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
	t.Cleanup(gateway.CloseOpenAIWSPool)
	handler := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(testutil.StubConcurrencyCache{}), billingCache, &service.APIKeyService{}, nil, nil, nil, nil, cfg)
	return handler, repo, upstream, key
}
func refusalHandlerRequest(h *OpenAIGatewayHandler, key *service.APIKey, endpoint, model, question string) *httptest.ResponseRecorder {
	body := `{"model":"` + model + `","messages":[{"role":"user","content":"` + question + `"}],"max_tokens":32}`
	if endpoint == "responses" {
		body = `{"model":"` + model + `","input":[{"role":"user","content":[{"type":"input_text","text":"` + question + `"}]}]}`
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/"+endpoint, strings.NewReader(body)).WithContext(service.ContextWithAPIKeyRoute(context.Background(), key))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyAPIKey), key)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: key.UserID})
	switch endpoint {
	case "responses":
		h.Responses(c)
	case "messages":
		h.Messages(c)
	default:
		h.ChatCompletions(c)
	}
	return rec
}
func TestGrokRefusalHTTPThreeProtocolsSelectedRouteAndLocalBlock(t *testing.T) {
	for _, endpoint := range []string{"chat/completions", "responses", "messages"} {
		for _, price := range []*float64{nil, func() *float64 { v := 0.0; return &v }()} {
			t.Run(endpoint+func() string {
				if price == nil {
					return "/default"
				}
				return "/zero"
			}(), func(t *testing.T) {
				h, repo, up, key := newRefusalHandler(t, price)
				first := refusalHandlerRequest(h, key, endpoint, "grok-4", "refuse")
				require.Equal(t, 403, first.Code, first.Body.String())
				require.Equal(t, int32(1), up.calls.Load())
				require.Equal(t, 1, repo.applyCount)
				require.Len(t, repo.jobs, 1)
				expected := .1
				if price != nil {
					expected = *price
				}
				for _, job := range repo.jobs {
					require.Equal(t, int64(17), job.GroupID)
					require.Equal(t, expected, job.Intent.Command.BalanceCost)
					require.Equal(t, float64(1), job.Intent.Usage.RateMultiplier)
					require.Nil(t, job.SubscriptionID)
					require.Equal(t, service.MediaBillingDone, job.Status)
				}
				otherKey := *key
				otherKey.ID = 72
				for _, next := range []string{"chat/completions", "responses", "messages"} {
					response := refusalHandlerRequest(h, &otherKey, next, "grok-4-fast", "refuse")
					require.Equal(t, 403, response.Code, response.Body.String())
				}
				require.Equal(t, int32(1), up.calls.Load())
				require.Equal(t, 1, repo.applyCount)
				otherUser := otherKey
				otherUser.UserID = 43
				otherUser.User = &service.User{ID: 43, Status: service.StatusActive, Balance: 10}
				response := refusalHandlerRequest(h, &otherUser, endpoint, "grok-4", "refuse")
				require.Equal(t, 403, response.Code, response.Body.String())
				require.Equal(t, int32(2), up.calls.Load())
				require.Equal(t, 2, repo.applyCount)
			})
		}
	}
}
func TestGrokRefusalHTTPStorageFailureAndNonContentExclusions(t *testing.T) {
	for _, mode := range []string{"lookup", "commit", "account", "ip", "global_ip", "cyber"} {
		t.Run(mode, func(t *testing.T) {
			h, repo, up, key := newRefusalHandler(t, nil)
			switch mode {
			case "lookup":
				repo.lookupErr = errors.New("db offline")
			case "commit":
				repo.commitErr = errors.New("commit failed")
			case "account":
				up.body = `{"error":{"code":"account_disabled","message":"I'm sorry, I can't help with that request."}}`
			case "ip":
				up.body = `{"error":{"message":"IP address blocked. I'm sorry, I can't help with that request."}}`
			case "global_ip":
				up.body = `{"error":{"message":"blocked by global ip blacklist. I'm sorry, I can't help with that request. Failed check: SAFETY_CHECK_TYPE"}}`
			case "cyber":
				up.body = `{"error":{"code":"cyber_policy","message":"request rejected"}}`
			}
			response := refusalHandlerRequest(h, key, "chat/completions", "grok-4", "refuse")
			if mode == "lookup" || mode == "commit" {
				require.Equal(t, 503, response.Code, response.Body.String())
			}
			if mode == "lookup" {
				require.Zero(t, up.calls.Load())
			}
			require.Empty(t, repo.jobs)
			require.Zero(t, repo.applyCount)
		})
	}
}

func TestGrokRefusalHTTPConcurrentRefusalsHaveOneCharge(t *testing.T) {
	h, repo, up, key := newRefusalHandler(t, nil)
	var wg sync.WaitGroup
	responses := make(chan *httptest.ResponseRecorder, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			responses <- refusalHandlerRequest(h, key, "chat/completions", "grok-4", "same request")
		}()
	}
	wg.Wait()
	close(responses)
	for response := range responses {
		require.Equal(t, 403, response.Code, response.Body.String())
	}
	require.Equal(t, 1, repo.applyCount)
	require.Len(t, repo.jobs, 1)
	require.GreaterOrEqual(t, up.calls.Load(), int32(1))
}

func TestGrokRefusalCheckDoesNotAffectNonGrokRequest(t *testing.T) {
	h, repo, _, key := newRefusalHandler(t, nil)
	repo.lookupErr = errors.New("storage unavailable")
	keyCopy := *key
	group := *key.Group
	group.Platform = service.PlatformOpenAI
	keyCopy.Group = &group
	keyCopy.RouteGroupIDs = nil
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil).WithContext(service.ContextWithAPIKeyRoute(context.Background(), &keyCopy))
	require.False(t, rejectGrokRefusedRequest(c, h.gatewayService, &keyCopy, []byte(`{"model":"gpt-5","messages":[{"role":"user","content":"x"}]}`), nil, h.errorResponse))
}

func TestGrokRefusalHTTPThreeProtocolsRequireReal403(t *testing.T) {
	for _, endpoint := range []string{"chat/completions", "responses", "messages"} {
		for _, status := range []int{http.StatusBadRequest, http.StatusOK} {
			t.Run(fmt.Sprintf("%s/%d", endpoint, status), func(t *testing.T) {
				h, repo, up, key := newRefusalHandler(t, nil)
				up.status = status
				up.body = `{"error":{"status":403,"code":"content_policy_violation","message":"I'm sorry, I can't help with that request. Failed check: SAFETY_CHECK_TYPE"}}`
				_ = refusalHandlerRequest(h, key, endpoint, "grok-4", "refuse")
				require.Empty(t, repo.jobs)
				require.Empty(t, repo.blocks)
				require.Zero(t, repo.applyCount)
			})
		}
	}
}
