package repository

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type contentTestRoundTripper func(*http.Request) (*http.Response, error)

func (f contentTestRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestContentDownloadCrossOriginNeverRestoresCredentials(t *testing.T) {
	var received []*http.Request
	base := &http.Client{Transport: contentTestRoundTripper(func(r *http.Request) (*http.Response, error) {
		received = append(received, r.Clone(r.Context()))
		header := make(http.Header)
		status := http.StatusOK
		if len(received) == 1 {
			status = http.StatusFound
			header.Set("Location", "https://93.184.216.35/cdn")
		} else if len(received) == 2 {
			status = http.StatusFound
			header.Set("Location", "https://93.184.216.34/final")
		}
		return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader("video")), Request: r}, nil
	})}
	req, err := http.NewRequestWithContext(service.WithHTTPUpstreamPublicHostsOnly(t.Context()), http.MethodGet, "https://93.184.216.34/start", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("X-Relay-Token", "custom-secret")
	req.Header.Set("Range", "bytes=0-3")
	client, err := newContentDownloadClient(base, "", poolSettings{}, false, req.Header, defaultContentDownloadNetwork())
	require.NoError(t, err)
	client.Transport = base.Transport
	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Len(t, received, 3)
	for _, r := range received[1:] {
		require.Empty(t, r.Header.Get("X-Relay-Token"), "cross-origin custom credentials must be removed on every hop")
		require.Empty(t, r.Header.Get("Authorization"), "returning to the initial origin must not restore credentials")
		require.Equal(t, "bytes=0-3", r.Header.Get("Range"))
	}
}

type contentTestRequest struct {
	host, serverName string
	headers          http.Header
}

type contentTestNetworkState struct {
	mu       sync.Mutex
	dials    []string
	requests []contentTestRequest
}

func (s *contentTestNetworkState) snapshot() ([]string, []contentTestRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.dials...), append([]contentTestRequest(nil), s.requests...)
}

func contentTestTLSConfig(t *testing.T) (*tls.Config, *tls.Config) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		DNSNames: []string{"relay.example", "sub.relay.example", "cdn.example", "cdn2.example", "proxy.example"},
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, publicKey, privateKey)
	require.NoError(t, err)
	parsed, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: privateKey}}}, &tls.Config{RootCAs: roots}
}

// Only the dial destination is mapped to a local test listener. The real
// Transport still performs TLS verification, SNI, HTTP and client redirects.
func contentTestNetwork(t *testing.T, handler http.HandlerFunc) (contentDownloadNetwork, *contentTestNetworkState) {
	t.Helper()
	state := &contentTestNetworkState{}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		state.requests = append(state.requests, contentTestRequest{host: r.Host, serverName: r.TLS.ServerName, headers: r.Header.Clone()})
		state.mu.Unlock()
		handler(w, r)
	}))
	server.TLS, _ = contentTestTLSConfig(t)
	server.StartTLS()
	t.Cleanup(server.Close)
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	network := contentDownloadNetwork{
		lookup: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		},
		tlsConfig: &tls.Config{RootCAs: roots},
		dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			state.mu.Lock()
			state.dials = append(state.dials, address)
			state.mu.Unlock()
			host, _, err := net.SplitHostPort(address)
			if err != nil || net.ParseIP(host) == nil {
				return nil, errors.New("unvalidated destination hostname reached dialer")
			}
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
	}
	return network, state
}

func contentTestClient(t *testing.T, network contentDownloadNetwork, allowPrivateInitial bool) *http.Client {
	t.Helper()
	client, err := newContentDownloadClient(nil, "", defaultPoolSettings(nil), allowPrivateInitial, http.Header{"Accept": {"*/*"}, "Range": {"bytes=0-3"}}, network)
	require.NoError(t, err)
	return client
}

func TestContentDownloadOriginAndHeadersThroughRealTransport(t *testing.T) {
	for _, tc := range []struct {
		name, location string
		cross, deny    bool
	}{
		{"relative", "/end", false, false},
		{"explicit-default-port", "https://relay.example:443/end", false, false},
		{"subdomain", "https://sub.relay.example/end", true, false},
		{"new-port", "https://relay.example:8443/end", true, false},
		{"cross-host", "https://cdn.example/end", true, false},
		{"same-host-downgrade", "http://relay.example/end", true, true},
		{"cross-host-downgrade", "http://cdn.example/end", true, true},
		{"userinfo", "https://injected:secret@cdn.example/end", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			network, state := contentTestNetwork(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/start" {
					http.Redirect(w, r, tc.location, http.StatusFound)
					return
				}
				w.Header().Set("Content-Range", "bytes 0-3/10")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = io.WriteString(w, "vid!")
			})
			client := contentTestClient(t, network, false)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://relay.example/start", nil)
			require.NoError(t, err)
			req.Header = http.Header{"Authorization": {"Bearer secret"}, "X-Relay-Token": {"override-secret"}, "User-Agent": {"private-agent"}, "Cookie": {"session=secret"}, "Accept": {"override-secret"}, "Range": {"override-secret"}}
			resp, err := client.Do(req)
			if tc.deny {
				require.Error(t, err)
				if resp != nil {
					_ = resp.Body.Close()
				}
				dials, requests := state.snapshot()
				require.Len(t, dials, 1)
				require.Len(t, requests, 1)
				return
			}
			require.NoError(t, err)
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, http.StatusPartialContent, resp.StatusCode)
			require.Equal(t, "bytes 0-3/10", resp.Header.Get("Content-Range"))
			require.Equal(t, "vid!", string(body))
			dials, requests := state.snapshot()
			require.Len(t, requests, 2)
			require.Len(t, dials, 2)
			require.Equal(t, "8.8.8.8:443", dials[0])
			last := requests[1]
			if tc.cross {
				for _, name := range []string{"Authorization", "X-Relay-Token", "Cookie", "Referer"} {
					require.Empty(t, last.headers.Get(name), name)
				}
				require.NotEqual(t, "private-agent", last.headers.Get("User-Agent"))
				require.Equal(t, "*/*", last.headers.Get("Accept"))
				require.Equal(t, "bytes=0-3", last.headers.Get("Range"))
			} else {
				require.Equal(t, "Bearer secret", last.headers.Get("Authorization"))
				require.Equal(t, "override-secret", last.headers.Get("X-Relay-Token"))
				require.Equal(t, "private-agent", last.headers.Get("User-Agent"))
			}
			require.Equal(t, strings.Split(last.host, ":")[0], last.serverName, "Host and certificate-verified SNI keep the original hostname")
		})
	}
}

func TestContentDownloadMultihopNeverRestoresOverrides(t *testing.T) {
	locations := map[string]string{
		"/start": "https://sub.relay.example/hop1",
		"/hop1":  "https://cdn.example/hop2",
		"/hop2":  "https://relay.example/hop3",
		"/hop3":  "/end",
	}
	network, state := contentTestNetwork(t, func(w http.ResponseWriter, r *http.Request) {
		if location := locations[r.URL.Path]; location != "" {
			http.Redirect(w, r, location, http.StatusTemporaryRedirect)
			return
		}
		_, _ = io.WriteString(w, "video")
	})
	client := contentTestClient(t, network, false)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://relay.example/start", nil)
	require.NoError(t, err)
	req.Header = http.Header{"Authorization": {"Bearer secret"}, "X-Relay-Token": {"custom-secret"}, "User-Agent": {"private-agent"}, "Cookie": {"session=secret"}}
	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	_, requests := state.snapshot()
	require.Len(t, requests, 5)
	for _, r := range requests[1:] {
		for _, name := range []string{"Authorization", "X-Relay-Token", "Cookie", "Referer"} {
			require.Empty(t, r.headers.Get(name), name)
		}
		require.NotEqual(t, "private-agent", r.headers.Get("User-Agent"))
		require.Equal(t, "bytes=0-3", r.headers.Get("Range"))
	}
}

func TestContentDownloadRedirectLimit(t *testing.T) {
	for _, count := range []int{10, 11} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			network, state := contentTestNetwork(t, func(w http.ResponseWriter, r *http.Request) {
				index, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/"))
				if index < count {
					http.Redirect(w, r, "/"+strconv.Itoa(index+1), http.StatusPermanentRedirect)
					return
				}
				_, _ = io.WriteString(w, "video")
			})
			resp, err := contentTestClient(t, network, false).Get("https://relay.example/0")
			if count == 10 {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "10 content redirects")
			}
			if resp != nil {
				_ = resp.Body.Close()
			}
			dials, requests := state.snapshot()
			require.Len(t, requests, 11)
			require.Len(t, dials, 11)
		})
	}
}

func TestContentDownloadDNSRejectsEveryNonpublicAnswerBeforeDial(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answers []net.IPAddr
		err     error
	}{
		{"private", []net.IPAddr{{IP: net.ParseIP("10.0.0.1")}}, nil},
		{"metadata", []net.IPAddr{{IP: net.ParseIP("169.254.169.254")}}, nil},
		{"cgnat-metadata", []net.IPAddr{{IP: net.ParseIP("100.100.100.200")}}, nil},
		{"mixed", []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("192.168.1.1")}}, nil},
		{"ipv6-mixed", []net.IPAddr{{IP: net.ParseIP("2606:4700:4700::1111")}, {IP: net.ParseIP("::1")}}, nil},
		{"mapped-private", []net.IPAddr{{IP: net.ParseIP("::ffff:127.0.0.1")}}, nil},
		{"zoned", []net.IPAddr{{IP: net.ParseIP("8.8.8.8"), Zone: "eth0"}}, nil},
		{"invalid", []net.IPAddr{{IP: net.IP{1, 2}}}, nil},
		{"empty", nil, nil},
		{"lookup-error", nil, errors.New("dns unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var dials atomic.Int64
			network := contentDownloadNetwork{lookup: func(context.Context, string) ([]net.IPAddr, error) { return tc.answers, tc.err }, dial: func(context.Context, string, string) (net.Conn, error) {
				dials.Add(1)
				return nil, errors.New("must not dial")
			}}
			for _, proxyURL := range []string{"", "http://proxy.example:8080", "socks5h://proxy.example:1080"} {
				client, err := newContentDownloadClient(nil, proxyURL, poolSettings{}, false, nil, network)
				require.NoError(t, err)
				resp, err := client.Get("https://cdn.example/video")
				require.Error(t, err)
				require.Nil(t, resp)
			}
			require.Zero(t, dials.Load(), "fail closed before direct OR proxy connections")
		})
	}
}

func TestContentDownloadPinsDNSAndRevalidatesSameOrigin(t *testing.T) {
	network, state := contentTestNetwork(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/second", http.StatusFound) })
	lookups := 0
	network.lookup = func(context.Context, string) ([]net.IPAddr, error) {
		lookups++
		if lookups == 1 {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	resp, err := contentTestClient(t, network, false).Get("https://relay.example/start")
	require.ErrorContains(t, err, "not allowed")
	if resp != nil {
		_ = resp.Body.Close()
	}
	dials, requests := state.snapshot()
	require.Equal(t, []string{"8.8.8.8:443"}, dials)
	require.Len(t, requests, 1)
	require.Equal(t, 2, lookups, "resolve once for each hop, never again between validation and dial")
}

func TestContentDownloadPrivateInitialRelayPolicyAndLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name                                            string
		enabled, allowPrivate, relay, redirect, success bool
	}{
		{"configured-private-relay", true, true, true, false, true},
		{"legacy-disabled-allowlist", false, false, true, false, true},
		{"strict-initial-policy", true, false, true, false, false},
		{"signed-cdn-private-initial", true, true, false, false, false},
		{"private-redirect-from-private-relay", true, true, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if tc.redirect {
					http.Redirect(w, r, "/end", http.StatusFound)
					return
				}
				_, _ = io.WriteString(w, "video")
			}))
			t.Cleanup(server.Close)
			cfg := &config.Config{}
			cfg.Security.URLAllowlist.Enabled = tc.enabled
			cfg.Security.URLAllowlist.AllowPrivateHosts = tc.allowPrivate
			upstream := NewHTTPUpstream(cfg).(*httpUpstreamService)
			ctx := service.WithHTTPUpstreamContentDownload(t.Context(), tc.relay, http.Header{"Accept": {"*/*"}})
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/start", nil)
			require.NoError(t, err)
			resp, err := upstream.Do(req, "", 9, 1)
			if tc.success {
				require.NoError(t, err)
				for _, entry := range upstream.clients {
					require.EqualValues(t, 1, atomic.LoadInt64(&entry.inFlight))
				}
				require.NoError(t, resp.Body.Close())
				require.NoError(t, resp.Body.Close())
			} else {
				require.ErrorContains(t, err, "not allowed")
			}
			for _, entry := range upstream.clients {
				require.Zero(t, atomic.LoadInt64(&entry.inFlight))
			}
			if tc.success || tc.redirect {
				require.EqualValues(t, 1, calls.Load())
			} else {
				require.Zero(t, calls.Load())
			}
		})
	}
}

func TestContentDownloadRejectsHTTPSDowngrade(t *testing.T) {
	calls := 0
	base := &http.Client{Transport: contentTestRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": {"http://93.184.216.34/final"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("video")), Request: r}, nil
	})}
	req, err := http.NewRequestWithContext(service.WithHTTPUpstreamPublicHostsOnly(t.Context()), http.MethodGet, "https://93.184.216.34/start", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer secret")
	client, err := newContentDownloadClient(base, "", poolSettings{}, false, req.Header, defaultContentDownloadNetwork())
	require.NoError(t, err)
	client.Transport = base.Transport
	resp, err := client.Do(req)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	require.Error(t, err, "credential-bearing HTTPS must never redirect to HTTP")
	require.Equal(t, 1, calls)
}
