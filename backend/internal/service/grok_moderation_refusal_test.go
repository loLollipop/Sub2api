//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"math"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGrokRefusalToolOnlyResponsesHavePreciseIdentity(t *testing.T) {
	input := []byte(`{"instructions":"common","previous_response_id":"resp_123","input":[{"type":"function_call_output","call_id":"c1","output":"denied text"}]}`)
	lookup, written := grokRefusalIdentity(nil, input)
	require.Len(t, written, 1)
	require.Contains(t, lookup, written[0])
	_, instructions := grokRefusalIdentity(nil, []byte(`{"instructions":"common"}`))
	require.Empty(t, instructions)
	other, _ := grokRefusalIdentity(nil, []byte(strings.ReplaceAll(string(input), "denied text", "allowed text")))
	require.NotContains(t, other, written[0])
}

type refusalOnlyRecoveryRepo struct {
	*mediaJobMemoryRepo
	unrestricted bool
}

func (r *refusalOnlyRecoveryRepo) ListMediaBillingJobs(context.Context, int) ([]*MediaBillingJob, error) {
	r.unrestricted = true
	return nil, errors.New("unrestricted claim")
}
func (r *refusalOnlyRecoveryRepo) ListGrokRefusalBillingJobs(_ context.Context, limit int) ([]*MediaBillingJob, error) {
	for _, job := range r.jobs {
		if job.Kind == MediaBillingKindGrokRefusal && job.Status == MediaBillingBilled && job.Intent.Command.BalanceCost == 0 {
			return []*MediaBillingJob{cloneMediaTestJob(job)}, nil
		}
	}
	return nil, nil
}
func TestGrokRefusalSimpleRecoveryDoesNotLeaseOldMedia(t *testing.T) {
	base := &mediaJobMemoryRepo{jobs: map[string]*MediaBillingJob{
		"image":   {ID: "image", Kind: MediaBillingKindImage, Status: MediaBillingReady},
		"video":   {ID: "video", Kind: MediaBillingKindGrokVideo, Status: MediaBillingSubmitted},
		"refusal": {ID: "refusal", Kind: MediaBillingKindGrokRefusal, Status: MediaBillingBilled, Intent: &MediaBillingIntent{}},
		"paid":    {ID: "paid", Kind: MediaBillingKindGrokRefusal, Status: MediaBillingBilled, Intent: &MediaBillingIntent{Command: UsageBillingCommand{BalanceCost: .1}}},
	}}
	repo := &refusalOnlyRecoveryRepo{mediaJobMemoryRepo: base}
	svc := &OpenAIGatewayService{cfg: &config.Config{RunMode: config.RunModeSimple}, mediaBillingJobs: repo, usageLogRepo: &mediaUsageMemoryRepo{}}
	svc.recoverMediaBillingJobs(context.Background())
	require.False(t, repo.unrestricted)
	require.Equal(t, MediaBillingDone, base.jobs["refusal"].Status)
	for _, id := range []string{"image", "video", "paid"} {
		require.True(t, base.jobs[id].AvailableAt.IsZero())
	}
	require.Equal(t, 0, base.applied)
}

type refusalQuotaTestRepo struct {
	*fakeQuotaRepo
	usage *GrokRefusalQuotaUsage
	err   error
}

func (r *refusalQuotaTestRepo) GetGrokRefusalQuotaUsage(context.Context, int64, string) (*GrokRefusalQuotaUsage, error) {
	return r.usage, r.err
}
func TestGrokRefusalQuotaSurvivesBaseSnapshotsAndCacheRefill(t *testing.T) {
	now := time.Now()
	limit := 1.0
	q := &GrokRefusalQuotaUsage{}
	q.Add(.2, now, &now)
	base := &UserPlatformQuotaRecord{DailyLimitUSD: &limit, DailyUsageUSD: .85, DailyWindowStart: q.DailyStart, WeeklyWindowStart: q.WeeklyStart, MonthlyWindowStart: q.MonthlyStart}
	repo := &refusalQuotaTestRepo{fakeQuotaRepo: &fakeQuotaRepo{rec: base}, usage: q}
	cache := &fakeFullCache{}
	svc := &BillingCacheService{cfg: &config.Config{}, cache: cache, userPlatformQuotaRepo: repo}
	for i := 0; i < 2; i++ {
		require.ErrorIs(t, svc.checkUserPlatformQuotaEligibility(context.Background(), 1, PlatformGrok), ErrUserPlatformDailyQuotaExhausted)
		require.InDelta(t, .85, cache.getEntry().DailyUsageUSD, 1e-9, "refusal must never enter flusher snapshot")
	}
	repo.err = errors.New("database down")
	require.ErrorIs(t, svc.checkUserPlatformQuotaEligibility(context.Background(), 1, PlatformGrok), ErrBillingServiceUnavailable)
	q.Add(.1, now.Add(31*24*time.Hour), nil)
	d, w, m := q.Current(now.Add(31 * 24 * time.Hour))
	require.InDelta(t, .1, d, 1e-9)
	require.InDelta(t, .1, w, 1e-9)
	require.InDelta(t, .1, m, 1e-9)
}

func TestGrokRefusalIdentityAcrossProtocolsAndLongHistory(t *testing.T) {
	messages := []map[string]any{{"role": "system", "content": " exact system "}, {"role": "user", "content": "hello"}, {"role": "assistant", "content": "hi"}, {"role": "user", "content": "refused request"}}
	raw, _ := json.Marshal(map[string]any{"model": "grok-4", "messages": messages})
	_, written := grokRefusalIdentity(nil, raw)
	require.Len(t, written, 1)
	// A new question with the same public greeting is a different conversation.
	messages[3]["content"] = "new allowed request"
	other, _ := json.Marshal(map[string]any{"messages": messages})
	keys, _ := grokRefusalIdentity(nil, other)
	require.NotContains(t, keys, written[0])
	messages[3]["content"] = "refused request"
	for i := 0; i < 600; i++ {
		messages = append(messages, map[string]any{"role": "assistant", "content": fmt.Sprintf("turn %d", i)})
	}
	for _, protocol := range []string{"chat", "responses", "messages"} {
		t.Run(protocol, func(t *testing.T) {
			var payload map[string]any
			if protocol == "chat" {
				payload = map[string]any{"messages": messages}
			} else {
				copyItems := make([]map[string]any, 0, len(messages)-1)
				for _, m := range messages[1:] {
					copyItems = append(copyItems, map[string]any{"role": m["role"], "content": []any{map[string]any{"type": "text", "text": m["content"]}}})
				}
				if protocol == "responses" {
					payload = map[string]any{"instructions": " exact system ", "input": copyItems}
				} else {
					payload = map[string]any{"system": " exact system ", "messages": copyItems}
				}
			}
			raw, _ := json.Marshal(payload)
			keys, _ := grokRefusalIdentity(nil, raw)
			require.Contains(t, keys, written[0])
		})
	}
}

func TestGrokRefusalIdentityStrongIDsAndPreciseContent(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	c.Request.Header.Set("X-Session-Id", "same")
	c.Request.Header.Set("X-Conversation-Id", "same")
	body := []byte(`{"messages":[{"role":"user","content":" x "}],"prompt_cache_key":"shared","previous_response_id":"resp_shared"}`)
	_, write := grokRefusalIdentity(c, body)
	require.Len(t, write, 3)
	require.NotEqual(t, write[0], write[1])
	lookup, _ := grokRefusalIdentity(nil, body)
	require.Contains(t, lookup, write[2])
	trimmed := []byte(strings.ReplaceAll(string(body), `" x "`, `"x"`))
	lookup, _ = grokRefusalIdentity(nil, trimmed)
	require.NotContains(t, lookup, write[2])
	_, write = grokRefusalIdentity(nil, []byte(`{"input":"one","prompt_cache_key":"same"}`))
	require.Len(t, write, 1)
}

func TestGrokRefusalExcludesAccountAndIPBeforeApology(t *testing.T) {
	for _, prefix := range []string{"account suspended: ", "IP address blocked: ", "insufficient permissions: ", "blocked by global ip blacklist: "} {
		require.False(t, isGrokContentPolicyRejection(403, []byte(prefix+GrokModerationRefusalMessage())))
	}
	require.True(t, isGrokContentPolicyRejection(403, []byte("I'm sorry, I can't help with that request.")))
}

func TestGrokRefusalWSRequiresTypedHTTP403AndBoundIdentity(t *testing.T) {
	for _, mode := range []string{"valid", "forged_error", "non_403", "global_ip", "wrong_platform", "missing_binding", "commit_failure"} {
		t.Run(mode, func(t *testing.T) {
			svc, media, input, _ := newMediaBillingServiceTest(t)
			repo := &grokRefusalMemoryRepo{mediaJobMemoryRepo: media}
			svc.usageBillingRepo, svc.mediaBillingJobs = repo, repo
			input.Account.Platform = PlatformGrok
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/openai/v1/responses", nil)
			if mode != "missing_binding" {
				blocked, err := svc.CheckGrokRefusalWebSocketRequest(c, input.APIKey, []byte(`{"type":"response.create","input":"refuse"}`), "grok-4", nil)
				require.NoError(t, err)
				require.False(t, blocked)
			}
			upstream := &openAIWSHTTPBridgeUpstreamError{StatusCode: 403, ResponseBody: []byte(`{"error":{"code":"content_policy_violation","message":"explicit refusal"}}`)}
			var forwardErr error = upstream
			if mode == "forged_error" {
				forwardErr = fmt.Errorf("upstream status=403: %s", GrokModerationRefusalMessage())
			}
			if mode == "non_403" {
				upstream.StatusCode = 400
			}
			if mode == "global_ip" {
				upstream.ResponseBody = []byte("blocked by global ip blacklist: " + GrokModerationRefusalMessage())
			}
			if mode == "wrong_platform" {
				input.Account.Platform = PlatformOpenAI
			}
			if mode == "commit_failure" {
				repo.commitErr = errors.New("database unavailable")
			}
			err := svc.settleGrokRefusalWebSocketFailure(c, input.Account, forwardErr)
			switch mode {
			case "valid", "missing_binding", "commit_failure":
				require.True(t, IsGrokRefusalWSFailure(err))
				var closeErr *OpenAIWSClientCloseError
				require.ErrorAs(t, err, &closeErr)
				want := coderws.StatusInternalError
				if mode == "valid" {
					want = coderws.StatusPolicyViolation
				}
				require.Equal(t, want, closeErr.StatusCode())
			default:
				require.NoError(t, err)
			}
			if mode == "valid" {
				require.Len(t, repo.jobs, 1)
			} else {
				require.Empty(t, repo.jobs)
				require.Zero(t, repo.applied)
			}
		})
	}
}

func TestGrokRefusalFixedPriceIgnoresOtherPricingAndSubscription(t *testing.T) {
	for _, price := range []float64{0, .1, .37} {
		for _, mode := range []BillingMode{BillingModeToken, BillingModePerRequest} {
			t.Run(fmt.Sprintf("%s/%g", mode, price), func(t *testing.T) {
				svc, _, input, _ := newMediaBillingServiceTest(t)
				input.Result = &OpenAIForwardResult{Model: "unpriced-model", GrokModerationRefusalPrice: &price}
				input.APIKey.Group.SubscriptionType = SubscriptionTypeSubscription
				input.Subscription = &UserSubscription{ID: 5, UserID: input.User.ID, GroupID: *input.APIKey.GroupID}
				regular := 9.0
				input.APIKey.Group.ModelPricing = []ChannelModelPricing{{Models: []string{"*"}, BillingMode: mode, PerRequestPrice: &regular}}
				input.APIKey.Group.RateMultiplier = 13
				input.APIKey.Group.PeakRateEnabled = true
				input.APIKey.Group.PeakRateMultiplier = 7
				intent, err := svc.prepareMediaUsage(context.Background(), input)
				require.NoError(t, err)
				require.Equal(t, price, intent.Command.BalanceCost)
				require.Zero(t, intent.Command.SubscriptionCost)
				require.Nil(t, intent.Command.SubscriptionID)
				require.Nil(t, intent.Usage.SubscriptionID)
				require.Equal(t, BillingTypeBalance, intent.Usage.BillingType)
				require.Equal(t, float64(1), intent.Usage.RateMultiplier)
				require.Equal(t, string(BillingModePerRequest), *intent.Usage.BillingMode)
			})
		}
	}
	for _, invalid := range []float64{-1, math.NaN(), math.Inf(1)} {
		require.Error(t, validateGrokRefusalPrice(&invalid))
	}
}

type grokRefusalMemoryRepo struct {
	UsageBillingRepository
	*mediaJobMemoryRepo
	blocks    map[int64]map[string]string
	commitErr error
	lookupErr error
}

func (r *grokRefusalMemoryRepo) FindGrokRefusal(_ context.Context, user int64, keys []string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lookupErr != nil {
		return false, r.lookupErr
	}
	for _, key := range keys {
		if r.blocks[user][key] != "" {
			return true, nil
		}
	}
	return false, nil
}
func (r *grokRefusalMemoryRepo) CommitGrokRefusal(_ context.Context, job *MediaBillingJob, lookup, write []string) (*MediaBillingJob, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.commitErr != nil {
		return nil, false, r.commitErr
	}
	for _, key := range lookup {
		if id := r.blocks[job.UserID][key]; id != "" {
			return cloneMediaTestJob(r.jobs[id]), false, nil
		}
	}
	if r.blocks == nil {
		r.blocks = map[int64]map[string]string{}
	}
	if r.blocks[job.UserID] == nil {
		r.blocks[job.UserID] = map[string]string{}
	}
	if r.jobs == nil {
		r.jobs = map[string]*MediaBillingJob{}
	}
	job = cloneMediaTestJob(job)
	job.Status = MediaBillingBilled
	r.jobs[job.ID] = job
	r.applied++
	r.balance -= job.Intent.Command.BalanceCost
	for _, key := range write {
		r.blocks[job.UserID][key] = job.ID
	}
	return cloneMediaTestJob(job), true, nil
}
func refusalServiceContext(t *testing.T, svc *OpenAIGatewayService, input *OpenAIRecordUsageInput, body string) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	blocked, err := svc.CheckGrokRefusalRequest(c, input.APIKey, []byte(body), nil)
	require.NoError(t, err)
	require.False(t, blocked)
	return c
}
func TestGrokRefusalDurableRecoveryAndPreauthorizationOwnership(t *testing.T) {
	for _, mode := range []string{"success", "commit_failure", "apply_failure", "log_failure", "duplicate", "simple"} {
		t.Run(mode, func(t *testing.T) {
			svc, media, input, _ := newMediaBillingServiceTest(t)
			repo := &grokRefusalMemoryRepo{mediaJobMemoryRepo: media}
			svc.usageBillingRepo = repo
			svc.mediaBillingJobs = repo
			c := refusalServiceContext(t, svc, input, `{"model":"grok-4","messages":[{"role":"user","content":"refuse"}]}`)
			fixture := newPreauthorizationFixture()
			guard, err := fixture.service.Preauthorize(context.Background(), balancePreauthorizationTestRequest())
			require.NoError(t, err)
			if mode != "apply_failure" && mode != "log_failure" {
				c.Request = c.Request.WithContext(ContextWithBalancePreauthorizationGuard(c.Request.Context(), guard))
			}
			if mode == "commit_failure" {
				repo.commitErr = errors.New("commit unavailable")
			}
			if mode == "apply_failure" {
				media.applyErr = errors.New("apply unavailable")
			}
			if mode == "log_failure" {
				svc.usageLogRepo = nil
			}
			if mode == "simple" {
				svc.cfg.RunMode = config.RunModeSimple
			}
			err = svc.settleGrokRefusal(c, input.Account)
			if mode == "commit_failure" {
				require.Error(t, err)
				require.False(t, guard.IsTransferred())
				require.NoError(t, guard.Refund(context.Background()))
				require.Empty(t, repo.jobs)
				return
			}
			require.NoError(t, err)
			if mode != "apply_failure" && mode != "log_failure" {
				require.True(t, guard.IsTransferred())
				require.NoError(t, guard.Refund(context.Background()))
			}
			require.Equal(t, 1, repo.applied)
			if mode == "simple" {
				require.Equal(t, 1, fixture.wallet.refundCalls)
			} else {
				require.Equal(t, 0, fixture.wallet.refundCalls)
			}
			if mode == "duplicate" {
				c2, _ := gin.CreateTestContext(httptest.NewRecorder())
				c2.Request = httptest.NewRequest("POST", "/v1/messages", nil)
				c2.Set(grokRefusalRequestKey, c.MustGet(grokRefusalRequestKey))
				f2 := newPreauthorizationFixture()
				g2, e := f2.service.Preauthorize(context.Background(), balancePreauthorizationTestRequest())
				require.NoError(t, e)
				c2.Request = c2.Request.WithContext(ContextWithBalancePreauthorizationGuard(c2.Request.Context(), g2))
				require.NoError(t, svc.settleGrokRefusal(c2, input.Account))
				require.False(t, g2.IsTransferred())
				require.NoError(t, g2.Refund(context.Background()))
				require.Equal(t, 1, f2.wallet.refundCalls)
			}
			if mode == "apply_failure" || mode == "log_failure" {
				for _, job := range repo.jobs {
					require.Equal(t, MediaBillingBilled, job.Status)
				}
				media.applyErr = nil
				svc.usageLogRepo = &mediaUsageMemoryRepo{}
				svc.recoverMediaBillingJobs(context.Background())
				for _, job := range repo.jobs {
					require.Equal(t, MediaBillingDone, job.Status)
				}
				require.Equal(t, 1, repo.applied)
			}
		})
	}
}
