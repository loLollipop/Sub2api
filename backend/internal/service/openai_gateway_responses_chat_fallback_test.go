//go:build unit

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
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestForwardResponses_ForceChatCompletionsRoutesNonStreamingToChatCompletions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false,"service_tier":"priority"}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_resp_chat_json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_json","object":"chat.completion","model":"gpt-5.4","service_tier":"default","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5,"prompt_tokens_details":{"cached_tokens":1}}}`,
		)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}
	SetActualOpenAIUpstreamEndpoint(c, "/v1/responses")

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "http://upstream.example/v1/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, "/v1/chat/completions", GetActualOpenAIUpstreamEndpoint(c))
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(upstream.lastReq.Context()))
	require.Equal(t, "hello", gjson.GetBytes(upstream.lastBody, "messages.0.content").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "input").Exists())
	require.Equal(t, "response", gjson.Get(rec.Body.String(), "object").String())
	require.Equal(t, "ok", gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
	require.Equal(t, 3, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Equal(t, 1, result.Usage.CacheReadInputTokens)
	require.NotNil(t, result.ServiceTier)
	require.Equal(t, "priority", *result.ServiceTier)
	require.Equal(t, "default", result.UpstreamResponseServiceTier)
	require.False(t, result.Stream)
}

// Scenario: 第三方无推理模型不收到兼容档位。
func TestForwardResponses_ForceChatCompletionsOmitsNoneReasoningEffort(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"company-coding-model","input":"hello","reasoning":{"effort":"none"},"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_none","object":"chat.completion","model":"company-coding-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
		)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "company-coding-model", gjson.GetBytes(upstream.lastBody, "model").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "reasoning_effort").Exists())
	require.Nil(t, result.ReasoningEffort)
}

func TestForwardResponses_PassthroughFlagWithUnsupportedResponsesUsesAccountMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, path := range []string{"/v1/responses", "/v1/responses/compact"} {
		path := path
		t.Run(path, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.4-channel","input":"hello","stream":false}`)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"id":"chatcmpl_mapping","object":"chat.completion","model":"gpt-5.4-account","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
				)),
			}}
			svc := &OpenAIGatewayService{
				cfg:          rawChatCompletionsTestConfig(),
				httpUpstream: upstream,
			}
			account := rawChatCompletionsTestAccount()
			account.Credentials["model_mapping"] = map[string]any{
				"gpt-5.4-channel": "gpt-5.4-account",
			}
			account.Credentials["compact_model_mapping"] = map[string]any{
				"gpt-5.4-account": "gpt-5.4-compact",
			}
			account.Extra = map[string]any{
				"openai_passthrough":                true,
				openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
			}

			result, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, "http://upstream.example/v1/chat/completions", upstream.lastReq.URL.String())
			require.Equal(t, "gpt-5.4-account", gjson.GetBytes(upstream.lastBody, "model").String())
		})
	}
}

func TestForwardResponses_ForceChatCompletionsRoutesStreamingToChatCompletions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"content":"he"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"content":"llo"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		"",
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[],"usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_resp_chat_stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "http://upstream.example/v1/chat/completions", upstream.lastReq.URL.String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream_options.include_usage").Bool())
	require.Contains(t, rec.Body.String(), "event: response.output_text.delta")
	require.Contains(t, rec.Body.String(), `"delta":"he"`)
	require.Contains(t, rec.Body.String(), "event: response.completed")
	require.Contains(t, rec.Body.String(), `"input_tokens":4`)
	require.Contains(t, rec.Body.String(), "data: [DONE]")
	require.Equal(t, 4, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.True(t, result.Stream)
	require.NotNil(t, result.FirstTokenMs)
}

func TestForwardResponses_ChatFallbackRejectsEmptyOrInvalidStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name string
		wire string
	}{
		{name: "empty EOF"},
		{name: "done only", wire: "data: [DONE]\n\n"},
		{name: "SSE comments", wire: ": keepalive\n\nevent: message\ndata: \n\n"},
		{name: "role and empty text", wire: "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"\",\"reasoning_content\":\"\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"},
		{name: "usage only", wire: "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":0}}\n\ndata: [DONE]\n\n"},
		{name: "invalid JSON", wire: "data: {broken\n\ndata: [DONE]\n\n"},
		{name: "unknown SSE JSON", wire: "data: {}\n\ndata: [DONE]\n\n"},
		{name: "empty tool placeholder", wire: "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n"},
		{name: "non SSE JSON", wire: `{"choices":[{"message":{"content":"not a stream"},"finish_reason":"stop"}]}`},
		{name: "non SSE error", wire: `{"error":{"message":"temporarily unavailable, please retry"}}`},
		{name: "pretty non SSE error", wire: "{\n  \"error\": {\n    \"code\": \"server_error\",\n    \"message\": \"temporarily unavailable, please retry\"\n  }\n}"},
		{name: "retryable SSE error", wire: "data: {\"error\":{\"code\":\"server_error\",\"message\":\"temporarily unavailable, please retry\"}}\n\ndata: [DONE]\n\n"},
		{name: "typed SSE error", wire: "event: error\ndata: {\"code\":\"request_timeout\",\"message\":\"temporary failure, please retry\"}\n\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err, rec, c := forwardResponsesChatFallbackStream(t, tt.wire)
			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Nil(t, result, "a retryable attempt must not be recorded as a successful usage row")
			require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
			require.False(t, c.Writer.Written(), "role/usage/lifecycle frames must not commit an empty attempt")
			require.Empty(t, rec.Body.String())
		})
	}
}

func TestForwardResponses_ChatFallbackReadErrorRespectsOutputBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, withOutput := range []bool{false, true} {
		wire := "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n"
		if withOutput {
			wire += "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":2}}\n\n"
		}
		result, err, rec, c := forwardResponsesChatFallbackStream(t, wire, io.ErrUnexpectedEOF)
		require.Error(t, err)
		var failoverErr *UpstreamFailoverError
		require.Equal(t, !withOutput, errors.As(err, &failoverErr))
		if withOutput {
			require.ErrorIs(t, err, io.ErrUnexpectedEOF)
			require.NotNil(t, result)
			require.Equal(t, 4, result.Usage.InputTokens)
			require.Equal(t, 2, result.Usage.OutputTokens)
			require.Contains(t, rec.Body.String(), "event: response.failed")
		} else {
			require.Nil(t, result)
			require.False(t, c.Writer.Written())
		}
		require.NotContains(t, rec.Body.String(), "event: response.completed")
	}
}

func TestForwardResponses_ChatFallbackDoesNotRetryRequestErrorsOrCancellation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name    string
		wire    string
		readErr error
	}{
		{name: "context length", wire: "data: {\"error\":{\"code\":\"context_length_exceeded\",\"type\":\"invalid_request_error\",\"message\":\"Your input exceeds the context window\"}}\n\n"},
		{name: "typed context length", wire: "event: error\ndata: {\"code\":\"context_length_exceeded\",\"message\":\"Your input exceeds the context window\"}\n\n"},
		{name: "JSON context length", wire: `{"error":{"code":"context_length_exceeded","type":"invalid_request_error","message":"Your input exceeds the context window"}}`},
		{name: "pretty JSON context length", wire: "{\n  \"error\": {\n    \"code\": \"context_length_exceeded\",\n    \"type\": \"invalid_request_error\",\n    \"message\": \"Your input exceeds the context window\"\n  }\n}"},
		{name: "unclassified error", wire: `{"error":{"message":"upstream failed"}}`},
		{name: "cancellation", readErr: context.Canceled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err, rec, _ := forwardResponsesChatFallbackStream(t, tt.wire, tt.readErr)
			require.Error(t, err)
			require.NotNil(t, result)
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr))
			require.NotContains(t, rec.Body.String(), "event: response.completed")
			if tt.readErr != nil {
				require.ErrorIs(t, err, tt.readErr)
				require.Empty(t, rec.Body.String())
			} else {
				require.Contains(t, rec.Body.String(), "event: response.failed")
				if strings.Contains(tt.name, "context length") {
					require.True(t, result.NonBillableUpstreamError)
					require.Contains(t, rec.Body.String(), `"code":"context_length_exceeded"`)
				}
			}
		})
	}
}

func TestForwardResponses_ChatFallbackRejectsTruncatedStreamAfterOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const content = "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}\n\n"
	for _, suffix := range []string{
		"",
		"data: {broken\n\n",
		"data: {\"error\":{\"message\":\"temporarily unavailable, please retry\"}}\n\ndata: [DONE]\n\n",
	} {
		t.Run(suffix, func(t *testing.T) {
			result, err, rec, _ := forwardResponsesChatFallbackStream(t, content+suffix)
			require.Error(t, err)
			require.NotNil(t, result)
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr), "partial semantic output cannot be replayed on another account")
			require.Contains(t, rec.Body.String(), `"delta":"partial"`)
			require.Contains(t, rec.Body.String(), "event: response.failed")
			require.NotContains(t, rec.Body.String(), "event: response.completed")
			require.NotContains(t, rec.Body.String(), "data: [DONE]")
			lastSequence := int64(-1)
			for _, line := range strings.Split(rec.Body.String(), "\n") {
				if payload, ok := extractOpenAISSEDataLine(line); ok {
					sequence := gjson.Get(payload, "sequence_number").Int()
					require.Greater(t, sequence, lastSequence)
					lastSequence = sequence
				}
			}
		})
	}
}

func TestForwardResponses_ChatFallbackAcceptsSemanticOutputAndTerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name string
		wire string
		want string
	}{
		{name: "finish without done", wire: "data: {\"choices\":[{\"delta\":{\"content\":\"answer\"},\"finish_reason\":\"stop\"}]}\n\n", want: "response.output_text.delta"},
		{name: "done without finish", wire: "data: {\"choices\":[{\"delta\":{\"content\":\"answer\"}}]}\n\ndata: [DONE]\n\n", want: "response.output_text.delta"},
		{name: "empty choices preamble", wire: "data: {\"choices\":[],\"usage\":null,\"error\":null}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"answer\"},\"finish_reason\":\"stop\"}]}\n\n", want: "response.output_text.delta"},
		{name: "usage without choices", wire: "data: {\"choices\":[{\"delta\":{\"content\":\"answer\"},\"finish_reason\":\"stop\"}]}\n\ndata: {\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":1}}\n\ndata: [DONE]\n\n", want: "response.output_text.delta"},
		{name: "reasoning alias", wire: "data: {\"choices\":[{\"delta\":{\"reasoning\":\"thinking\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", want: "response.reasoning_summary_text.delta"},
		{name: "refusal only", wire: "data: {\"choices\":[{\"delta\":{\"refusal\":\"I cannot help.\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", want: "response.refusal.delta"},
		{name: "inline buffered text", wire: "data: {\"choices\":[{\"delta\":{\"content\":\"<\"}}]}\n\ndata: [DONE]\n\n", want: "response.output_text.delta"},
		{name: "tool only", wire: "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"get_value\",\"arguments\":\"{}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n", want: "response.function_call_arguments.done"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err, rec, _ := forwardResponsesChatFallbackStream(t, tt.wire)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, result.FirstTokenMs)
			require.Contains(t, rec.Body.String(), tt.want)
			require.Contains(t, rec.Body.String(), "event: response.completed")
			require.Contains(t, rec.Body.String(), "data: [DONE]")
		})
	}
}

func forwardResponsesChatFallbackStream(t *testing.T, wire string, readErrors ...error) (*OpenAIForwardResult, error, *httptest.ResponseRecorder, *gin.Context) {
	t.Helper()
	body := []byte(`{"model":"glm-5.3","input":"hello","stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"rid_empty_chat_stream"}},
		Body:       io.NopCloser(strings.NewReader(wire)),
	}}
	if len(readErrors) > 0 && readErrors[0] != nil {
		upstream.resp.Body = &openAIStreamReadThenErrorCloser{reader: strings.NewReader(wire), err: readErrors[0]}
	}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	result, err := svc.Forward(c.Request.Context(), c, forceChatResponsesFallbackAccount(), body)
	return result, err, rec, c
}

func TestForwardResponses_ChatFallbackRefusalCannotReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const refusal = "data: {\"choices\":[{\"delta\":{\"refusal\":\"I cannot help.\"}}]}\n\n"
	for _, suffix := range []string{"", "data: {\"error\":{\"code\":\"server_error\",\"message\":\"please retry\"}}\n\n"} {
		result, err, rec, _ := forwardResponsesChatFallbackStream(t, refusal+suffix)
		require.Error(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.FirstTokenMs)
		var failoverErr *UpstreamFailoverError
		require.False(t, errors.As(err, &failoverErr))
		require.Contains(t, rec.Body.String(), "event: response.refusal.delta")
		require.Contains(t, rec.Body.String(), "event: response.failed")
		require.NotContains(t, rec.Body.String(), "event: response.completed")
	}
}

func TestScanCCStream_JSONErrorFormattingAndBounds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, contentType := range []string{"application/json", "text/event-stream", ""} {
		for _, pretty := range []bool{false, true} {
			for _, code := range []string{"context_length_exceeded", "server_error"} {
				t.Run(fmt.Sprintf("%s/pretty=%v/%s", contentType, pretty, code), func(t *testing.T) {
					message := "temporary failure, please retry"
					payload := fmt.Sprintf(`{"error":{"code":%q,"message":%q}}`, code, message)
					if pretty {
						payload = fmt.Sprintf("{\n  \"error\": {\n    \"code\": %q,\n    \"message\": %q\n  }\n}", code, message)
					}
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					resp := &http.Response{Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(payload))}
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
					scan := svc.scanCCStream(c, resp, "test", "", time.Now(), func(*apicompat.ChatCompletionsChunk) {
						t.Fatal("a JSON error must never be converted into output")
					}, true)
					require.Error(t, scan.Err)
					require.JSONEq(t, payload, string(scan.ErrorPayload))
					require.Equal(t, code == "server_error", openAIStreamErrorEventShouldFailover(scan.ErrorPayload, message))
				})
			}
		}
	}

	t.Run("bounded multiline body", func(t *testing.T) {
		svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
		limit := openAIUpstreamErrorBodyReadLimitForConfig(svc.cfg)
		reader := strings.NewReader("{\n" + strings.Repeat(" \n", int(limit)) + "}")
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		scan := svc.scanCCStream(c, &http.Response{Body: io.NopCloser(reader)}, "test", "", time.Now(), nil, true)
		require.ErrorContains(t, scan.Err, "body too large")
		require.Empty(t, scan.ErrorPayload)
		require.Positive(t, reader.Len(), "stop reading once the bounded JSON buffer is full")
	})

	t.Run("SSE line limit retained", func(t *testing.T) {
		cfg := rawChatCompletionsTestConfig()
		cfg.Gateway.MaxLineSize = 1024
		svc := &OpenAIGatewayService{cfg: cfg}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		resp := &http.Response{Body: io.NopCloser(strings.NewReader("data: " + strings.Repeat("x", 4096)))}
		scan := svc.scanCCStream(c, resp, "test", "", time.Now(), nil, true)
		require.ErrorContains(t, scan.Err, "token too long")
	})
}

func TestForwardResponses_ChatFallbackSanitizesFailureAfterOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, eventType := range []string{"error", "response.failed"} {
		for _, tt := range []struct {
			code    string
			message string
			want    string
		}{
			{code: "server_is_overloaded", message: "servers overloaded", want: "server_error"},
			{code: "slow_down", message: "slow down", want: "server_error"},
			{code: "rate_limit_exceeded", message: "try again in 3s", want: "rate_limit_exceeded"},
			{code: "insufficient_quota", message: "insufficient quota: billing balance exhausted", want: "upstream_error"},
		} {
			t.Run(eventType+"/"+tt.code, func(t *testing.T) {
				errObject := fmt.Sprintf(`{"code":%q,"message":%q}`, tt.code, tt.message)
				payload := fmt.Sprintf(`{"type":"error","error":%s}`, errObject)
				if eventType == "response.failed" {
					payload = fmt.Sprintf(`{"type":"response.failed","response":{"error":%s}}`, errObject)
				}
				wire := "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\ndata: " + payload + "\n\n"
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				body := []byte(`{"model":"glm-5.3","input":"hello","stream":true}`)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
				cfg := rawChatCompletionsTestConfig()
				cfg.Gateway.LogUpstreamErrorBody = true
				upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(wire))}}
				svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
				result, err := svc.Forward(c.Request.Context(), c, forceChatResponsesFallbackAccount(), body)
				require.Error(t, err)
				require.NotNil(t, result)
				var failoverErr *UpstreamFailoverError
				require.False(t, errors.As(err, &failoverErr))
				lastSequence := int64(-1)
				var failure gjson.Result
				for _, line := range strings.Split(rec.Body.String(), "\n") {
					if data, ok := extractOpenAISSEDataLine(line); ok {
						frame := gjson.Parse(data)
						require.Greater(t, frame.Get("sequence_number").Int(), lastSequence)
						lastSequence = frame.Get("sequence_number").Int()
						if frame.Get("type").String() == "response.failed" {
							failure = frame
						}
					}
				}
				require.True(t, failure.Exists())
				require.Equal(t, tt.want, failure.Get("response.error.code").String())
				require.NotContains(t, rec.Body.String(), "event: response.completed")
				if tt.code == "insufficient_quota" {
					require.Equal(t, UpstreamUnavailableMessage, failure.Get("response.error.message").String())
					require.NotContains(t, rec.Body.String(), "insufficient_quota")
					require.NotContains(t, rec.Body.String(), "billing")
				} else {
					require.Equal(t, tt.message, failure.Get("response.error.message").String())
				}
				// Client rewriting must not alter the monitored upstream payload.
				require.Contains(t, c.GetString(OpsUpstreamErrorDetailKey), tt.code)
			})
		}
	}
}

func TestForwardResponses_ChatFallbackRejectsInvalidToolArgumentsAtOutputLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"deepseek-v4-flash","input":"run the command","stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_length_tool","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_length","type":"function","function":{"name":"exec_command","arguments":"{\"cmd\":\"ssh root@HOST"}}]},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_length_tool","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{},"finish_reason":"length"}],"usage":{"prompt_tokens":4,"completion_tokens":6492,"total_tokens":6496}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_length_tool"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.ErrorContains(t, err, "invalid JSON")
	require.NotNil(t, result)
	require.Equal(t, 4, result.Usage.InputTokens)
	require.Equal(t, 6492, result.Usage.OutputTokens)
	require.NotContains(t, rec.Body.String(), "response.function_call_arguments.done")
	require.NotContains(t, rec.Body.String(), "response.output_item.done")
	require.NotContains(t, rec.Body.String(), "data: [DONE]")
}

func TestForwardResponses_DeepSeekReasoningOnlyStreamProducesVisibleText(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"deepseek-reasoner","input":"hello","stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_reasoning","object":"chat.completion.chunk","model":"deepseek-reasoner","choices":[{"index":0,"delta":{"role":"assistant","content":null,"reasoning_content":""},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_reasoning","object":"chat.completion.chunk","model":"deepseek-reasoner","choices":[{"index":0,"delta":{"reasoning_content":"visible fallback"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_reasoning","object":"chat.completion.chunk","model":"deepseek-reasoner","choices":[{"index":0,"delta":{"content":""},"finish_reason":"length"}],"usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_deepseek_reasoning_responses_stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Contains(t, rec.Body.String(), "event: response.output_text.delta")
	require.Contains(t, rec.Body.String(), `"delta":"visible fallback"`)
	require.Contains(t, rec.Body.String(), `"status":"incomplete"`)
	require.Contains(t, rec.Body.String(), "data: [DONE]")
}

func TestForwardResponses_AutoSupportedAccountStillUsesResponsesEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_resp_native"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"resp_native","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}],"status":"completed"}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}`,
		)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}
	account := rawChatCompletionsTestAccount()
	account.Extra = map[string]any{
		openai_compat.ExtraKeyResponsesMode:      string(openai_compat.ResponsesSupportModeAuto),
		openai_compat.ExtraKeyResponsesSupported: true,
	}

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "http://upstream.example/v1/responses", upstream.lastReq.URL.String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "input").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "messages").Exists())
	require.Equal(t, "ok", gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
}

func forceChatResponsesFallbackAccount() *Account {
	account := rawChatCompletionsTestAccount()
	account.Extra = map[string]any{
		openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
	}
	return account
}

func TestShouldPreemptivelyConvertInboundResponsesToChat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{name: "nil", want: false},
		{name: "oauth never", account: &Account{Type: AccountTypeOAuth, Platform: PlatformOpenAI}, want: false},
		{name: "unknown extra keeps responses", account: &Account{Type: AccountTypeAPIKey, Platform: PlatformOpenAI}, want: false},
		{name: "probe false still tries responses", account: &Account{Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Extra: map[string]any{openai_compat.ExtraKeyResponsesSupported: false}}, want: false},
		{name: "force chat", account: forceChatResponsesFallbackAccount(), want: true},
		{name: "force responses overrides probe false", account: &Account{Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Extra: map[string]any{openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceResponses), openai_compat.ExtraKeyResponsesSupported: false}}, want: false},
		{name: "cn chat protocol", account: &Account{Type: AccountTypeAPIKey, Platform: PlatformZhipu, Credentials: map[string]any{"api_protocol": APIProtocolChatCompletions}}, want: true},
		{name: "deepseek responses protocol", account: &Account{Type: AccountTypeAPIKey, Platform: PlatformDeepseek, Credentials: map[string]any{"api_protocol": APIProtocolResponses}}, want: false},
		{name: "kimi adaptive native", account: &Account{Type: AccountTypeAPIKey, Platform: PlatformKimi, Credentials: map[string]any{"api_protocol": APIProtocolAdaptive}}, want: false},
		{name: "zhipu adaptive no native", account: &Account{Type: AccountTypeAPIKey, Platform: PlatformZhipu, Credentials: map[string]any{"api_protocol": APIProtocolAdaptive}}, want: true},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, shouldPreemptivelyConvertInboundResponsesToChat(tt.account))
		})
	}
}

func TestShouldForwardOpenAIResponsesViaRawChatCompletions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{name: "nil", want: false},
		{name: "oauth never", account: &Account{Type: AccountTypeOAuth, Platform: PlatformOpenAI}, want: false},
		{name: "unknown extra keeps responses", account: &Account{Type: AccountTypeAPIKey, Platform: PlatformOpenAI}, want: false},
		{name: "probe false keeps inbound chat on raw CC", account: &Account{Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Extra: map[string]any{openai_compat.ExtraKeyResponsesSupported: false}}, want: true},
		{name: "force chat", account: forceChatResponsesFallbackAccount(), want: true},
		{name: "force responses overrides probe false", account: &Account{Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Extra: map[string]any{openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceResponses), openai_compat.ExtraKeyResponsesSupported: false}}, want: false},
		{name: "cn chat protocol", account: &Account{Type: AccountTypeAPIKey, Platform: PlatformZhipu, Credentials: map[string]any{"api_protocol": APIProtocolChatCompletions}}, want: true},
		{name: "deepseek responses protocol", account: &Account{Type: AccountTypeAPIKey, Platform: PlatformDeepseek, Credentials: map[string]any{"api_protocol": APIProtocolResponses}}, want: false},
		{name: "kimi adaptive native", account: &Account{Type: AccountTypeAPIKey, Platform: PlatformKimi, Credentials: map[string]any{"api_protocol": APIProtocolAdaptive}}, want: false},
		{name: "zhipu adaptive no native", account: &Account{Type: AccountTypeAPIKey, Platform: PlatformZhipu, Credentials: map[string]any{"api_protocol": APIProtocolAdaptive}}, want: true},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, shouldForwardOpenAIResponsesViaRawChatCompletions(tt.account))
		})
	}
}

func TestForwardResponses_ProbeFalseTriesVersionedResponsesThenFallsBackToChat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusNotFound,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"not found"}}`)),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_resp_chat_fallback"}},
			Body: io.NopCloser(strings.NewReader(
				`{"id":"chatcmpl_fallback","object":"chat.completion","model":"gpt-5.4","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`,
			)),
		},
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}
	account := rawChatCompletionsTestAccount()
	account.Extra = map[string]any{openai_compat.ExtraKeyResponsesSupported: false}

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "http://upstream.example/v1/responses", upstream.requests[0].URL.String())
	require.Equal(t, "http://upstream.example/v1/chat/completions", upstream.requests[1].URL.String())
	require.True(t, gjson.GetBytes(upstream.bodies[0], "input").Exists())
	require.Equal(t, "hello", gjson.GetBytes(upstream.bodies[1], "messages.0.content").String())
	require.Equal(t, "/v1/chat/completions", GetActualOpenAIUpstreamEndpoint(c))
	require.Equal(t, "ok", gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
}

func TestForwardResponses_SupportedAccountKeepsResponsesOn400(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"bad request"}}`)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}
	account := rawChatCompletionsTestAccount()
	account.Extra = map[string]any{openai_compat.ExtraKeyResponsesSupported: true}

	_, err := svc.Forward(context.Background(), c, account, body)
	require.Error(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "http://upstream.example/v1/responses", upstream.requests[0].URL.String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "input").Exists())
	require.NotEqual(t, "/v1/chat/completions", GetActualOpenAIUpstreamEndpoint(c))
}

// reasoningRecordingCache 记录 reasoning 缓存写入、并按需响应回查。
type reasoningRecordingCache struct {
	stubGatewayCache
	mu      sync.Mutex
	sets    map[string]string
	getResp map[string]string
}

func (c *reasoningRecordingCache) SetReasoningContent(_ context.Context, itemID string, content string, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sets == nil {
		c.sets = make(map[string]string)
	}
	c.sets[itemID] = content
	return nil
}

func (c *reasoningRecordingCache) GetReasoningContent(_ context.Context, itemID string) (string, error) {
	if v, ok := c.getResp[itemID]; ok {
		return v, nil
	}
	return "", ErrReasoningContentNotFound
}

func (c *reasoningRecordingCache) snapshotSets() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]string, len(c.sets))
	for k, v := range c.sets {
		out[k] = v
	}
	return out
}

// 流式响应里的 reasoning_content 应按 reasoning item id 写入缓存，供后续轮次
// 客户端不回传明文 summary 时回注（DeepSeek thinking mode 400 修复的写入侧）。
func TestForwardResponses_ChatFallbackCachesStreamedReasoning(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"deepseek-reasoner","input":"hello","stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_rc","object":"chat.completion.chunk","model":"deepseek-reasoner","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_rc","object":"chat.completion.chunk","model":"deepseek-reasoner","choices":[{"index":0,"delta":{"reasoning_content":"think "},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_rc","object":"chat.completion.chunk","model":"deepseek-reasoner","choices":[{"index":0,"delta":{"reasoning_content":"first"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_rc","object":"chat.completion.chunk","model":"deepseek-reasoner","choices":[{"index":0,"delta":{"content":"answer"},"finish_reason":"stop"}]}`,
		"",
		`data: {"id":"chatcmpl_rc","object":"chat.completion.chunk","model":"deepseek-reasoner","choices":[],"usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_reasoning_cache_stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	cache := &reasoningRecordingCache{}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
		cache:        cache,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)

	sets := cache.snapshotSets()
	require.Len(t, sets, 1, "应恰好缓存一个 reasoning item")
	for itemID, content := range sets {
		require.NotEmpty(t, itemID)
		require.Equal(t, "think first", content)
	}
}

// 请求侧：encrypted-only reasoning item（无明文 summary）经缓存回查补回
// reasoning_content；带明文 summary 的 item 顺手回写缓存（自愈）。
func TestForwardResponses_ChatFallbackRestoresReasoningFromCache(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{
		"model":"deepseek-reasoner",
		"stream":false,
		"input":[
			{"type":"reasoning","id":"item_plain","summary":[{"type":"summary_text","text":"plain thinking"}]},
			{"type":"function_call","call_id":"call_0","name":"get_value","arguments":"{}"},
			{"type":"function_call_output","call_id":"call_0","output":"ok"},
			{"type":"reasoning","id":"item_enc1","summary":[],"encrypted_content":"opaque"},
			{"type":"function_call","call_id":"call_1","name":"get_value","arguments":"{}"},
			{"type":"function_call_output","call_id":"call_1","output":"ok"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"go on"}]}
		]
	}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_reasoning_cache_restore"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_restore","object":"chat.completion","model":"deepseek-reasoner","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`,
		)),
	}}
	cache := &reasoningRecordingCache{
		getResp: map[string]string{"item_enc1": "cached thinking"},
	}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
		cache:        cache,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)

	// 明文 summary 的 assistant 工具调用消息：reasoning_content 来自 summary 本身。
	require.Equal(t, "plain thinking", gjson.GetBytes(upstream.lastBody, "messages.0.reasoning_content").String())
	require.Equal(t, "call_0", gjson.GetBytes(upstream.lastBody, "messages.0.tool_calls.0.id").String())
	require.Equal(t, "tool", gjson.GetBytes(upstream.lastBody, "messages.1.role").String())
	// encrypted-only 的 assistant 工具调用消息：reasoning_content 来自缓存回查。
	require.Equal(t, "cached thinking", gjson.GetBytes(upstream.lastBody, "messages.2.reasoning_content").String())
	require.Equal(t, "call_1", gjson.GetBytes(upstream.lastBody, "messages.2.tool_calls.0.id").String())
	require.Equal(t, "tool", gjson.GetBytes(upstream.lastBody, "messages.3.role").String())

	// 明文 summary 的 item 被回写进缓存（自愈）。
	require.Equal(t, "plain thinking", cache.snapshotSets()["item_plain"])
}
