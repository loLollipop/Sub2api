package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestClientErrorPrivacyPreservesSuccessfulDocuments(t *testing.T) {
	for _, body := range []string{
		`{"error":null,"output":[{"text":"https://public.example/article"}],"usage":{"input_tokens":9}}`,
		`{"error":false,"content":"https://public.example/article"}`,
		`{"error":"","content":"https://public.example/article"}`,
		`{"error":0,"content":"https://public.example/article"}`,
		`{"response":{"error":null,"output":[{"text":"192.0.2.10"}]}}`,
		`{"results":[{"url":"https://public.example/article","content":"contact author@public.example"}]}`,
		`{"content":"api_key=synthetic-example in an article","url":"https://public.example/article"}`,
	} {
		require.Equal(t, body, string(sanitizeClientUpstreamErrorBody(http.StatusOK, []byte(body))))
	}
}

func TestClientErrorPrivacyResponseContentTypeAtRealWrites(t *testing.T) {
	for _, filter := range []*responseheaders.CompiledHeaderFilter{nil, responseheaders.CompileHeaderFilter(config.ResponseHeaderConfig{})} {
		for _, contentType := range []string{
			`application/json; upstream="https://private-provider.example"; api_key="synthetic-private-value"`,
			"not a media type", "application/json; charset=utf-8",
		} {
			for _, protocol := range []string{"anthropic-json", "anthropic-stream", "deepseek-tools"} {
				t.Run(protocol+"/"+contentType+fmt.Sprint(filter == nil), func(t *testing.T) {
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
					body := `{"type":"message","content":[{"type":"text","text":"https://public.example"}],"usage":{"input_tokens":5}}`
					if protocol == "anthropic-stream" {
						body = "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
					} else if protocol == "deepseek-tools" {
						body = `{"results":[{"url":"https://public.example"}]}`
					}
					resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{
						"Content-Type": {contentType}, "X-Request-Id": {"req-safe-123"},
					}, Body: io.NopCloser(strings.NewReader(body))}
					gateway := &GatewayService{responseHeaderFilter: filter}
					var err error
					switch protocol {
					case "anthropic-json":
						_, err = gateway.handleNonStreamingResponseAnthropicAPIKeyPassthrough(t.Context(), resp, c, &Account{ID: 1})
					case "anthropic-stream":
						_, err = gateway.handleStreamingResponseAnthropicAPIKeyPassthrough(t.Context(), resp, c, &Account{ID: 1}, time.Now(), "fixture")
					case "deepseek-tools":
						tools := &OpenAIGatewayService{cfg: &config.Config{}, responseHeaderFilter: filter, httpUpstream: &httpUpstreamRecorder{resp: resp}}
						_, err = tools.ForwardDeepSeekTools(t.Context(), c, deepSeekToolsTestAccount(), DeepSeekToolsPathSearch, nil)
					}
					require.NoError(t, err)
					want := "application/json"
					if protocol == "anthropic-stream" {
						want = "text/event-stream"
					}
					if contentType == "application/json; charset=utf-8" {
						want = contentType
					}
					require.Equal(t, want, rec.Header().Get("Content-Type"))
					if protocol != "deepseek-tools" || filter != nil {
						require.Equal(t, "req-safe-123", rec.Header().Get("X-Request-Id"))
					}
					require.Equal(t, body, rec.Body.String(), "successful wire content remains unchanged")
				})
			}
		}
	}
}

func TestClientErrorPrivacyGeminiErrorWritersKeepInternalEvidence(t *testing.T) {
	const message = "provider https://private-provider.example rejected api_key=synthetic-opaque-value"
	for _, google := range []bool{false, true} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		svc := &GeminiMessagesCompatService{}
		var err error
		if google {
			err = svc.writeGoogleError(c, http.StatusBadRequest, message)
		} else {
			err = svc.writeClaudeError(c, http.StatusBadRequest, "invalid_request_error", message)
		}
		require.EqualError(t, err, message, "internal callers retain the original diagnostic")
		require.NotContains(t, rec.Body.String(), "private-provider")
		require.NotContains(t, rec.Body.String(), "synthetic-opaque-value")
	}
}

func TestClientErrorPrivacyRecognizesEmptyErrorsAndFailedStatuses(t *testing.T) {
	for _, body := range []string{
		`{"error":{},"message":"https://private-provider.example failed"}`,
		`{"response":{"error":{},"message":"https://private-provider.example failed"}}`,
		`{"status":"failed","message":"https://private-provider.example failed"}`,
		`{"response":{"status":"failed","message":"https://private-provider.example failed"}}`,
		`{"error":null,"error":{"message":"https://private-provider.example failed","api_key":"synthetic-private-value"}}`,
		`{"status":"completed","status":"failed","message":"https://private-provider.example failed"}`,
		`{"response":{},"response":{"status":"failed","message":"https://private-provider.example failed"}}`,
		`{"response":{"status":"completed","status":"failed","message":"https://private-provider.example failed"}}`,
		`{"type":"response.completed","type":"response.failed","message":"https://private-provider.example failed"}`,
	} {
		original := []byte(body)
		cleaned := sanitizeClientUpstreamErrorBody(http.StatusOK, original)
		require.True(t, gjson.ValidBytes(cleaned))
		require.NotContains(t, string(cleaned), "private-provider")
		require.Equal(t, body, string(original), "internal classification retains the original body")
	}
}

func TestClientErrorPrivacyManualResponseHeaderCopies(t *testing.T) {
	for _, filter := range []*responseheaders.CompiledHeaderFilter{nil, responseheaders.CompileHeaderFilter(config.ResponseHeaderConfig{})} {
		for _, requestID := range []string{"https://private-provider.example", "Bearer synthetic-private-value", "192.0.2.42", "req-safe-123"} {
			source := http.Header{"X-Request-Id": {requestID}, "Content-Type": {"application/json"}}
			for _, write := range []func(http.Header, http.Header, *responseheaders.CompiledHeaderFilter){writeAnthropicPassthroughResponseHeaders, writeOpenAIPassthroughResponseHeaders} {
				destination := make(http.Header)
				write(destination, source, filter)
				if requestID == "req-safe-123" && filter != nil {
					require.Equal(t, requestID, destination.Get("X-Request-Id"))
				} else if requestID != "req-safe-123" {
					require.Empty(t, destination.Get("X-Request-Id"))
				}
			}
		}
	}
}

func TestClientErrorPrivacyAnthropicTerminalRetainsOriginalEvidenceAndUsage(t *testing.T) {
	const payload = `{"type":"error","error":{"type":"relay_stream_error","status_code":403,"message":"https://private-provider.example failed","api_key":"synthetic-opaque-value"}}`
	const prefix = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":11,\"cache_read_input_tokens\":7}}}\n\n"
	for _, frame := range []string{
		"event: error\ndata: " + payload + "\n\n",
		"data: " + payload + "\n\n",
		payload + "\n\n",
		"event: error\ndata: {\ndata: \"type\":\"error\",\n" + `data: "error":{"type":"relay_stream_error","status_code":403,"message":"https://private-provider.example failed","api_key":"synthetic-opaque-value"}}` + "\n\n",
	} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
		upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
			Body: &streamReadCloser{payload: []byte(prefix + frame), err: io.ErrUnexpectedEOF},
		}}
		svc := newForwardPartialUsageServiceForTest(upstream)
		body := []byte(`{"model":"claude-3-7-sonnet-20250219","stream":true,"messages":[{"role":"user","content":"hello"}]}`)
		result, err := svc.Forward(context.Background(), c, newAnthropicAPIKeyAccountForTest(), &ParsedRequest{
			Body: NewRequestBodyRef(body), Model: "claude-3-7-sonnet-20250219", Stream: true,
		})
		var failure *AnthropicPassthroughStreamError
		require.ErrorAs(t, err, &failure)
		require.Equal(t, 403, failure.StatusCode)
		require.True(t, failure.ResponseWritten)
		require.Contains(t, failure.RawData, "private-provider.example", "restricted original evidence must be preserved")
		var failover *UpstreamFailoverError
		require.False(t, errors.As(err, &failover))
		require.NotNil(t, result)
		require.Equal(t, 11, result.Usage.InputTokens)
		require.Equal(t, 7, result.Usage.CacheReadInputTokens)
		require.True(t, strings.HasPrefix(rec.Body.String(), prefix), "normal frames must keep their exact bytes")
		require.NotContains(t, rec.Body.String(), "private-provider")
		require.NotContains(t, rec.Body.String(), "synthetic-opaque-value")
		require.Contains(t, rec.Body.String(), `"status_code":403`)
	}
}

func TestClientErrorPrivacyOpenAITerminalAndSyntheticEOF(t *testing.T) {
	for _, eventType := range []string{"error", "response.failed"} {
		body := []byte(`{"type":"` + eventType + `","error":{"code":"upstream_error","message":"dial tcp 192.0.2.42:443 failed; account_name=private-account"}}`)
		original := bytes.Clone(body)
		cleaned, changed := sanitizeOpenAIResponseFailedEventForClient(body, eventType, true)
		require.True(t, changed)
		require.Equal(t, original, body)
		require.Equal(t, eventType, gjson.GetBytes(cleaned, "type").String())
		require.NotContains(t, string(cleaned), "192.0.2.42")
		require.NotContains(t, string(cleaned), "private-account")
	}
	wire := buildOpenAIResponseFailedSSE("resp_safe", "safe-model", 0, nil, "Get https://private-provider.example: unexpected EOF")
	require.Contains(t, wire, "event: response.failed\n")
	require.Contains(t, wire, "unexpected EOF")
	require.NotContains(t, wire, "private-provider")
	tool := []byte(`{"type":"response.function_call_arguments.delta","delta":"https://public.example"}`)
	cleaned, changed := sanitizeOpenAIResponseFailedEventForClient(tool, "response.function_call_arguments.delta", true)
	require.False(t, changed)
	require.Equal(t, tool, cleaned)
}

func TestClientErrorPrivacyKeepsOpaqueCodexTurnState(t *testing.T) {
	// The protocol defines this as opaque. Credential-shaped syntax alone
	// cannot justify corrupting a legitimate state value needed next turn.
	state := strings.Join([]string{"eyJhbGciOiJub25lIn0", "eyJzdGF0ZSI6InN5bnRoZXRpYyJ9", "synthetic-signature"}, ".")
	header := http.Header{"X-Codex-Turn-State": {state}}
	require.Equal(t, state, extractOpenAICodexTurnState(header))
	dst := make(http.Header)
	writeOpenAIPassthroughResponseHeaders(dst, header, nil)
	require.Equal(t, state, dst.Get("X-Codex-Turn-State"))
}

func TestClientErrorPrivacyMalformedTerminalRemainsRecognizable(t *testing.T) {
	for _, eventType := range []string{"error", "response.failed"} {
		input := []byte(`{"error":{"api\u005fkey":"synthetic-private-value",`)
		output, changed := sanitizeOpenAIResponseFailedEventForClient(input, eventType, true)
		require.True(t, changed)
		require.True(t, gjson.ValidBytes(output))
		require.Equal(t, eventType, gjson.GetBytes(output, "type").String())
		require.NotContains(t, string(output), "synthetic-private-value")
		if eventType == "response.failed" {
			require.Equal(t, "failed", gjson.GetBytes(output, "response.status").String())
		}
	}
}

func TestClientErrorPrivacyCompactAndOrdinaryAnthropicErrorBoundaries(t *testing.T) {
	const body = `{"error":{"type":"invalid_request_error","code":"invalid_parameter","message":"https://private-provider.example failed","api_key":"synthetic-opaque"}}`
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)
	MarkOpenAICompactClientStream(c)
	require.True(t, writeOpenAICompactSSEBridge(c, http.StatusOK, []byte(body)))
	require.NotContains(t, rec.Body.String(), "private-provider")
	require.NotContains(t, rec.Body.String(), "synthetic-opaque")

	rec = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	svc := &GatewayService{}
	resp := &http.Response{StatusCode: http.StatusBadRequest, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
	_, err := svc.handleErrorResponse(context.Background(), resp, c, &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey})
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NotContains(t, rec.Body.String(), "private-provider")
	require.NotContains(t, rec.Body.String(), "synthetic-opaque")
}
