//go:build unit

package admin

import (
	"github.com/gin-gonic/gin"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCustomEndpointMinRechargeValidation(t *testing.T) {
	require.True(t, validCustomEndpointMinRecharge(nil))
	for _, value := range []float64{0, 0.25, 1000, 10000000} {
		require.True(t, validCustomEndpointMinRecharge(&value), "value %v", value)
	}
	for _, value := range []float64{-0.01, 10000000.01, math.NaN(), math.Inf(1), math.Inf(-1)} {
		require.False(t, validCustomEndpointMinRecharge(&value), "value %v", value)
	}
}

func TestUpdateSettingsCustomEndpointRecharge(t *testing.T) {
	for _, value := range []float64{0, 0.25, 10000000, -1, 10000001} {
		t.Run(strconv.FormatFloat(value, 'g', -1, 64), func(t *testing.T) {
			h, repo := newStepUpSwitchTestHandler(t, map[string]string{service.SettingKeyCustomEndpoints: "[]"})
			rec := doUpdateSettings(t, h, map[string]any{"custom_endpoints": []map[string]any{{"name": "test", "endpoint": "https://example.com", "min_recharge": value}}}, nil)
			if value < 0 || value > 10000000 {
				require.Equal(t, http.StatusBadRequest, rec.Code)
				require.Equal(t, "[]", repo.values[service.SettingKeyCustomEndpoints])
			} else {
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				endpoints := dto.ParseCustomEndpoints(repo.values[service.SettingKeyCustomEndpoints])
				require.Len(t, endpoints, 1)
				require.NotNil(t, endpoints[0].MinRecharge)
				require.Equal(t, value, *endpoints[0].MinRecharge)
			}
		})
	}
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{service.SettingKeyCustomEndpoints: "[]"})
	rec := doUpdateSettings(t, h, map[string]any{"custom_endpoints": []map[string]any{{"name": "legacy", "endpoint": "https://example.com"}}}, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Nil(t, dto.ParseCustomEndpoints(repo.values[service.SettingKeyCustomEndpoints])[0].MinRecharge)
}

func TestUpdateSettingsRejectsNonFiniteEndpointJSON(t *testing.T) {
	for _, value := range []string{"NaN", "Infinity", "-Infinity", "1e309", `"NaN"`, `"Infinity"`} {
		t.Run(value, func(t *testing.T) {
			h, repo := newStepUpSwitchTestHandler(t, map[string]string{service.SettingKeyCustomEndpoints: "[]"})
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", strings.NewReader(`{"custom_endpoints":[{"name":"test","endpoint":"https://example.com","min_recharge":`+value+`}]}`))
			c.Request.Header.Set("Content-Type", "application/json")
			h.UpdateSettings(c)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Equal(t, "[]", repo.values[service.SettingKeyCustomEndpoints])
		})
	}
}
