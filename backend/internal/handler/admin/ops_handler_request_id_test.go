package admin

import (
	"context"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type requestIDOpsRepo struct {
	service.OpsRepository
	got    string
	filter *service.OpsErrorLogFilter
	detail *service.OpsErrorLogDetail
}

func (r *requestIDOpsRepo) ListErrorLogs(_ context.Context, f *service.OpsErrorLogFilter) (*service.OpsErrorLogList, error) {
	r.got = f.RequestID
	r.filter = f
	return &service.OpsErrorLogList{Page: 1, PageSize: 20}, nil
}

func (r *requestIDOpsRepo) GetErrorLogByID(context.Context, int64) (*service.OpsErrorLogDetail, error) {
	return r.detail, nil
}

func TestOpsErrorListsValidateRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, name := range []string{"all", "request", "upstream"} {
		for _, tc := range []struct {
			id, window string
			status     int
		}{
			{"ab", "30d", 400}, {"client:a", "30d", 400}, {"%_", "30d", 400},
			{"=", "1h", 400}, {"^", "1h", 400},
			{"ab", "1h", 200}, {"%_", "1h", 200}, {"=a", "30d", 200},
			{"abc", "30d", 200}, {"^client:abc", "30d", 200},
			{"ab", "empty", 400},
		} {
			t.Run(name+tc.id+tc.window, func(t *testing.T) {
				repo := &requestIDOpsRepo{}
				svc := service.NewOpsService(repo, nil, &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
				h := NewOpsHandler(svc)
				r := gin.New()
				r.GET("/errors", map[string]gin.HandlerFunc{"all": h.GetErrorLogs, "request": h.ListRequestErrors, "upstream": h.ListUpstreamErrors}[name])
				q := url.Values{"request_id": {tc.id}, "time_range": {tc.window}}
				if tc.window == "empty" {
					q.Set("start_time", "2026-10-13T00:00:00Z")
					q.Set("end_time", "2026-10-13T00:00:00Z")
				}
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest("GET", "/errors?"+q.Encode(), nil))
				if w.Code != tc.status {
					t.Fatalf("status %d: %s", w.Code, w.Body.String())
				}
				if tc.status == 400 && repo.filter != nil {
					t.Fatal("invalid search reached repository")
				}
				if tc.status == 200 && repo.got != tc.id {
					t.Fatalf("filter %q", repo.got)
				}
			})
		}
	}
}

func TestOpsLinkedUpstreamRequestIDRemainsExact(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &requestIDOpsRepo{detail: &service.OpsErrorLogDetail{OpsErrorLog: service.OpsErrorLog{RequestID: "abc%_", ClientRequestID: "client-other"}}}
	svc := service.NewOpsService(repo, nil, &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
	h := NewOpsHandler(svc)
	r := gin.New()
	r.GET("/errors/:id/upstream", h.ListRequestErrorUpstreamErrors)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/errors/1/upstream", nil))
	if w.Code != 200 || repo.got != "=abc%_" {
		t.Fatalf("status=%d request ID=%q body=%s", w.Code, repo.got, w.Body.String())
	}
	if repo.filter.Owner != "provider" || !repo.filter.IncludeRecoveredUpstream {
		t.Fatalf("provider scope changed: %+v", repo.filter)
	}
}
func TestOpsErrorListsForwardRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, name := range []string{"all", "request", "upstream"} {
		t.Run(name, func(t *testing.T) {
			repo := &requestIDOpsRepo{}
			svc := service.NewOpsService(repo, nil, &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
			h := NewOpsHandler(svc)
			handlers := map[string]gin.HandlerFunc{"all": h.GetErrorLogs, "request": h.ListRequestErrors, "upstream": h.ListUpstreamErrors}
			r := gin.New()
			r.GET("/errors", handlers[name])
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/errors?request_id=client%3Auuid", nil))
			if w.Code != 200 || repo.got != "client:uuid" {
				t.Fatalf("status %d filter %q", w.Code, repo.got)
			}
		})
	}
}
