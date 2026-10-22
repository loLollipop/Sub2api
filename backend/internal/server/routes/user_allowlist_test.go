package routes

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserAllowlistRoutesRemoved(t *testing.T) {
	router := retiredFeatureRouteEnvironment(t)
	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/user/cf-allowlist"},
		{http.MethodPost, "/api/v1/user/cf-allowlist"},
		{http.MethodDelete, "/api/v1/user/cf-allowlist/1"},
	} {
		t.Run(route.method, func(t *testing.T) {
			response := retiredFeatureRouteRequest(router, route.method, route.path)
			require.Equal(t, http.StatusNotFound, response.Code)
		})
	}
}
