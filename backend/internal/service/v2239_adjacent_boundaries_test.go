package service

import (
	"bytes"
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
)

func TestV2239AdjacentAnthropicDuplicateErrorsAreTerminal(t *testing.T) {
	const diagnostic = `"message":"https://private-provider.example","api_key":"synthetic-private-value"`
	const prefix = "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":11,\"cache_read_input_tokens\":7}}}\n\n" +
		"data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"https://public.example/article\"}}\n\n" +
		"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":3}}\n\n"
	const trailing = "data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":999}}\n\ndata: {\"type\":\"message_stop\"}\n\n"
	for name, fields := range map[string]string{
		"type success then error": `"type":"message_delta","type":"error","error":{"type":"relay_stream_error",` + diagnostic + `}`,
		"type error then success": `"type":"error","type":"message_delta","error":{"type":"relay_stream_error",` + diagnostic + `}`,
		"error then null":         `"type":"message_delta","error":{"type":"relay_stream_error"},"error":null,` + diagnostic,
		"null then error":         `"type":"message_delta","error":null,"error":{"type":"relay_stream_error"},` + diagnostic,
		"failed then completed":   `"type":"message_delta","status":"failed","status":"completed",` + diagnostic,
		"completed then failed":   `"type":"message_delta","status":"completed","status":"failed",` + diagnostic,
		"nested response":         `"type":"message_delta","response":{"status":"completed","status":"failed"},` + diagnostic,
	} {
		t.Run(name, func(t *testing.T) {
			payload := "{" + fields + `,"usage":{"output_tokens":998}}`
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader(prefix + "data: " + payload + "\n\n" + trailing)),
			}}
			svc := newForwardPartialUsageServiceForTest(nil)
			svc.httpUpstream = upstream
			result, err := svc.Forward(t.Context(), c, newAnthropicAPIKeyAccountForTest(), &ParsedRequest{
				Body:  NewRequestBodyRef([]byte(`{"model":"claude-3-7-sonnet-20250219","stream":true,"messages":[{"role":"user","content":"hello"}]}`)),
				Model: "claude-3-7-sonnet-20250219", Stream: true,
			})
			var failure *AnthropicPassthroughStreamError
			require.ErrorAs(t, err, &failure)
			require.Equal(t, http.StatusBadGateway, failure.StatusCode)
			require.True(t, failure.ResponseWritten)
			require.Equal(t, payload, failure.RawData, "classification keeps every original duplicate key")
			var failover *UpstreamFailoverError
			require.False(t, errors.As(err, &failover), "accepted work cannot replay")
			require.Len(t, upstream.requests, 1)
			require.NotNil(t, result)
			require.Equal(t, 11, result.Usage.InputTokens)
			require.Equal(t, 7, result.Usage.CacheReadInputTokens)
			require.Equal(t, 3, result.Usage.OutputTokens, "neither terminal error usage nor subsequent fake usage is billable")
			require.True(t, strings.HasPrefix(rec.Body.String(), prefix))
			require.NotContains(t, rec.Body.String(), "private-provider")
			require.NotContains(t, rec.Body.String(), "synthetic-private-value")
			require.NotContains(t, rec.Body.String(), "message_stop")
			marker, ok := GetOpsStreamError(c)
			require.True(t, ok)
			require.True(t, marker.CountTowardsSLA)
		})
	}
}

func TestV2239AdjacentAnthropicSuccessfulDeltaRemainsUnchanged(t *testing.T) {
	const wire = "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":11}}}\n\n" +
		"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"https://public.example/article\",\"error\":{},\"status\":\"failed\"}}\n\n" +
		"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":3}}\n\ndata: {\"type\":\"message_stop\"}\n\n"
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK,
		Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}}
	svc := newForwardPartialUsageServiceForTest(nil)
	svc.httpUpstream = upstream
	result, err := svc.Forward(t.Context(), c, newAnthropicAPIKeyAccountForTest(), &ParsedRequest{
		Body:  NewRequestBodyRef([]byte(`{"model":"claude-3-7-sonnet-20250219","stream":true,"messages":[{"role":"user","content":"hello"}]}`)),
		Model: "claude-3-7-sonnet-20250219", Stream: true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 11, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.Equal(t, wire, rec.Body.String(), "nested model content is not an error envelope")
	require.Len(t, upstream.requests, 1)
	_, failed := GetOpsStreamError(c)
	require.False(t, failed)
}

func TestV2239AdjacentGeminiProjectionPreservesRawErrors(t *testing.T) {
	for _, fields := range []string{
		`"error":{},"error":null`, `"error":null,"error":{}`,
		`"status":"failed","status":"completed"`, `"status":"completed","status":"failed"`,
	} {
		for _, detail := range []bool{false, true} {
			for _, mapped := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/detail=%t/mapped=%t", fields, detail, mapped), func(t *testing.T) {
					metadata := `"models":[{"name":"models/gemini-native","custom":42}]`
					if detail {
						metadata = `"name":"models/gemini-native","custom":42`
					}
					body := "{" + metadata + "," + fields + `,"message":"https://private-provider.example","api_key":"synthetic-private-value"}`
					upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK,
						Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}}
					svc := &GeminiMessagesCompatService{cfg: &config.Config{}, httpUpstream: upstream}
					account := &Account{ID: 1, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture-only"}}
					model := "gemini-native"
					if mapped {
						model = "gemini-public"
						account.Credentials["model_mapping"] = map[string]any{model: "gemini-native"}
					}
					var result *UpstreamHTTPResult
					var err error
					if detail {
						result, err = svc.ForwardGetModel(t.Context(), account, model)
					} else {
						result, err = svc.ForwardListModels(t.Context(), account)
					}
					require.NoError(t, err)
					require.Equal(t, http.StatusOK, result.StatusCode)
					require.Equal(t, body, string(result.Body), "projection must preserve raw error evidence for the final write")
					require.Len(t, upstream.requests, 1)
				})
			}
		}
	}
}

func TestV2239AdjacentOpenAIResponseWrites(t *testing.T) {
	const success = `{"model":"fixture-model","choices":[{"message":{"role":"assistant","content":"https://public.example/article"}}],"data":[{"url":"https://public.example/article"}],"usage":{"prompt_tokens":13,"completion_tokens":2}}`
	const failure = `{"error":{"type":"invalid_request_error","message":"https://private-provider.example","api_key":"synthetic-private-value"},"usage":{"prompt_tokens":13,"completion_tokens":2}}`
	for _, protocol := range []string{"raw-chat-buffer", "embeddings"} {
		for _, contentType := range []string{
			`application/json; upstream="https://private-provider.example"; api_key="synthetic-private-value"`,
			"not a media type", "application/json; charset=utf-8",
		} {
			for _, filtered := range []bool{false, true} {
				for _, kind := range []string{"success-200", "error-200", "error-400"} {
					if protocol == "raw-chat-buffer" && kind == "error-400" {
						continue // HTTP failures have an earlier owner in raw chat forwarding.
					}
					t.Run(fmt.Sprintf("%s/%s/filter=%t/%s", protocol, contentType, filtered, kind), func(t *testing.T) {
						body, status := success, http.StatusOK
						if kind != "success-200" {
							body = failure
						}
						if kind == "error-400" {
							status = http.StatusBadRequest
						}
						original := []byte(body)
						resp := &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(bytes.NewReader(original))}
						rec := httptest.NewRecorder()
						c, _ := gin.CreateTestContext(rec)
						c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
						upstream := &httpUpstreamRecorder{resp: resp}
						svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
						if filtered {
							svc.responseHeaderFilter = responseheaders.CompileHeaderFilter(config.ResponseHeaderConfig{})
						}
						account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture-only"}}
						var result *OpenAIForwardResult
						var err error
						if protocol == "raw-chat-buffer" {
							result, err = svc.bufferRawChatCompletions(c, resp, account, "fixture-model", "fixture-model", "fixture-model", nil, nil, time.Now())
						} else {
							result, err = svc.ForwardEmbeddings(t.Context(), c, account, []byte(`{"model":"fixture-model","input":"hello"}`), "")
							require.Len(t, upstream.requests, 1)
						}
						if status == http.StatusBadRequest {
							require.EqualError(t, err, "upstream returned status 400")
							require.Nil(t, result)
							internalBody, readErr := io.ReadAll(resp.Body)
							require.NoError(t, readErr)
							require.Equal(t, body, string(internalBody), "the retained upstream body remains unredacted")
						} else {
							require.NoError(t, err)
							require.NotNil(t, result)
							require.Equal(t, 13, result.Usage.InputTokens)
							require.Equal(t, 2, result.Usage.OutputTokens)
							require.Equal(t, contentType, result.UpstreamHeaders.Get("Content-Type"), "internal response metadata remains raw")
						}
						require.Equal(t, body, string(original))
						require.Equal(t, status, rec.Code)
						wantContentType := "application/json"
						if contentType == "application/json; charset=utf-8" {
							wantContentType = contentType
						}
						require.Equal(t, wantContentType, rec.Header().Get("Content-Type"))
						if kind == "success-200" {
							require.Equal(t, success, rec.Body.String())
						} else {
							require.NotContains(t, rec.Body.String(), "private-provider")
							require.NotContains(t, rec.Body.String(), "synthetic-private-value")
						}
					})
				}
			}
		}
	}
}
