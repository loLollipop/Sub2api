package admin

import (
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpsWSOriginUsesValidatedRequestHost(t *testing.T) {
	previous := opsWSProxyConfig
	opsWSProxyConfig = OpsWSProxyConfig{TrustProxy: true, TrustedProxies: defaultTrustedProxies(), OriginPolicy: OriginPolicyStrict}
	t.Cleanup(func() { opsWSProxyConfig = previous })
	for _, tc := range []struct {
		name, host, origin, forwarded string
		want                          bool
	}{
		{"same host", "example.com", "https://example.com", "", true},
		{"case and port policy", "EXAMPLE.com:8080", "https://example.com", "", true},
		{"IPv6", "[::1]:8080", "http://[::1]:8080", "", true},
		{"spoofed forwarded host", "example.com", "https://evil.example", "evil.example", false},
		{"forwarded host cannot override valid host", "example.com", "https://example.com", "evil.example", true},
		{"userinfo", "example.com", "https://evil@example.com", "", false},
		{"scheme", "example.com", "ftp://example.com", "", false},
		{"path", "example.com", "https://example.com/", "", false},
		{"query", "example.com", "https://example.com?", "", false},
		{"fragment", "example.com", "https://example.com#", "", false},
		{"invalid origin host", "example.com", "https://example.com:0", "", false},
		{"invalid request host", "example.com,evil.example", "https://example.com", "", false},
		{"invalid IPv6", "[not-an-ip]", "http://[not-an-ip]", "", false},
		{"strict missing", "example.com", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://example.com/ops/ws", nil)
			r.Host, r.RemoteAddr = tc.host, "127.0.0.1:12345"
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("X-Forwarded-Host", tc.forwarded)
			require.Equal(t, tc.want, isAllowedOpsWSOrigin(r))
		})
	}
	opsWSProxyConfig.OriginPolicy = OriginPolicyPermissive
	require.True(t, isAllowedOpsWSOrigin(httptest.NewRequest("GET", "http://example.com", nil)))
}

func TestOpsWSTrustRejectsWildcardAndNormalizesMappedIPv4(t *testing.T) {
	prefixes, invalid := parseTrustedProxyList("0.0.0.0/0,::/0,::ffff:0.0.0.0/96,::ffff:10.0.0.0/104,127.0.0.1")
	require.Len(t, invalid, 3)
	require.Equal(t, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("127.0.0.1/32")}, prefixes)
	require.True(t, isAddrInTrustedProxies(netip.MustParseAddr("10.1.2.3"), prefixes))
	require.False(t, isAddrInTrustedProxies(netip.MustParseAddr("8.8.8.8"), []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")}))
}
