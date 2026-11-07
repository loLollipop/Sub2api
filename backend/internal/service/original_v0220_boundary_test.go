//go:build unit

package service

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAnthropicMessagesURLStrictCloudflareAndExplicitQueries(t *testing.T) {
	path := "/client/v4/accounts/account/ai"
	for _, tc := range []struct{ base, query string }{
		{"https://api.cloudflare.com" + path, ""},
		{"https://API.CLOUDFLARE.COM:443" + path + "/", ""},
		{"https://api.cloudflare.com" + path + "?beta=false&x=1", "beta=false&x=1"},
		{"https://anthropic.example?beta=", "beta="},
		{"https://anthropic.example?x=1", "x=1&beta=true"},
		{"https://api.cloudflare.com.evil.test" + path, "beta=true"},
		{"https://api.cloudflare.com@evil.test" + path, "beta=true"},
		{"https://user@api.cloudflare.com" + path, "beta=true"},
		{"http://api.cloudflare.com" + path, "beta=true"},
		{"https://api.cloudflare.com:8443" + path, "beta=true"},
		{"https://api.cloudflare.com/client/v4/accounts/../ai", "beta=true"},
		{"https://api.cloudflare.com/client/v4/accounts/a%2Fb/ai", "beta=true"},
		{"https://api.cloudflare.com/client/v4/accounts//ai", "beta=true"},
		{"https://gateway.ai.cloudflare.com/v1/account/gateway/anthropic", "beta=true"},
	} {
		t.Run(tc.base, func(t *testing.T) {
			u, err := url.Parse(buildAnthropicMessagesURL(tc.base))
			require.NoError(t, err)
			require.Equal(t, tc.query, u.RawQuery)
			require.True(t, strings.HasSuffix(u.Path, "/v1/messages"))
		})
	}
}

func TestGeminiSSEBoundariesAndEventReset(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: 16}}
	r := newBoundedSSELineReader(strings.NewReader("data: 12345678\r\n\r\ndata: end"), cfg)
	line, err := r.ReadLineString()
	require.NoError(t, err)
	require.Equal(t, "data: 12345678\r\n", line)
	line, err = r.ReadLineString()
	require.NoError(t, err)
	require.Equal(t, "\r\n", line)
	line, err = r.ReadLineString()
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, "data: end", line)
	r = newBoundedSSELineReader(strings.NewReader("data: 123\ndata: 456\n"), cfg)
	_, err = r.ReadLineString()
	require.NoError(t, err)
	line, err = r.ReadLineString()
	require.ErrorIs(t, err, bufio.ErrTooLong)
	require.Empty(t, line)
	r = newBoundedSSELineReader(strings.NewReader(strings.Repeat(":\n", 4097)), nil)
	for i := 0; i < 4096; i++ {
		_, err = r.ReadLineString()
		require.NoError(t, err)
	}
	line, err = r.ReadLineString()
	require.ErrorIs(t, err, bufio.ErrTooLong)
	require.Empty(t, line)
}

func TestGeminiSSEOverflowRetainsObservedUsageWithoutFailover(t *testing.T) {
	for _, protocol := range []string{"native", "messages", "chat"} {
		t.Run(protocol, func(t *testing.T) {
			body := []byte(`{"model":"gemini-3-flash-preview","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)
			if protocol == "native" {
				body = []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
			}
			svc, c, rec := newGeminiAbortingStreamService(t, "/v1/messages", body, geminiAbortUsageChunk+"data: "+strings.Repeat("x", 4096), false)
			svc.cfg.Gateway.MaxLineSize = 256
			var result *ForwardResult
			var err error
			switch protocol {
			case "native":
				result, err = svc.ForwardNative(context.Background(), c, geminiVertexAPIKeyTestAccount(""), "gemini-3-flash-preview", "streamGenerateContent", true, body)
			case "messages":
				result, err = svc.Forward(context.Background(), c, geminiVertexAPIKeyTestAccount(""), body)
			case "chat":
				result, err = svc.ForwardAsChatCompletions(context.Background(), c, geminiVertexAPIKeyTestAccount(""), body)
			}
			require.ErrorIs(t, err, bufio.ErrTooLong)
			require.NotNil(t, result)
			require.Equal(t, 10, result.Usage.InputTokens)
			require.Equal(t, 2, result.Usage.OutputTokens)
			require.False(t, result.ClientDisconnect)
			require.NotContains(t, rec.Body.String(), "xxxx")
			var failover *UpstreamFailoverError
			require.False(t, errors.As(err, &failover))
		})
	}
	_, usage, err := collectGeminiSSE(&config.Config{Gateway: config.GatewayConfig{MaxLineSize: 256}}, strings.NewReader(geminiAbortUsageChunk+"data: "+strings.Repeat("x", 4096)), false)
	require.ErrorIs(t, err, bufio.ErrTooLong)
	require.Equal(t, 2, usage.OutputTokens)
}

func TestCleanGeminiRequestSchemaBoundaries(t *testing.T) {
	for _, body := range []string{
		`{"generationConfig":{"responseSchema":null,"responseJsonSchema":{"type":"object"}}}`,
		`{"generationConfig":{"response_schema":{"type":"STRING"},"response_json_schema":{"type":"object"}}}`,
		`{"generationConfig":{"responseJsonSchema":true,"response_json_schema":{"type":"object"}}}`,
	} {
		payload := cleanGeminiRequestMap(t, body)
		config := payload["generationConfig"].(map[string]any)
		require.NotContains(t, config, "responseJsonSchema")
		require.NotContains(t, config, "response_json_schema")
	}
}

func TestGeminiNativeSSEOverflowRetainsDeliveredImageWithoutTokens(t *testing.T) {
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"draw"}]}]}`)
	sse := `data: {"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"AQID"}}]}}]}` + "\n\n" + "data: " + strings.Repeat("x", 4096)
	svc, c, rec := newGeminiAbortingStreamService(t, "/v1beta/models/gemini-3-pro-image:streamGenerateContent", body, sse, false)
	svc.cfg.Gateway.MaxLineSize = 256
	result, err := svc.ForwardNative(context.Background(), c, geminiVertexAPIKeyTestAccount(""), "gemini-3-pro-image", "streamGenerateContent", true, body)
	require.ErrorIs(t, err, bufio.ErrTooLong)
	require.NotNil(t, result)
	require.Equal(t, 1, result.ImageCount)
	require.Zero(t, result.Usage.OutputTokens)
	require.Contains(t, rec.Body.String(), "AQID")
	require.NotContains(t, rec.Body.String(), "xxxx")
}

func TestGeminiNativeFirstLineOverflowWritesGoogleError(t *testing.T) {
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	svc, c, rec := newGeminiAbortingStreamService(t, "/v1beta/models/gemini-3-flash-preview:streamGenerateContent", body, "data: "+strings.Repeat("x", 4096), false)
	svc.cfg.Gateway.MaxLineSize = 256
	result, err := svc.ForwardNative(context.Background(), c, geminiVertexAPIKeyTestAccount(""), "gemini-3-flash-preview", "streamGenerateContent", true, body)
	require.ErrorIs(t, err, bufio.ErrTooLong)
	require.Nil(t, result)
	require.Equal(t, 502, rec.Code)
	require.Equal(t, "application/json; charset=utf-8", rec.Result().Header.Get("Content-Type"))
	require.Contains(t, rec.Body.String(), `"error"`)
	require.NotContains(t, rec.Body.String(), "xxxx")
}

func TestGeminiNativeInnerFrameLimitPreservesObservedResult(t *testing.T) {
	for _, tc := range []struct {
		name       string
		prefix     string
		wantTokens int
		wantImages int
	}{
		{name: "first line"},
		{name: "observed usage", prefix: "data: {\"usageMetadata\":{\"promptTokenCount\":10,\"candidatesTokenCount\":2}}\n\n", wantTokens: 2},
		{name: "delivered image", prefix: `data: {"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"AQID"}}]}}]}` + "\n\n", wantImages: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"contents":[{"role":"user","parts":[{"text":"draw"}]}]}`)
			svc, c, rec := newGeminiAbortingStreamService(t, "/v1beta/models/gemini-3-pro-image:streamGenerateContent", body, tc.prefix+"data: "+strings.Repeat("x", 4096), false)
			svc.cfg.Gateway.MaxLineSize = 256
			svc.cfg.Gateway.UpstreamResponseReadMaxBytes = 128
			result, err := svc.ForwardNative(context.Background(), c, geminiVertexAPIKeyTestAccount(""), "gemini-3-pro-image", "streamGenerateContent", true, body)
			require.ErrorIs(t, err, errSSETransportLimit)
			require.ErrorIs(t, err, bufio.ErrTooLong)
			var failover *UpstreamFailoverError
			require.False(t, errors.As(err, &failover))
			if tc.prefix == "" {
				require.Nil(t, result)
				require.Equal(t, http.StatusBadGateway, rec.Code)
				require.Equal(t, "application/json; charset=utf-8", rec.Result().Header.Get("Content-Type"))
				require.Contains(t, rec.Body.String(), `"error"`)
			} else {
				require.NotNil(t, result)
				require.Equal(t, tc.wantTokens, result.Usage.OutputTokens)
				require.Equal(t, tc.wantImages, result.ImageCount)
				require.False(t, result.ClientDisconnect)
				if tc.wantImages > 0 {
					require.Contains(t, rec.Body.String(), "AQID")
				}
			}
			require.NotContains(t, rec.Body.String(), "xxxx")
		})
	}
}

func TestAntigravityForwardUpstreamNonstreamReadLimit(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", 65)))}}
	svc := &AntigravityGatewayService{httpUpstream: upstream, settingService: &SettingService{cfg: &config.Config{Gateway: config.GatewayConfig{UpstreamResponseReadMaxBytes: 64}}}}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	account := &Account{ID: 1, Credentials: map[string]any{"base_url": "https://upstream.example", "api_key": "synthetic"}}
	result, err := svc.ForwardUpstream(context.Background(), c, account, []byte(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hi"}]}`))
	require.ErrorIs(t, err, ErrUpstreamResponseBodyTooLarge)
	require.Nil(t, result)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.NotContains(t, rec.Body.String(), "xxxx")
}

func TestGeminiToolMediaUsesMappedModelOnActualUpstreamRequest(t *testing.T) {
	for _, model := range []string{"gemini-3-flash-preview", "gemini-2.5-flash"} {
		for _, protocol := range []string{"messages", "chat"} {
			t.Run(model+"/"+protocol, func(t *testing.T) {
				body := []byte(`{"model":"client-alias","max_tokens":16,"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"a","name":"read","signature":"valid-signature","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AQID"}}]}]}]}`)
				svc, upstream := newGeminiVertexAPIKeyTestService()
				account := geminiVertexAPIKeyTestAccount("")
				account.Credentials["model_mapping"] = map[string]any{"client-alias": model}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				var err error
				if protocol == "messages" {
					_, err = svc.Forward(context.Background(), c, account, body)
				} else {
					_, err = svc.forwardClaudeBodyAsChatCompletions(context.Background(), c, account, body, "client-alias", false, false, time.Now(), body)
				}
				require.NoError(t, err)
				require.Contains(t, upstream.lastReq.URL.Path, model)
				wire, readErr := io.ReadAll(upstream.lastReq.Body)
				require.NoError(t, readErr)
				require.Equal(t, "valid-signature", gjson.GetBytes(wire, "contents.0.parts.0.thoughtSignature").String())
				if strings.HasPrefix(model, "gemini-3") {
					require.Equal(t, "AQID", gjson.GetBytes(wire, "contents.1.parts.0.functionResponse.parts.0.inlineData.data").String())
					require.False(t, gjson.GetBytes(wire, "contents.1.parts.1.inlineData").Exists())
				} else {
					require.Equal(t, "AQID", gjson.GetBytes(wire, "contents.1.parts.1.inlineData.data").String())
					require.False(t, gjson.GetBytes(wire, "contents.1.parts.0.functionResponse.parts").Exists())
				}
			})
		}
	}
}

func TestGeminiSSEOverflowRetainsCacheOnlyUsage(t *testing.T) {
	sse := "data: {\"usageMetadata\":{\"promptTokenCount\":900,\"cachedContentTokenCount\":900}}\n\ndata: " + strings.Repeat("x", 4096)
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	svc, c, _ := newGeminiAbortingStreamService(t, "/v1beta/models/gemini-3-flash-preview:streamGenerateContent", body, sse, false)
	svc.cfg.Gateway.MaxLineSize = 256
	result, err := svc.ForwardNative(context.Background(), c, geminiVertexAPIKeyTestAccount(""), "gemini-3-flash-preview", "streamGenerateContent", true, body)
	require.ErrorIs(t, err, bufio.ErrTooLong)
	require.NotNil(t, result)
	require.Equal(t, 900, result.Usage.CacheReadInputTokens)
	require.Zero(t, result.Usage.InputTokens)
	require.Zero(t, result.Usage.OutputTokens)
}
