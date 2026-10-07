package repository

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"golang.org/x/net/proxy"
)

// This adapter is used only for video content downloads. It does not inherit
// the gateway's pooled transport, remote destination DNS, cookies, or automatic
// provider retries. A fresh transport per hop binds the validated addresses to
// the actual connection, while net/http retains the original Host and TLS SNI.
type contentDownloadNetwork struct {
	lookup         func(context.Context, string) ([]net.IPAddr, error)
	dial           func(context.Context, string, string) (net.Conn, error)
	tlsConfig      *tls.Config
	proxyTLSConfig *tls.Config
}

func defaultContentDownloadNetwork() contentDownloadNetwork {
	return contentDownloadNetwork{lookup: net.DefaultResolver.LookupIPAddr, dial: newUpstreamDialer().DialContext}
}

type contentDownloadTransport struct {
	network             contentDownloadNetwork
	proxyURL            *url.URL
	settings            poolSettings
	allowPrivateInitial bool
}

func newContentDownloadClient(base *http.Client, proxyRaw string, settings poolSettings, allowPrivateInitial bool, headers http.Header, network contentDownloadNetwork) (*http.Client, error) {
	_, proxyURL, err := proxyurl.Parse(proxyRaw)
	if err != nil {
		return nil, err
	}
	if proxyURL != nil {
		proxyURL.Scheme = strings.ToLower(proxyURL.Scheme)
	}
	safeHeaders := make(http.Header)
	for _, name := range []string{"Accept", "Range"} {
		if values := headers.Values(name); len(values) > 0 {
			safeHeaders[name] = append([]string(nil), values...)
		}
	}
	client := &http.Client{Transport: &contentDownloadTransport{
		network: network, proxyURL: proxyURL, settings: settings, allowPrivateInitial: allowPrivateInitial,
	}}
	if base != nil {
		client.Timeout = base.Timeout
	}
	leftInitialOrigin := false
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 10 {
			return errors.New("stopped after 10 content redirects")
		}
		if err := validateContentDownloadURL(req.URL); err != nil {
			return err
		}
		previous := via[len(via)-1]
		if strings.EqualFold(previous.URL.Scheme, "https") && strings.EqualFold(req.URL.Scheme, "http") {
			return errors.New("content redirect HTTPS downgrade is not allowed")
		}
		if contentDownloadOrigin(previous.URL) != contentDownloadOrigin(req.URL) {
			leftInitialOrigin = true
		}
		if leftInitialOrigin {
			// Go copies the INITIAL headers again for every hop. Replace the
			// entire map every time, even if the chain returns to the relay.
			clear(req.Header)
			for _, name := range []string{"Accept", "Range"} {
				if values := safeHeaders.Values(name); len(values) > 0 {
					req.Header[name] = append([]string(nil), values...)
				}
			}
			req.Host = ""
		}
		return nil
	}
	return client, nil
}

func validateContentDownloadURL(u *url.URL) error {
	if u == nil || u.Hostname() == "" || u.User != nil || u.Opaque != "" || strings.ContainsAny(u.Host, "\\\r\n\x00%") {
		return errors.New("invalid content destination")
	}
	if !strings.EqualFold(u.Scheme, "https") && !strings.EqualFold(u.Scheme, "http") {
		return errors.New("invalid content destination scheme")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n <= 0 || n > 65535 {
			return errors.New("invalid content destination port")
		}
	}
	return nil
}

func contentDownloadPort(u *url.URL) string {
	if port := u.Port(); port != "" {
		n, _ := strconv.Atoi(port)
		return strconv.Itoa(n)
	}
	if strings.EqualFold(u.Scheme, "https") {
		return "443"
	}
	return "80"
}

func contentDownloadOrigin(u *url.URL) string {
	return strings.ToLower(u.Scheme) + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), contentDownloadPort(u))
}

func (t *contentDownloadTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := validateContentDownloadURL(req.URL); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(req.Context(), defaultUpstreamDialTimeout)
	// Resolve and validate ALL answers before any direct or proxy connection.
	// Only the operator-configured initial relay may use private destinations.
	ips, err := t.network.resolve(ctx, req.URL.Hostname(), !t.allowPrivateInitial || req.Response != nil)
	cancel()
	if err != nil {
		return nil, err
	}
	port := contentDownloadPort(req.URL)
	transport := &http.Transport{
		Proxy: nil, DisableKeepAlives: true,
		TLSHandshakeTimeout:   defaultUpstreamTLSHandshakeTimeout,
		ResponseHeaderTimeout: t.settings.responseHeaderTimeout,
		TLSClientConfig:       t.network.tlsConfig,
	}
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		ctx, cancel := context.WithTimeout(ctx, defaultUpstreamDialTimeout)
		defer cancel()
		var lastErr error
		for _, ip := range ips {
			// Neither direct dialers nor proxies receive the destination hostname.
			conn, err := t.network.dialDestination(ctx, network, net.JoinHostPort(ip.String(), port), t.proxyURL)
			if err == nil {
				return conn, nil
			}
			lastErr = err
			if ctx.Err() != nil {
				break
			}
		}
		return nil, lastErr
	}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, err
	}
	resp.Body = wrapTrackedBody(resp.Body, transport.CloseIdleConnections)
	return resp, nil
}

func (n contentDownloadNetwork) resolve(ctx context.Context, host string, publicOnly bool) ([]netip.Addr, error) {
	var answers []net.IPAddr
	if ip, err := netip.ParseAddr(host); err == nil {
		answers = []net.IPAddr{{IP: net.IP(ip.AsSlice()), Zone: ip.Zone()}}
	} else {
		var err error
		answers, err = n.lookup(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("content destination DNS failed: %w", err)
		}
	}
	if len(answers) == 0 {
		return nil, errors.New("content destination DNS returned no addresses")
	}
	ips := make([]netip.Addr, 0, len(answers))
	for _, answer := range answers {
		ip, ok := netip.AddrFromSlice(answer.IP)
		if !ok || answer.Zone != "" || (publicOnly && !urlvalidator.IsPublicResolvedIP(ip)) {
			return nil, errors.New("content destination IP is not allowed")
		}
		ips = append(ips, ip.Unmap())
	}
	return ips, nil
}

func (n contentDownloadNetwork) dialDestination(ctx context.Context, network, destination string, proxyURL *url.URL) (net.Conn, error) {
	if proxyURL == nil {
		return n.dial(ctx, network, destination)
	}
	proxyAddress := proxyURL.Host
	if proxyURL.Port() == "" {
		port := "80"
		switch proxyURL.Scheme {
		case "https":
			port = "443"
		case "socks5h", "socks5":
			port = "1080"
		}
		proxyAddress = net.JoinHostPort(proxyURL.Hostname(), port)
	}
	switch strings.ToLower(proxyURL.Scheme) {
	case "http", "https":
		return n.dialHTTPProxy(ctx, network, proxyAddress, destination, proxyURL)
	case "socks5", "socks5h":
		var auth *proxy.Auth
		if proxyURL.User != nil {
			password, _ := proxyURL.User.Password()
			auth = &proxy.Auth{User: proxyURL.User.Username(), Password: password}
		}
		dialer, err := proxy.SOCKS5("tcp", proxyAddress, auth, contentDownloadForwardDialer{dial: n.dial})
		if err != nil {
			return nil, err
		}
		contextDialer, ok := dialer.(proxy.ContextDialer)
		if !ok {
			return nil, errors.New("content SOCKS proxy does not support cancellation")
		}
		return contextDialer.DialContext(ctx, network, destination)
	default:
		return nil, errors.New("unsupported content proxy scheme")
	}
}

type contentDownloadForwardDialer struct {
	dial func(context.Context, string, string) (net.Conn, error)
}

func (d contentDownloadForwardDialer) Dial(network, address string) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultUpstreamDialTimeout)
	defer cancel()
	return d.dial(ctx, network, address)
}

func (d contentDownloadForwardDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return d.dial(ctx, network, address)
}

func (n contentDownloadNetwork) dialHTTPProxy(ctx context.Context, network, proxyAddress, destination string, proxyURL *url.URL) (net.Conn, error) {
	conn, err := n.dial(ctx, network, proxyAddress)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			_ = conn.Close()
		}
	}()
	rawConn := conn
	stopCancel := context.AfterFunc(ctx, func() { _ = rawConn.Close() })
	defer stopCancel()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if strings.EqualFold(proxyURL.Scheme, "https") {
		cfg := &tls.Config{ServerName: proxyURL.Hostname()}
		if n.proxyTLSConfig != nil {
			cfg = n.proxyTLSConfig.Clone()
			cfg.ServerName = proxyURL.Hostname()
		}
		tlsConn := tls.Client(conn, cfg)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return nil, err
		}
		conn = tlsConn
	}
	connect := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: destination}, Host: destination, Header: make(http.Header)}
	if proxyURL.User != nil {
		password, _ := proxyURL.User.Password()
		credentials := proxyURL.User.Username() + ":" + password
		connect.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(credentials)))
	}
	if err := connect.Write(conn); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, connect)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("content proxy CONNECT failed (status %d)", resp.StatusCode)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	success = true
	return &contentDownloadBufferedConn{Conn: conn, reader: reader}, nil
}

type contentDownloadBufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *contentDownloadBufferedConn) Read(p []byte) (int, error) { return c.reader.Read(p) }
