//go:build unit

package service

import (
	"bytes"
	"context"
	"fmt"
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

func TestV226ThinkingBudgetRetryRequiresOptInAndAdmin(t *testing.T) {
	for _, provider := range []string{"anthropic", "antigravity"} {
		for _, tc := range []struct {
			name     string
			optIn    bool
			admin    bool
			adaptive bool
			want     int
		}{{"default off", false, true, false, 1}, {"admin off", true, false, false, 1}, {"explicit on", true, true, false, 2}, {"adaptive", true, true, true, 1}} {
			t.Run(provider+"/"+tc.name, func(t *testing.T) {
				cfg := &config.Config{Gateway: config.GatewayConfig{ThinkingBudgetRectifyEnabled: tc.optIn, MaxLineSize: defaultMaxLineSize}}
				settings := NewSettingService(&contentModerationTestSettingRepo{values: map[string]string{
					SettingKeyRectifierSettings: fmt.Sprintf(`{"enabled":%t,"thinking_signature_enabled":false,"thinking_budget_enabled":%t}`, tc.admin, tc.admin),
				}}, cfg)
				thinking := `{"type":"enabled","budget_tokens":16}`
				if tc.adaptive {
					thinking = `{"type":"adaptive"}`
				}
				body := []byte(`{"model":"claude-sonnet-4-6","max_tokens":128,"thinking":` + thinking + `,"messages":[{"role":"user","content":"hi"}],"stream":false}`)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
				good := `{"id":"msg_budget","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`
				goodType := "application/json"
				if provider == "antigravity" {
					good = "data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":1}}}\n\n"
					goodType = "text/event-stream"
				}
				upstream := &httpUpstreamRecorder{responses: []*http.Response{
					{StatusCode: 400, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"invalid_request_error","message":"thinking budget_tokens must be >= 1024"}}`))},
					{StatusCode: 200, Header: http.Header{"Content-Type": {goodType}}, Body: io.NopCloser(strings.NewReader(good))},
				}}
				account := &Account{ID: 26, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Status: StatusActive, Concurrency: 1,
					Credentials: map[string]any{"api_key": "test", "base_url": "https://api.anthropic.com"}}
				var err error
				if provider == "anthropic" {
					svc := &GatewayService{cfg: cfg, settingService: settings, httpUpstream: upstream, rateLimitService: &RateLimitService{}, deferredService: &DeferredService{}}
					_, err = svc.Forward(context.Background(), c, account, &ParsedRequest{Body: NewRequestBodyRef(body), Model: "claude-sonnet-4-6"})
				} else {
					account.Platform = PlatformAntigravity
					account.Type = AccountTypeOAuth
					account.Credentials = map[string]any{"access_token": "test", "project_id": "test-project", "model_mapping": map[string]any{"claude-sonnet-4-6": "claude-sonnet-4-6"}}
					svc := &AntigravityGatewayService{settingService: settings, tokenProvider: &AntigravityTokenProvider{}, httpUpstream: upstream}
					_, err = svc.Forward(context.Background(), c, account, body, false)
				}
				require.Len(t, upstream.requests, tc.want)
				if tc.want == 2 {
					require.NoError(t, err)
					if provider == "anthropic" {
						require.Equal(t, int64(BudgetRectifyBudgetTokens), gjson.GetBytes(upstream.lastBody, "thinking.budget_tokens").Int())
						require.Equal(t, int64(BudgetRectifyMaxTokens), gjson.GetBytes(upstream.lastBody, "max_tokens").Int())
					}
				} else {
					require.Error(t, err)
				}
			})
		}
	}
}
