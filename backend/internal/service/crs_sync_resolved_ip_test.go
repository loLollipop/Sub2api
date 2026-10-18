//go:build unit

package service

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// Private CRS targets require explicit CRSHosts even when global private access is enabled.
func TestCRSSyncResolvedIPValidationIsNotTiedToAllowlist(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)

	t.Run("allowlist off plus private hosts disallowed still blocks loopback", func(t *testing.T) {
		t.Parallel()

		cfg := &config.Config{}
		cfg.Security.URLAllowlist.Enabled = false // allowlist 关着
		cfg.Security.URLAllowlist.AllowInsecureHTTP = true
		cfg.Security.URLAllowlist.AllowPrivateHosts = false // 但私网不允许

		svc := NewCRSSyncService(newCRSLongContextAccountRepo(), nil, nil, nil, nil, cfg)
		_, err := svc.SyncFromCRS(context.Background(), SyncFromCRSInput{
			BaseURL:  server.URL, // 127.0.0.1 —— 环回
			Username: "admin",
			Password: "password",
		})

		require.Error(t, err, "closing the allowlist must not silently disable DNS-rebinding protection")
		require.Contains(t, err.Error(), "blocked_destination",
			"the loopback destination must be rejected by the resolved-IP check")
	})

	t.Run("global private hosts allowed still blocks loopback", func(t *testing.T) {
		t.Parallel()

		cfg := &config.Config{}
		cfg.Security.URLAllowlist.Enabled = false
		cfg.Security.URLAllowlist.AllowInsecureHTTP = true
		cfg.Security.URLAllowlist.AllowPrivateHosts = true

		svc := NewCRSSyncService(newCRSLongContextAccountRepo(), nil, nil, nil, nil, cfg)
		_, err := svc.SyncFromCRS(context.Background(), SyncFromCRSInput{
			BaseURL:  server.URL,
			Username: "admin",
			Password: "password",
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "blocked_destination")
	})
}

func TestCRSSyncExplicitPrivateHostAndRedirectPolicy(t *testing.T) {
	var targetHits int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetHits++; w.Write([]byte("internal secret")) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Security.URLAllowlist.CRSHosts = []string{"127.0.0.1"}
	svc := NewCRSSyncService(nil, nil, nil, nil, nil, cfg)
	_, err := svc.fetchCRSExport(context.Background(), source.URL, "admin", "password")
	require.ErrorContains(t, err, "upstream returned 307")
	require.NotContains(t, err.Error(), "internal secret")
	require.Zero(t, targetHits)
}

func TestCRSExplicitHostPolicy(t *testing.T) {
	require.True(t, crsHostExplicitlyAllowed("https://relay.internal:8443", []string{"relay.internal"}))
	require.True(t, crsHostExplicitlyAllowed("https://[::1]:8443", []string{"[::1]:8443"}))
	require.False(t, crsHostExplicitlyAllowed("https://relay.internal", []string{"*.internal"}))
	require.False(t, crsHostExplicitlyAllowed("https://relay.internal.evil", []string{"relay.internal"}))
}

func TestCRSSyncPinsDNSAndRejectsRebinding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/web/auth/login", r.URL.Path)
		w.Header().Set("Connection", "close")
		w.Write([]byte(`{"success":true,"token":"secret"}`))
	}))
	defer server.Close()
	calls, dials := 0, 0
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	svc := NewCRSSyncService(nil, nil, nil, nil, nil, cfg)
	svc.crsNetwork = &customUsageNetwork{
		lookup: func(context.Context, string) ([]net.IPAddr, error) {
			calls++
			if calls == 1 {
				return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
			}
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		},
		dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			dials++
			require.Equal(t, "8.8.8.8:80", address, "the actual dial must receive the validated IP")
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
	}
	_, err := svc.fetchCRSExport(context.Background(), "http://relay.example", "admin", "password")
	require.ErrorContains(t, err, "blocked_destination")
	require.Equal(t, 2, calls)
	require.Equal(t, 1, dials, "rebinding must be rejected before the second socket connects")
	svc.crsNetwork.lookup = func(context.Context, string) ([]net.IPAddr, error) { return nil, errors.New("DNS unavailable") }
	_, err = svc.fetchCRSExport(context.Background(), "http://relay.example", "admin", "password")
	require.ErrorContains(t, err, "dns_failed")
	require.Equal(t, 1, dials)
}
