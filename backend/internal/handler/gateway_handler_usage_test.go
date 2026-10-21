package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUsageUnrestrictedIncludesWeeklyWindowStart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage", nil)

	weeklyWindowStart := time.Date(2026, time.July, 13, 0, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	c.Set(string(middleware.ContextKeySubscription), &service.UserSubscription{
		WeeklyWindowStart: &weeklyWindowStart,
	})

	handler := &GatewayHandler{}
	handler.usageUnrestricted(
		c,
		&service.APIKey{Group: &service.Group{
			Name:             "Weekly plan",
			SubscriptionType: service.SubscriptionTypeSubscription,
		}},
		nil,
		nil,
		nil,
		nil,
	)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Subscription struct {
			WeeklyWindowStart *time.Time `json:"weekly_window_start"`
		} `json:"subscription"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.NotNil(t, response.Subscription.WeeklyWindowStart)
	require.True(t, weeklyWindowStart.Equal(*response.Subscription.WeeklyWindowStart))
}

func TestUsageUnrestrictedRequiresPreloadedUser(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	h := &GatewayHandler{}
	h.usageUnrestricted(c, &service.APIKey{}, nil, nil, nil, nil)
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Failed to get user info")
}

type usageWalletContextKey struct{}

type usageWalletUserRepo struct {
	service.UserRepository
	events       *[]string
	deadline     time.Time
	hasDeadline  bool
	ctxErr       error
	requestValue any
	err          error
}

func (r *usageWalletUserRepo) GetByID(ctx context.Context, id int64) (*service.User, error) {
	*r.events = append(*r.events, "user")
	r.deadline, r.hasDeadline = ctx.Deadline()
	r.ctxErr = ctx.Err()
	r.requestValue = ctx.Value(usageWalletContextKey{})
	if r.err != nil {
		return nil, r.err
	}
	return &service.User{ID: id, Balance: 12.5, TokenVersionResolved: true}, nil
}

func (r *usageWalletUserRepo) GetUserAvatar(context.Context, int64) (*service.UserAvatar, error) {
	return nil, nil
}

type usageWalletStatsRepo struct {
	service.UsageLogRepository
	events *[]string
	cancel context.CancelFunc
}

func (r *usageWalletStatsRepo) GetAPIKeyDashboardStats(ctx context.Context, _ int64) (*usagestats.UserDashboardStats, error) {
	*r.events = append(*r.events, "stats")
	r.cancel()
	return nil, ctx.Err()
}

func (r *usageWalletStatsRepo) GetUsageTrendWithFilters(ctx context.Context, _, _ time.Time, _ string, _, _, _, _ int64, _ string, _ *int16, _ *bool, _ *int8) ([]usagestats.TrendDataPoint, error) {
	return nil, ctx.Err()
}

func (r *usageWalletStatsRepo) GetModelStatsWithFilters(ctx context.Context, _, _ time.Time, _, _, _, _ int64, _ *int16, _ *bool, _ *int8) ([]usagestats.ModelStat, error) {
	return nil, ctx.Err()
}

func TestUsageLoadsWalletBeforeStatsWithBoundedContext(t *testing.T) {
	for _, canceledBeforeLookup := range []bool{false, true} {
		name := "disconnect during statistics"
		if canceledBeforeLookup {
			name = "already disconnected"
		}
		t.Run(name, func(t *testing.T) {
			events := []string{}
			repo := &usageWalletUserRepo{events: &events}
			requestCtx, cancel := context.WithCancel(context.WithValue(context.Background(), usageWalletContextKey{}, "request-value"))
			defer cancel()
			if canceledBeforeLookup {
				cancel()
			}
			statsRepo := &usageWalletStatsRepo{events: &events, cancel: cancel}
			h := &GatewayHandler{userService: service.NewUserService(repo, nil, nil, nil), usageService: service.NewUsageService(statsRepo, nil, nil, nil)}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage", nil).WithContext(requestCtx)
			c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 7})
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 11})
			started := time.Now()
			h.Usage(c)
			require.Equal(t, []string{"user", "stats"}, events)
			require.NoError(t, repo.ctxErr)
			require.Equal(t, "request-value", repo.requestValue)
			require.True(t, repo.hasDeadline)
			require.WithinDuration(t, started.Add(5*time.Second), repo.deadline, time.Second)
			require.ErrorIs(t, requestCtx.Err(), context.Canceled)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.JSONEq(t, `{"mode":"unrestricted","isValid":true,"planName":"钱包余额","remaining":12.5,"unit":"USD","balance":12.5}`, recorder.Body.String())
		})
	}
}

func TestUsageWalletLookupFailureStopsBeforeStats(t *testing.T) {
	events := []string{}
	repo := &usageWalletUserRepo{events: &events, err: errors.New("wallet unavailable")}
	h := &GatewayHandler{userService: service.NewUserService(repo, nil, nil, nil)}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage", nil)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 7})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 11})
	h.Usage(c)
	require.Equal(t, []string{"user"}, events)
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
}

func TestUsageQuotaAndSubscriptionSkipWalletLookup(t *testing.T) {
	for _, key := range []*service.APIKey{
		{Quota: 10},
		{RateLimit5h: 1},
		{Group: &service.Group{Name: "Subscription", SubscriptionType: service.SubscriptionTypeSubscription}},
	} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage", nil)
		c.Set(string(middleware.ContextKeyAPIKey), key)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 11})
		h := &GatewayHandler{}
		h.Usage(c)
		require.Equal(t, http.StatusOK, recorder.Code)
	}
}
