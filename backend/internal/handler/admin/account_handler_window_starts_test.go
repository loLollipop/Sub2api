package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type accountListWindowStatsRepo struct {
	service.UsageLogRepository
	starts map[int64]time.Time
	calls  int
}

func (r *accountListWindowStatsRepo) GetAccountWindowStatsByStart(_ context.Context, starts map[int64]time.Time) (map[int64]*usagestats.AccountStats, error) {
	r.calls++
	r.starts = starts
	return map[int64]*usagestats.AccountStats{1: {StandardCost: 2.5}, 2: {}}, nil
}
func TestAccountHandlerListBatchesDistinctWindowStartsAndZeroCost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	start1 := now.Add(-time.Hour)
	start2 := now.Add(-2 * time.Hour)
	end := now.Add(time.Hour)
	admin := newStubAdminService()
	admin.accounts = []service.Account{
		{ID: 1, Platform: service.PlatformAnthropic, Type: service.AccountTypeOAuth, Extra: map[string]any{"window_cost_limit": 10.0}, SessionWindowStart: &start1, SessionWindowEnd: &end},
		{ID: 2, Platform: service.PlatformAnthropic, Type: service.AccountTypeSetupToken, Extra: map[string]any{"window_cost_limit": 10.0}, SessionWindowStart: &start2, SessionWindowEnd: &end},
		{ID: 3, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Extra: map[string]any{"window_cost_limit": 10.0}},
	}
	repo := &accountListWindowStatsRepo{}
	usage := service.NewAccountUsageService(nil, repo, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := &AccountHandler{adminService: admin, accountUsageService: usage}
	router := gin.New()
	router.GET("/accounts", h.List)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/accounts?page=1&page_size=20", nil))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, 1, repo.calls)
	require.Equal(t, map[int64]time.Time{1: start1, 2: start2}, repo.starts)
	require.Equal(t, 2.5, gjson.GetBytes(rec.Body.Bytes(), "data.items.0.current_window_cost").Float())
	require.True(t, gjson.GetBytes(rec.Body.Bytes(), "data.items.1.current_window_cost").Exists())
	require.Zero(t, gjson.GetBytes(rec.Body.Bytes(), "data.items.1.current_window_cost").Float())
	require.False(t, gjson.GetBytes(rec.Body.Bytes(), "data.items.2.current_window_cost").Exists())
}
