package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type requestIDErrorSettingRepo struct{ service.SettingRepository }

func (*requestIDErrorSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{service.SettingKeyAllowUserViewErrorRequests: "true"}, nil
}

type userRequestIDErrorRepo struct {
	service.OpsRepository
	filter *service.OpsErrorLogFilter
}

func (r *userRequestIDErrorRepo) ListErrorLogs(_ context.Context, filter *service.OpsErrorLogFilter) (*service.OpsErrorLogList, error) {
	r.filter = filter
	return &service.OpsErrorLogList{Page: 1, PageSize: 20}, nil
}

func TestUserUsageRequestIDValidationAllLists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"/usage", "/usage?inflight_only=true", "/usage/errors"} {
		for _, tc := range []struct {
			input, start, end string
			status            int
		}{
			{"ab", "", "", 400}, {"client:a", "", "", 400}, {"%_", "", "", 400},
			{"=", "2026-10-13", "2026-10-13", 400}, {"^", "2026-10-13", "2026-10-13", 400},
			{"ab", "2026-10-13", "2026-10-14", 400},
			{"ab", "2026-10-14", "2026-10-12", 400},
			{"ab", "2026-10-14", "2026-10-13", 400},
			{"ab", "2026-10-13", "2026-10-13", 200},
			{"=a", "", "", 200}, {"^abc", "", "", 200}, {"abc", "", "", 200},
		} {
			t.Run(endpoint+tc.input+tc.start+tc.end, func(t *testing.T) {
				repo := &userUsageRepoCapture{}
				errRepo := &userRequestIDErrorRepo{}
				ops := service.NewOpsService(errRepo, nil, &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
				h := NewUsageHandler(service.NewUsageService(repo, nil, nil, nil), nil, ops, service.NewSettingService(&requestIDErrorSettingRepo{}, nil))
				router := gin.New()
				router.Use(func(c *gin.Context) {
					c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})
					c.Next()
				})
				router.GET("/usage", h.List)
				router.GET("/usage/errors", h.ListErrors)
				u, err := url.Parse(endpoint)
				require.NoError(t, err)
				q := u.Query()
				q.Set("request_id", tc.input)
				q.Set("start_date", tc.start)
				q.Set("end_date", tc.end)
				q.Set("timezone", "UTC")
				u.RawQuery = q.Encode()
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, u.String(), nil))
				require.Equal(t, tc.status, w.Code, w.Body.String())
				if tc.status == 400 {
					require.Nil(t, errRepo.filter)
					require.Empty(t, repo.listFilters.RequestID)
				}
				if tc.status == 200 && endpoint == "/usage/errors" {
					require.NotNil(t, errRepo.filter.UserID)
					require.Equal(t, int64(42), *errRepo.filter.UserID)
					require.True(t, errRepo.filter.ExcludeCountTokens)
					require.False(t, errRepo.filter.IncludeRecoveredUpstream)
					require.Equal(t, tc.input, errRepo.filter.RequestID)
				}
			})
		}
	}
}
