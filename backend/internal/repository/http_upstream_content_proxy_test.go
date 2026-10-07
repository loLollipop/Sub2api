package repository

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func contentTestBridge(a, b net.Conn, aReader io.Reader) {
	defer func() { _ = a.Close() }()
	defer func() { _ = b.Close() }()
	go func() { _, _ = io.Copy(b, aReader); _ = b.Close() }()
	_, _ = io.Copy(a, b)
}

func TestContentDownloadHTTPConnectPinsDestinationAndKeepsProxy(t *testing.T) {
	for _, scheme := range []string{"http", "https"} {
		t.Run(scheme, func(t *testing.T) {
			network, state := contentTestNetwork(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/video" {
					http.Redirect(w, r, "https://cdn2.example/end", http.StatusFound)
					return
				}
				_, _ = io.WriteString(w, "video")
			})
			destinationDial := network.dial
			connects := make(chan *http.Request, 2)
			proxyServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodConnect {
					http.Error(w, "CONNECT required", http.StatusBadRequest)
					return
				}
				connects <- r.Clone(r.Context())
				destination, err := destinationDial(r.Context(), "tcp", r.Host)
				if err != nil {
					http.Error(w, "bad pinned address", http.StatusBadGateway)
					return
				}
				hijacker, ok := w.(http.Hijacker)
				if !ok {
					_ = destination.Close()
					http.Error(w, "hijacking unsupported", http.StatusInternalServerError)
					return
				}
				conn, rw, err := hijacker.Hijack()
				if err != nil {
					_ = destination.Close()
					return
				}
				_, _ = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
				_ = rw.Flush()
				contentTestBridge(conn, destination, rw)
			}))
			if scheme == "https" {
				proxyServer.TLS, network.proxyTLSConfig = contentTestTLSConfig(t)
				proxyServer.StartTLS()
			} else {
				proxyServer.Start()
			}
			t.Cleanup(proxyServer.Close)
			var mu sync.Mutex
			var proxyDials []string
			network.dial = func(ctx context.Context, netw, address string) (net.Conn, error) {
				mu.Lock()
				proxyDials = append(proxyDials, address)
				mu.Unlock()
				if address != "proxy.example:8080" {
					return nil, errors.New("proxy was bypassed")
				}
				return (&net.Dialer{}).DialContext(ctx, netw, proxyServer.Listener.Addr().String())
			}
			client, err := newContentDownloadClient(nil, scheme+"://proxyuser:proxypass@proxy.example:8080", defaultPoolSettings(nil), false, http.Header{"Accept": {"*/*"}}, network)
			require.NoError(t, err)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://cdn.example/video", nil)
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer upstream-secret")
			req.Header.Set("X-Relay-Token", "relay-secret")
			resp, err := client.Do(req)
			require.NoError(t, err)
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, "video", string(body))
			for i := 0; i < 2; i++ {
				connect := <-connects
				require.Equal(t, "8.8.8.8:443", connect.Host)
				require.Equal(t, "8.8.8.8:443", connect.RequestURI)
				require.Equal(t, "Basic cHJveHl1c2VyOnByb3h5cGFzcw==", connect.Header.Get("Proxy-Authorization"))
				require.Empty(t, connect.Header.Get("Authorization"))
				require.Empty(t, connect.Header.Get("X-Relay-Token"))
			}
			_, requests := state.snapshot()
			require.Len(t, requests, 2)
			require.Equal(t, "cdn.example", requests[0].host)
			require.Equal(t, "cdn.example", requests[0].serverName)
			require.Empty(t, requests[0].headers.Get("Proxy-Authorization"))
			require.Equal(t, "cdn2.example", requests[1].host)
			require.Equal(t, "cdn2.example", requests[1].serverName)
			require.Empty(t, requests[1].headers.Get("Authorization"))
			require.Empty(t, requests[1].headers.Get("X-Relay-Token"))
			mu.Lock()
			require.Equal(t, []string{"proxy.example:8080", "proxy.example:8080"}, proxyDials)
			mu.Unlock()
		})
	}
}

func TestContentDownloadSOCKSPinsDestinationAndKeepsProxy(t *testing.T) {
	for _, scheme := range []string{"socks5", "socks5h"} {
		t.Run(scheme, func(t *testing.T) {
			network, state := contentTestNetwork(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "video") })
			destinationDial := network.dial
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			t.Cleanup(func() { _ = listener.Close() })
			results := make(chan string, 2)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					results <- err.Error()
					return
				}
				defer func() { _ = conn.Close() }()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				head := make([]byte, 2)
				if _, err := io.ReadFull(conn, head); err != nil {
					results <- err.Error()
					return
				}
				methods := make([]byte, int(head[1]))
				if _, err := io.ReadFull(conn, methods); err != nil {
					results <- err.Error()
					return
				}
				_, _ = conn.Write([]byte{5, 2})
				if _, err := io.ReadFull(conn, head); err != nil {
					results <- err.Error()
					return
				}
				username := make([]byte, int(head[1]))
				if _, err := io.ReadFull(conn, username); err != nil {
					results <- err.Error()
					return
				}
				length := make([]byte, 1)
				if _, err := io.ReadFull(conn, length); err != nil {
					results <- err.Error()
					return
				}
				password := make([]byte, int(length[0]))
				if _, err := io.ReadFull(conn, password); err != nil {
					results <- err.Error()
					return
				}
				if string(username) != "proxyuser" || string(password) != "proxypass" {
					results <- "proxy credentials lost"
					return
				}
				_, _ = conn.Write([]byte{1, 0})
				request := make([]byte, 4)
				if _, err := io.ReadFull(conn, request); err != nil {
					results <- err.Error()
					return
				}
				var address []byte
				switch request[3] {
				case 1:
					address = make([]byte, 4)
				case 4:
					address = make([]byte, 16)
				default:
					results <- "destination hostname leaked to SOCKS"
					return
				}
				if _, err := io.ReadFull(conn, address); err != nil {
					results <- err.Error()
					return
				}
				port := make([]byte, 2)
				if _, err := io.ReadFull(conn, port); err != nil {
					results <- err.Error()
					return
				}
				destinationAddress := net.JoinHostPort(net.IP(address).String(), fmt.Sprint(binary.BigEndian.Uint16(port)))
				results <- destinationAddress
				destination, err := destinationDial(context.Background(), "tcp", destinationAddress)
				if err != nil {
					return
				}
				_, _ = conn.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
				contentTestBridge(conn, destination, conn)
			}()
			var mu sync.Mutex
			var proxyDials []string
			network.dial = func(ctx context.Context, netw, address string) (net.Conn, error) {
				mu.Lock()
				proxyDials = append(proxyDials, address)
				mu.Unlock()
				if address != "proxy.example:1080" {
					return nil, errors.New("proxy was bypassed")
				}
				return (&net.Dialer{}).DialContext(ctx, netw, listener.Addr().String())
			}
			client, err := newContentDownloadClient(nil, scheme+"://proxyuser:proxypass@proxy.example:1080", defaultPoolSettings(nil), false, nil, network)
			require.NoError(t, err)
			resp, err := client.Get("https://cdn.example/video")
			require.NoError(t, err)
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, "video", string(body))
			require.Equal(t, "8.8.8.8:443", <-results)
			_, requests := state.snapshot()
			require.Len(t, requests, 1)
			require.Equal(t, "cdn.example", requests[0].host)
			require.Equal(t, "cdn.example", requests[0].serverName)
			mu.Lock()
			require.Equal(t, []string{"proxy.example:1080"}, proxyDials)
			mu.Unlock()
		})
	}
}

func TestContentDownloadProxyFailureNeverFallsBackToDirect(t *testing.T) {
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusProxyAuthRequired) }))
	t.Cleanup(proxyServer.Close)
	var mu sync.Mutex
	var dials []string
	network := contentDownloadNetwork{lookup: func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}, dial: func(ctx context.Context, netw, address string) (net.Conn, error) {
		mu.Lock()
		dials = append(dials, address)
		mu.Unlock()
		if address != "proxy.example:8080" {
			return nil, errors.New("unexpected direct fallback")
		}
		return (&net.Dialer{}).DialContext(ctx, netw, proxyServer.Listener.Addr().String())
	}}
	client, err := newContentDownloadClient(nil, "http://proxy.example:8080", defaultPoolSettings(nil), false, nil, network)
	require.NoError(t, err)
	resp, err := client.Get("https://cdn.example/video")
	require.ErrorContains(t, err, "407")
	require.Nil(t, resp)
	mu.Lock()
	require.Equal(t, []string{"proxy.example:8080"}, dials)
	mu.Unlock()
	_, err = newContentDownloadClient(nil, "ftp://proxy.example:8080", poolSettings{}, false, nil, network)
	require.Error(t, err)
}

func TestContentDownloadHTTPConnectCancellation(t *testing.T) {
	started := make(chan struct{})
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(proxyServer.Close)
	network := defaultContentDownloadNetwork()
	network.lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}
	client, err := newContentDownloadClient(nil, proxyServer.URL, defaultPoolSettings(nil), false, nil, network)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://cdn.example/video", nil)
	require.NoError(t, err)
	result := make(chan error, 1)
	go func() {
		resp, err := client.Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("proxy CONNECT did not start")
	}
	cancel()
	select {
	case err := <-result:
		require.Error(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("CONNECT ignored cancellation")
	}
}

func TestContentDownloadHTTPUpgradeThroughPinnedProxy(t *testing.T) {
	network, state := contentTestNetwork(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "video") })
	tlsDestinationDial := network.dial
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://cdn.example/end", http.StatusFound)
	}))
	t.Cleanup(plain.Close)
	connects := make(chan string, 2)
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connects <- r.Host
		var destination net.Conn
		var err error
		if r.Host == "8.8.8.8:80" {
			destination, err = (&net.Dialer{}).DialContext(r.Context(), "tcp", plain.Listener.Addr().String())
		} else {
			destination, err = tlsDestinationDial(r.Context(), "tcp", r.Host)
		}
		if err != nil {
			http.Error(w, "destination failed", http.StatusBadGateway)
			return
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			_ = destination.Close()
			http.Error(w, "hijacking unsupported", http.StatusInternalServerError)
			return
		}
		conn, rw, err := hijacker.Hijack()
		if err != nil {
			_ = destination.Close()
			return
		}
		_, _ = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = rw.Flush()
		contentTestBridge(conn, destination, rw)
	}))
	t.Cleanup(proxyServer.Close)
	network.dial = func(ctx context.Context, netw, address string) (net.Conn, error) {
		if address != "proxy.example:8080" {
			return nil, errors.New("proxy was bypassed")
		}
		return (&net.Dialer{}).DialContext(ctx, netw, proxyServer.Listener.Addr().String())
	}
	client, err := newContentDownloadClient(nil, "http://proxy.example:8080", defaultPoolSettings(nil), false, http.Header{"Accept": {"*/*"}, "Range": {"bytes=0-3"}}, network)
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://relay.example/start", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "8.8.8.8:80", <-connects)
	require.Equal(t, "8.8.8.8:443", <-connects)
	_, requests := state.snapshot()
	require.Len(t, requests, 1)
	require.Equal(t, "cdn.example", requests[0].host)
	require.Empty(t, requests[0].headers.Get("Authorization"))
	require.Equal(t, "bytes=0-3", requests[0].headers.Get("Range"))
}

func TestContentDownloadMetadataRedirectStopsBeforeNextDial(t *testing.T) {
	network, state := contentTestNetwork(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://169.254.169.254/latest/meta-data", http.StatusFound)
	})
	resp, err := contentTestClient(t, network, false).Get("https://relay.example/start")
	require.ErrorContains(t, err, "not allowed")
	if resp != nil {
		_ = resp.Body.Close()
	}
	dials, _ := state.snapshot()
	require.Equal(t, []string{"8.8.8.8:443"}, dials)
	require.False(t, strings.Contains(strings.Join(dials, ","), "169.254"))
}
