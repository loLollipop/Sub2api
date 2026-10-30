//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestParseCustomEndpointsPreservesMinRecharge(t *testing.T) {
	items := dto.ParseCustomEndpoints(`[
		{"name":"public","endpoint":"https://public.example.com","description":"","min_recharge":0},
		{"name":"legacy","endpoint":"https://legacy.example.com","description":""}
	]`)

	require.Len(t, items, 2)
	require.NotNil(t, items[0].MinRecharge)
	require.Equal(t, 0.0, *items[0].MinRecharge)
	require.Nil(t, items[1].MinRecharge)
}

func TestFilterCustomEndpointsByRecharge(t *testing.T) {
	zero := 0.0
	fiveHundred := 500.0
	twoThousand := 2000.0

	endpoints := []dto.CustomEndpoint{
		{Name: "public", Endpoint: "https://public.example.com", MinRecharge: &zero},
		{Name: "legacy", Endpoint: "https://legacy.example.com"},
		{Name: "mid", Endpoint: "https://mid.example.com", MinRecharge: &fiveHundred},
		{Name: "vip", Endpoint: "https://vip.example.com", MinRecharge: &twoThousand},
	}

	t.Run("zero recharge only sees endpoints open to everyone", func(t *testing.T) {
		got := filterCustomEndpointsByRecharge(endpoints, 0)
		require.Len(t, got, 1)
		require.Equal(t, "public", got[0].Name)
	})

	t.Run("partial recharge unlocks the matching tier", func(t *testing.T) {
		got := filterCustomEndpointsByRecharge(endpoints, 600)
		require.Len(t, got, 2)
		require.Equal(t, []string{"public", "mid"}, []string{got[0].Name, got[1].Name})
	})

	t.Run("legacy endpoint without threshold keeps the historical default", func(t *testing.T) {
		got := filterCustomEndpointsByRecharge(endpoints, service.CustomEndpointRechargeThreshold)
		require.Len(t, got, 3)
		require.Equal(t, []string{"public", "legacy", "mid"}, []string{got[0].Name, got[1].Name, got[2].Name})
	})

	t.Run("high recharge sees every endpoint in configured order", func(t *testing.T) {
		got := filterCustomEndpointsByRecharge(endpoints, 100000)
		require.Len(t, got, 4)
		require.Equal(t, []string{"public", "legacy", "mid", "vip"}, []string{got[0].Name, got[1].Name, got[2].Name, got[3].Name})
	})

	t.Run("empty input returns a non-nil empty slice", func(t *testing.T) {
		got := filterCustomEndpointsByRecharge(nil, 0)
		require.NotNil(t, got)
		require.Empty(t, got)
	})
}

type customEndpointsRepoStub struct{ settingHandlerPublicRepoStub }

func (s *customEndpointsRepoStub) GetValue(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}

func TestGetCustomEndpointsAuthenticatesAndFiltersPerUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &customEndpointsRepoStub{settingHandlerPublicRepoStub{values: map[string]string{
		service.SettingKeyCustomEndpoints: `[{"name":"public","endpoint":"https://public.example","min_recharge":0},{"name":"legacy","endpoint":"https://legacy.example"},{"name":"tier","endpoint":"https://tier.example","min_recharge":500}]`,
	}}}
	for _, tc := range []struct {
		name     string
		userID   int64
		recharge float64
		names    []string
	}{
		{"anonymous", 0, 10000, nil},
		{"new user", 1, 0, []string{"public"}},
		{"below boundary", 2, 499.99, []string{"public"}},
		{"at boundary", 3, 500, []string{"public", "tier"}},
		{"legacy boundary", 4, 1000, []string{"public", "legacy", "tier"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewSettingHandler(service.NewSettingService(repo, &config.Config{}), "test")
			h.SetUserService(service.NewUserService(&userHandlerRepoStub{user: &service.User{ID: tc.userID, TotalRecharged: tc.recharge}}, nil, nil, nil))
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/user/custom-endpoints", nil)
			if tc.userID > 0 {
				c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: tc.userID})
			}
			h.GetCustomEndpoints(c)
			require.Equal(t, "private, no-store", rec.Header().Get("Cache-Control"))
			if tc.userID == 0 {
				require.Equal(t, http.StatusUnauthorized, rec.Code)
				return
			}
			require.Equal(t, http.StatusOK, rec.Code)
			var result struct {
				Data []dto.CustomEndpoint `json:"data"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
			names := make([]string, 0, len(result.Data))
			for _, endpoint := range result.Data {
				names = append(names, endpoint.Name)
			}
			require.Equal(t, tc.names, names)
		})
	}
	// Public settings must keep every configured endpoint hidden, including the zero tier.
	h := NewSettingHandler(service.NewSettingService(repo, &config.Config{}), "test")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)
	h.GetPublicSettings(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "public.example")
	require.NotContains(t, rec.Body.String(), "legacy.example")
}
