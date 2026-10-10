package admin

import (
	"context"
	"encoding/json"
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
