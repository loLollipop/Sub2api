package repository

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func TestPublicProbePreservesGrokAccessDeniedFallback(t *testing.T) {
	var hosts, destinations []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		hosts = append(hosts, req.Host)
		if len(hosts) == 1 {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"Access denied"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"ok"}`))
	}))
	defer server.Close()
	network := defaultContentDownloadNetwork()
	transport, ok := server.Client().Transport.(*http.Transport)
	require.True(t, ok)
	network.tlsConfig = transport.TLSClientConfig.Clone()
	network.tlsConfig.ServerName = server.Certificate().IPAddresses[0].String()
	network.lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}
	network.dial = func(ctx context.Context, proto, address string) (net.Conn, error) {
		destinations = append(destinations, address)
		return (&net.Dialer{}).DialContext(ctx, proto, server.Listener.Addr().String())
	}
	upstream, ok := NewHTTPUpstream(nil).(*httpUpstreamService)
	require.True(t, ok)
	upstream.publicNetwork = &network
	req, err := http.NewRequest(http.MethodPost, "https://"+grokCLIProxyHost+"/v1/responses", bytes.NewBufferString(`{"input":"ping"}`))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer fixture")
	req = req.WithContext(service.WithHTTPUpstreamRedirectsDisabled(service.WithHTTPUpstreamPublicHostsOnly(req.Context())))
	resp, err := upstream.Do(req, "", 42, 1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, []string{grokCLIProxyHost, grokOfficialAPIHost}, hosts)
	require.Equal(t, []string{"8.8.8.8:443", "8.8.8.8:443"}, destinations)
}

func TestPublicProbeTransportPinsSocketAndPreservesDuplex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		kind, message, err := conn.Read(r.Context())
		if err == nil {
			_ = conn.Write(r.Context(), kind, message)
		}
	}))
	defer server.Close()
	var destinations []string
	network := defaultContentDownloadNetwork()
	network.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		destinations = append(destinations, address)
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	upstream, ok := NewHTTPUpstream(nil).(*httpUpstreamService)
	require.True(t, ok)
	upstream.publicNetwork = &network
	client := &http.Client{Transport: probeTestRoundTripper{upstream}}
	conn, _, err := websocket.Dial(t.Context(), "ws://8.8.8.8/probe", &websocket.DialOptions{HTTPClient: client})
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()
	require.NoError(t, conn.Write(t.Context(), websocket.MessageText, []byte("ping")))
	_, message, err := conn.Read(t.Context())
	require.NoError(t, err)
	require.Equal(t, "ping", string(message))
	require.Equal(t, []string{"8.8.8.8:80"}, destinations)
}

type probeTestRoundTripper struct{ upstream *httpUpstreamService }

func (t probeTestRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.upstream.Do(req.WithContext(service.WithHTTPUpstreamPublicHostsOnly(req.Context())), "", 1, 1)
}

func TestPublicProbeRebindingStopsBeforeSocket(t *testing.T) {
	for _, answers := range [][]net.IPAddr{
		{{IP: net.ParseIP("127.0.0.1")}},
		{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("10.0.0.1")}},
	} {
		network := defaultContentDownloadNetwork()
		network.lookup = func(context.Context, string) ([]net.IPAddr, error) { return answers, nil }
		called := false
		network.dial = func(context.Context, string, string) (net.Conn, error) { called = true; return nil, io.EOF }
		client, err := newContentDownloadClient(nil, "", defaultPoolSettings(nil), false, nil, network)
		require.NoError(t, err)
		resp, err := client.Get("http://relay.example/probe")
		require.Error(t, err)
		require.Nil(t, resp)
		require.False(t, called)
	}
}
