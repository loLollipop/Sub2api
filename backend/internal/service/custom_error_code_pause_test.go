//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestCustomErrorCodeStopsSchedulingFor30Days(t *testing.T) {
	for _, status := range []int{http.StatusPaymentRequired, http.StatusInternalServerError, 529} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			repo := &errorPolicyRepoStub{}
			svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			account := &Account{
				ID: 77, Platform: PlatformDeepseek, Type: AccountTypeAPIKey,
				Credentials: map[string]any{
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(status)},
				},
			}

			before := time.Now()
			require.True(t, svc.HandleUpstreamError(context.Background(), account, status, http.Header{}, []byte(`{"error":{"message":"provider unavailable"}}`)))
			require.Zero(t, repo.setErrCalls)
			require.Equal(t, 1, repo.tempCalls)
			require.WithinDuration(t, before.Add(30*24*time.Hour), repo.lastTempUntil, 2*time.Second)
			require.Contains(t, repo.lastTempReason, customErrorCodeReasonPrefix+": Custom error code ")
			require.NotContains(t, repo.lastTempReason, cnBalanceLowReasonPrefix)

			// Unselected errors must neither shorten nor replace the explicit pause.
			require.False(t, svc.HandleUpstreamError(context.Background(), account, http.StatusServiceUnavailable, http.Header{}, nil))
			require.Equal(t, 1, repo.tempCalls)
			require.Zero(t, repo.setErrCalls)
		})
	}
}
