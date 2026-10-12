package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type scheduledDefaultOffPlans struct {
	service.ScheduledTestPlanRepository
	saved *service.ScheduledTestPlan
}

func (r *scheduledDefaultOffPlans) Create(_ context.Context, plan *service.ScheduledTestPlan) (*service.ScheduledTestPlan, error) {
	r.saved = plan
	return plan, nil
}

type scheduledDefaultOffAdmin struct{ service.AdminService }

func (*scheduledDefaultOffAdmin) GetAccount(_ context.Context, id int64) (*service.Account, error) {
	return &service.Account{ID: id}, nil
}

func TestScheduledTestCreateRequiresExplicitQualityOptIn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name  string
		field string
		want  bool
	}{
		{"omitted", "", false},
		{"false", `,"quality_check_enabled":false`, false},
		{"true", `,"quality_check_enabled":true`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plans := &scheduledDefaultOffPlans{}
			h := NewScheduledTestHandler(service.NewScheduledTestService(plans, nil), &scheduledDefaultOffAdmin{})
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 2})
			})
			router.POST("/plans", h.Create)
			body := `{"account_id":22,"model_id":"gpt-test","cron_expression":"*/5 * * * *"` + tc.field + `}`
			req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.NotNil(t, plans.saved)
			require.Equal(t, tc.want, plans.saved.QualityCheckEnabled)
			require.Equal(t, service.QualityProviderChanshui, plans.saved.QualityProvider)
		})
	}
}
