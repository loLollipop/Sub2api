package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Only fake HTTP/WS upstream fixtures opt into this on their service instance.
func allowFakeAccountTestTarget(string) error { return nil }

func TestAccountTestTargetPolicy(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		cfg := &config.Config{}
		cfg.Security.URLAllowlist.Enabled = enabled
		cfg.Security.URLAllowlist.AllowInsecureHTTP = true
		cfg.Security.URLAllowlist.AllowPrivateHosts = true
		cfg.Security.URLAllowlist.UpstreamHosts = []string{"127.0.0.1", "localhost", "172.17.0.1", "169.254.169.254", "10.254.0.2", "100.100.100.200", "::1"}
		svc := &AccountTestService{cfg: cfg}
		for _, host := range []string{"127.0.0.1", "localhost", "172.17.0.1", "169.254.169.254", "10.254.0.2", "100.100.100.200", "[::1]"} {
			_, err := svc.validateUpstreamBaseURL("https://" + host + "/v1")
			require.Error(t, err, "allowlist=%v target=%s", enabled, host)
		}
	}
	cfg := &config.Config{}
	svc := &AccountTestService{cfg: cfg}
	_, err := svc.validateUpstreamBaseURL("https://8.8.8.8/v1")
	require.NoError(t, err)
	svc.validateTestTarget = func(string) error { return errors.New("DNS unavailable") }
	_, err = svc.validateUpstreamBaseURL("https://relay.example/v1")
	require.ErrorIs(t, err, errTestTargetNotAllowed)
}

func TestAdministrativeProbeHelpersCarrySocketPolicy(t *testing.T) {
	for _, name := range []string{"models", "adaptive", "websocket", "typesafe"} {
		t.Run(name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{err: errors.New("fixture transport stopped")}
			svc := &AccountTestService{httpUpstream: upstream, cfg: &config.Config{}, validateTestTarget: allowFakeAccountTestTarget}
			account := &Account{ID: 1, Platform: PlatformTypeSafe, Type: AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "fixture", "base_url": "https://relay.example"}}
			req := httptest.NewRequest(http.MethodGet, "https://relay.example/v1/models", nil).WithContext(context.Background())
			switch name {
			case "models":
				_, _ = svc.doUpstreamModelsRequest(req, "", account)
			case "adaptive":
				_, _ = svc.doCNProviderAdaptiveRequest(req, account)
			case "websocket":
				_, _ = (&accountTestWSTransport{service: svc, account: account}).RoundTrip(req)
			case "typesafe":
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = req
				_ = svc.testTypeSafeAccountConnection(c, account, "test")
			}
			require.NotNil(t, upstream.lastReq)
			require.True(t, HTTPUpstreamPublicHostsOnly(upstream.lastReq.Context()))
			require.True(t, HTTPUpstreamRedirectsDisabled(upstream.lastReq.Context()))
		})
	}
}
