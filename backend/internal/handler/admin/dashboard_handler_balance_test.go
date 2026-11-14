package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type dashboardBalanceUsageRepo struct {
	service.UsageLogRepository
	stats *usagestats.DashboardStats
}

func (r *dashboardBalanceUsageRepo) GetDashboardStats(context.Context) (*usagestats.DashboardStats, error) {
	return r.stats, nil
}

func TestDashboardHandlerGetStatsExposesFundedUserBalance(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count int64
		total float64
	}{
		{name: "funded users", count: 2, total: 14.75},
		{name: "zero balance"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &dashboardBalanceUsageRepo{stats: &usagestats.DashboardStats{
				TotalUsers: 7, UsersWithBalance: tc.count, UsersWithBalanceTotal: tc.total,
			}}
			handler := NewDashboardHandler(service.NewDashboardService(repo, nil, nil, nil), nil)
			router := gin.New()
			router.GET("/admin/dashboard/stats", handler.GetStats)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/admin/dashboard/stats", nil))
			require.Equal(t, http.StatusOK, recorder.Code)

			var response struct {
				Data map[string]json.RawMessage `json:"data"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
			require.Contains(t, response.Data, "users_with_balance")
			require.Contains(t, response.Data, "users_with_balance_total")
			var count int64
			var total float64
			require.NoError(t, json.Unmarshal(response.Data["users_with_balance"], &count))
			require.NoError(t, json.Unmarshal(response.Data["users_with_balance_total"], &total))
			require.Equal(t, tc.count, count)
			require.Equal(t, tc.total, total)
		})
	}
}
