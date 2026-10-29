package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGatewayAutomaticCacheWireAndTTLOverride(t *testing.T) {
	for _, tc := range []struct{ name, mode, extra, wantPath, wantTTL string }{
		{"automatic", "inherit", "", "messages.0.content.0", "1h"},
		{"ttl override", "5m", "", "messages.0.content.0", "5m"},
		{"explicit wins", "inherit", `,"system":[{"type":"text","text":"explicit","cache_control":{"type":"ephemeral","ttl":"5m"}}]`, "system.0", "1h"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			c.Set("api_key", &APIKey{AnthropicCacheTTLMode: tc.mode})
			account := newAnthropicAPIKeyAccountForTest()
			account.Extra["anthropic_passthrough"] = false
			upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"type":"message","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`))}}
			svc := &GatewayService{cfg: &config.Config{}, httpUpstream: upstream, rateLimitService: &RateLimitService{}, deferredService: &DeferredService{}}
			body := []byte(`{"model":"claude-sonnet-5-5","max_tokens":32,"stream":false,"cache_control":{"type":"ephemeral","ttl":"1h"},"messages":[{"role":"user","content":"hello"}]` + tc.extra + `}`)
			_, err := svc.Forward(context.Background(), c, account, &ParsedRequest{Body: NewRequestBodyRef(body), Model: "claude-sonnet-5-5"})
			require.NoError(t, err)
			require.Equal(t, tc.wantTTL, gjson.GetBytes(upstream.lastBody, tc.wantPath+".cache_control.ttl").String())
			require.True(t, gjson.GetBytes(upstream.lastBody, "cache_control").Exists())
			// Shared TTL normalization may lift the explicit marker to match the top-level intent.
			if tc.name == "explicit wins" {
				require.Equal(t, "hello", gjson.GetBytes(upstream.lastBody, "messages.0.content").String())
			}
			assertNoTTLOrderViolation(t, upstream.lastBody)
		})
	}
}
