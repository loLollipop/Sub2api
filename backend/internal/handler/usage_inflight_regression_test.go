package handler

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

type userUsageInflightStore struct{ calls int }

func (s *userUsageInflightStore) Save(context.Context, service.UsageInflightSnapshot) error {
	return nil
}
func (s *userUsageInflightStore) Delete(context.Context, int64, string) error { return nil }
func (s *userUsageInflightStore) ListUser(context.Context, int64) ([]service.UsageInflightSnapshot, error) {
	s.calls++
	return []service.UsageInflightSnapshot{{RequestID: "client:abc", UserID: 42, StartedAt: time.Now()}}, nil
}
func (s *userUsageInflightStore) ListAll(ctx context.Context) ([]service.UsageInflightSnapshot, error) {
	return s.ListUser(ctx, 42)
}

func TestUserUsageInflightExplicitFalseWinsRequestID(t *testing.T) {
	store := &userUsageInflightStore{}
	service.SetUsageInflightStore(store)
	t.Cleanup(func() { service.SetUsageInflightStore(nil) })
	r := newUserUsageRequestTypeTestRouter(&userUsageRepoCapture{})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/usage?request_id=abc&include_inflight=false", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Zero(t, store.calls, "request_id must not override explicit exclusion used by exports")
}

func TestUserUsageInflightExtremePageUsesFinalPageSize(t *testing.T) {
	store := &userUsageInflightStore{}
	service.SetUsageInflightStore(store)
	t.Cleanup(func() { service.SetUsageInflightStore(nil) })
	r := newUserUsageRequestTypeTestRouter(&userUsageRepoCapture{})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/usage?inflight_only=true&page=922337203685477582&page_size=1", nil))
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
