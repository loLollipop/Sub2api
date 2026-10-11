package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAnthropicCacheTTLBetaForwardModes(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, tc := range []struct {
			name, mode, originalTTL, wantTTL string
			wantBeta                         bool
		}{
			{"force1h", AnthropicCacheTTLMode1h, "5m", "1h", true},
			{"force5m", AnthropicCacheTTLMode5m, "1h", "5m", false},
			{"off_preserves1h", AnthropicCacheTTLModeOff, "1h", "1h", true},
			{"off_preserves5m", AnthropicCacheTTLModeOff, "5m", "5m", false},
			{"inherit_client1h", AnthropicCacheTTLModeInherit, "1h", "1h", true},
		} {
			t.Run(fmt.Sprintf("passthrough_%t/%s", passthrough, tc.name), func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				c.Request.Header.Set("Anthropic-Beta", "arbitrary-client-beta")
				c.Set("api_key", &APIKey{AnthropicCacheTTLMode: tc.mode})
				account := newAnthropicAPIKeyAccountForTest()
				account.Extra["anthropic_passthrough"] = passthrough
				upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
					StatusCode: http.StatusOK, Header: make(http.Header),
					Body: io.NopCloser(strings.NewReader(`{"type":"message","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`)),
				}}
				svc := &GatewayService{cfg: &config.Config{}, httpUpstream: upstream, rateLimitService: &RateLimitService{}, deferredService: &DeferredService{}}
				body := []byte(fmt.Sprintf(`{ "model":"claude-sonnet-5-5", "max_tokens":32,"stream":false,"system":[{"type":"text","text":"arbitrary system","cache_control":{"type":"ephemeral","ttl":"%s"}}],"messages":[{"role":"user","content":"literal cache_control ttl 1h"}],"metadata":{"user_id":"unchanged"},"custom_vendor":{"array":[1,true,"cache_control"],"large_integer":9007199254740993} }`, tc.originalTTL))
				original := append([]byte(nil), body...)
				_, err := svc.Forward(context.Background(), c, account, &ParsedRequest{Body: NewRequestBodyRef(body), Model: "claude-sonnet-5-5"})
				require.NoError(t, err)
				require.Equal(t, original, body, "rewriting must not mutate the caller's body")
				require.Equal(t, tc.wantTTL, gjson.GetBytes(upstream.lastBody, "system.0.cache_control.ttl").String())
				beta := getHeaderRaw(upstream.lastReq.Header, "anthropic-beta")
				require.Equal(t, tc.wantBeta, containsBetaToken(beta, claude.BetaExtendedCacheTTL))
				require.True(t, containsBetaToken(beta, "arbitrary-client-beta"))
				for _, path := range []string{"custom_vendor", "metadata", "messages", "system.0.text", "max_tokens"} {
					require.Equal(t, gjson.GetBytes(original, path).Raw, gjson.GetBytes(upstream.lastBody, path).Raw, "unrelated body field %s", path)
				}
				if tc.wantTTL == tc.originalTTL {
					require.Equal(t, original, upstream.lastBody, "no TTL rewrite must preserve the raw body")
				}
			})
		}
	}
}

func TestAnthropicCacheTTLBetaRequestDropAndOverridePrecedence(t *testing.T) {
	for _, tc := range []struct {
		name, override                           string
		passthrough, drop, hasOverride, wantBeta bool
	}{
		{name: "normal_policy_drop", drop: true},
		{name: "normal_override_removes_beta", hasOverride: true, override: "account-only-beta"},
		{name: "passthrough_override_removes_beta", passthrough: true, hasOverride: true, override: "account-only-beta"},
		{name: "normal_empty_override_ignored", hasOverride: true, wantBeta: true},
		{name: "passthrough_empty_override_ignored", passthrough: true, hasOverride: true, wantBeta: true},
		{name: "normal_override_wins_drop", drop: true, hasOverride: true, override: claude.BetaExtendedCacheTTL, wantBeta: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			c.Request.Header.Set("Anthropic-Beta", "client-only-beta")
			account := newAnthropicAPIKeyAccountForTest()
			if tc.hasOverride {
				account.Credentials["header_override_enabled"] = true
				account.Credentials["header_overrides"] = map[string]any{"anthropic-beta": tc.override}
			}
			svc := &GatewayService{cfg: &config.Config{}}
			if tc.drop {
				// Forward caches the evaluated filter set for this request builder.
				c.Set(betaPolicyFilterSetKey, map[string]struct{}{claude.BetaExtendedCacheTTL: {}})
			}
			body := []byte(`{ "model":"claude-sonnet-5-5","max_tokens":32,"system":[{"type":"text","text":"x","cache_control":{"type":"ephemeral","ttl":"1h"}}],"messages":[{"role":"user","content":"hi"}],"custom_vendor": [1,2,3] }`)
			var req *http.Request
			var wire []byte
			var err error
			if tc.passthrough {
				req, wire, err = svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(context.Background(), c, account, body, "token")
			} else {
				req, wire, err = svc.buildUpstreamRequest(context.Background(), c, account, body, "token", "apikey", "claude-sonnet-5-5", false, false)
			}
			require.NoError(t, err)
			require.Equal(t, body, wire)
			beta := getHeaderRaw(req.Header, "anthropic-beta")
			require.Equal(t, tc.wantBeta, containsBetaToken(beta, claude.BetaExtendedCacheTTL))
			if tc.hasOverride && tc.override != "" {
				require.Equal(t, tc.override, beta, "enabled nonempty account override is authoritative")
			} else if !tc.wantBeta {
				require.Equal(t, "client-only-beta", beta)
			} else {
				require.True(t, containsBetaToken(beta, "client-only-beta"), "empty override is only a template placeholder")
			}
		})
	}
}

func TestAnthropicCacheTTLBetaRequestRawGuards(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, tc := range []struct {
			name, body string
			wantBeta   bool
		}{
			{"literal_in_string", `{ "model":"claude-sonnet-5-5","messages":[{"role":"user","content":"literal cache_control ttl 1h"}],"custom":"{\"cache_control\":{\"type\":\"ephemeral\",\"ttl\":\"1h\"}}" }`, false},
			{"non_ephemeral", `{ "model":"claude-sonnet-5-5","system":[{"type":"text","text":"hi","cache_control":{"type":"persistent","ttl":"1h"}}],"messages":[] }`, false},
			{"escaped_ttl", `{ "model":"claude-sonnet-5-5","system":[{"type":"text","text":"hi","cache_control":{"type":"ephemeral","ttl":"\u0031h"}}],"messages":[] }`, true},
			{"escaped_field_name", `{ "model":"claude-sonnet-5-5","system":[{"type":"text","text":"hi","cache_contr\u006fl":{"type":"ephemeral","ttl":"1h"}}],"messages":[] }`, true},
		} {
			t.Run(fmt.Sprintf("passthrough_%t/%s", passthrough, tc.name), func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				svc := &GatewayService{cfg: &config.Config{}}
				account := newAnthropicAPIKeyAccountForTest()
				var req *http.Request
				var wire []byte
				var err error
				if passthrough {
					req, wire, err = svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(context.Background(), c, account, []byte(tc.body), "token")
				} else {
					req, wire, err = svc.buildUpstreamRequest(context.Background(), c, account, []byte(tc.body), "token", "apikey", "claude-sonnet-5-5", false, false)
				}
				require.NoError(t, err)
				require.Equal(t, tc.body, string(wire))
				require.Equal(t, tc.wantBeta, containsBetaToken(getHeaderRaw(req.Header, "anthropic-beta"), claude.BetaExtendedCacheTTL))
			})
		}
	}
}
