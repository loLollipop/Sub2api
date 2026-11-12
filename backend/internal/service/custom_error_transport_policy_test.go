//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTransportFailoverHonorsCustomErrorCodes(t *testing.T) {
	for _, platform := range []string{PlatformAnthropic, PlatformOpenAI} {
		for _, path := range []string{"anthropic", "gateway", "openai"} {
			for _, customCodes := range []bool{false, true} {
				name := platform + "/" + path + "/default"
				if customCodes {
					name = platform + "/" + path + "/custom"
				}
				t.Run(name, func(t *testing.T) {
					repo := &transportTempUnschedRepoStub{}
					openai := &OpenAIGatewayService{accountRepo: repo}
					rateLimit := &RateLimitService{accountRepo: repo, runtimeBlocker: openai}
					gateway := &GatewayService{accountRepo: repo, rateLimitService: rateLimit}
					account := &Account{ID: 719, Platform: platform, Type: AccountTypeAPIKey, Credentials: map[string]any{
						"custom_error_codes_enabled": customCodes,
						"custom_error_codes":         []any{float64(500)},
					}}
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
					transportErr := errors.New("dial tcp: connection refused")
					var err error
					switch path {
					case "anthropic":
						err = gateway.handleAnthropicUpstreamTransportError(context.Background(), c, account, "https://api.anthropic.com/v1/messages", false, transportErr)
					case "gateway":
						err = gateway.handleUpstreamTransportError(context.Background(), c, account, transportErr, OpsUpstreamErrorEvent{})
					case "openai":
						err = openai.handleOpenAIUpstreamTransportError(context.Background(), c, account, transportErr, false)
					}
					var failover *UpstreamFailoverError
					require.ErrorAs(t, err, &failover)
					require.Equal(t, http.StatusBadGateway, failover.StatusCode)
					require.False(t, c.Writer.Written(), "the handler still owns the failover response")
					require.Empty(t, rec.Body.String())
					if customCodes {
						require.Zero(t, repo.calls)
						require.False(t, openai.isOpenAIAccountRuntimeBlocked(account))
						// Ordinary selected statuses keep their existing 30-day pause.
						require.Equal(t, ErrorPolicySkipped, rateLimit.CheckErrorPolicy(context.Background(), account, 503, []byte(`{"error":"unavailable"}`)))
						require.Zero(t, repo.calls)
						require.Equal(t, ErrorPolicyMatched, rateLimit.CheckErrorPolicy(context.Background(), account, 500, []byte(`{"error":"unavailable"}`)))
						require.True(t, rateLimit.HandleUpstreamError(context.Background(), account, 500, http.Header{}, []byte(`{"error":"unavailable"}`)))
						require.Equal(t, 1, repo.calls)
						require.WithinDuration(t, time.Now().Add(customErrorCodeStopDuration), repo.lastUntil, time.Second)
						require.Contains(t, repo.lastReason, customErrorCodeReasonPrefix)
					} else {
						require.Equal(t, 1, repo.calls, "default durable-fault cooldown is retained")
					}
				})
			}
		}
	}
}
