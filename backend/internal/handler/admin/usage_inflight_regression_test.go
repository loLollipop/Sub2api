package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type adminUsageInflightStore struct{ calls int }

func (s *adminUsageInflightStore) Save(context.Context, service.UsageInflightSnapshot) error {
	return nil
}

type adminUsageManyInflightStore struct {
	adminUsageInflightStore
	rows []service.UsageInflightSnapshot
}

func (s *adminUsageManyInflightStore) ListAll(context.Context) ([]service.UsageInflightSnapshot, error) {
	return s.rows, nil
}

func TestAdminUsageInflightSupplementIsBoundedAfterFiltering(t *testing.T) {
	now := time.Now()
	store := &adminUsageManyInflightStore{}
	for i := 0; i < 80; i++ {
		store.rows = append(store.rows, service.UsageInflightSnapshot{
			RequestID: fmt.Sprintf("request-%d", i), UserID: 42, Model: "target",
			StartedAt: now.Add(-time.Duration(i) * time.Second),
		})
	}
	// Newer nonmatching rows must not consume the supplement limit.
	for i := 0; i < 60; i++ {
		store.rows = append(store.rows, service.UsageInflightSnapshot{
			RequestID: fmt.Sprintf("other-%d", i), UserID: 42, Model: "other", StartedAt: now,
		})
	}
	service.SetUsageInflightStore(store)
	t.Cleanup(func() { service.SetUsageInflightStore(nil) })
	router := newAdminUsageRequestTypeTestRouter(&adminUsageRepoCapture{})
	for _, tc := range []struct {
		query string
		count int
		total int
		first string
	}{
		{query: "include_inflight=true&model=target&page_size=10", count: 50, first: "request-0"},
		{query: "include_inflight=true&model=target&page=2&page_size=10"},
		{query: "inflight_only=true&model=target&page=6&page_size=10", count: 10, total: 80, first: "request-50"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/usage?"+tc.query, nil))
			require.Equal(t, http.StatusOK, rec.Code)
			var body struct {
				Data struct {
					Items []struct {
						RequestID string `json:"request_id"`
					} `json:"items"`
					Total int `json:"total"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Len(t, body.Data.Items, tc.count)
			require.Equal(t, tc.total, body.Data.Total)
			if tc.count > 0 {
				require.Equal(t, tc.first, body.Data.Items[0].RequestID)
			}
		})
	}
}
func (s *adminUsageInflightStore) Delete(context.Context, int64, string) error { return nil }
func (s *adminUsageInflightStore) ListUser(context.Context, int64) ([]service.UsageInflightSnapshot, error) {
	s.calls++
	return []service.UsageInflightSnapshot{{RequestID: "client:abc", UserID: 42, StartedAt: time.Now()}}, nil
}
func (s *adminUsageInflightStore) ListAll(ctx context.Context) ([]service.UsageInflightSnapshot, error) {
	return s.ListUser(ctx, 42)
}

func TestAdminUsageInflightExplicitFalseWinsRequestID(t *testing.T) {
	store := &adminUsageInflightStore{}
	service.SetUsageInflightStore(store)
	t.Cleanup(func() { service.SetUsageInflightStore(nil) })
	r := newAdminUsageRequestTypeTestRouter(&adminUsageRepoCapture{})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/usage?request_id=abc&include_inflight=false", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Zero(t, store.calls, "request_id must not override explicit exclusion used by exports")
}

func TestAdminUsageInflightExtremePageUsesFinalPageSize(t *testing.T) {
	store := &adminUsageInflightStore{}
	service.SetUsageInflightStore(store)
	t.Cleanup(func() { service.SetUsageInflightStore(nil) })
	r := newAdminUsageRequestTypeTestRouter(&adminUsageRepoCapture{})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/usage?inflight_only=true&page=922337203685477582&page_size=1", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data struct {
			Items    []json.RawMessage `json:"items"`
			PageSize int               `json:"page_size"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Empty(t, body.Data.Items)
	require.Equal(t, 10, body.Data.PageSize)
}
