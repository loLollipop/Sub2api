package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAntigravityMalformedFunctionCallNonStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name   string
		frames string
		status int
	}{
		{"signature only", `data: {"response":{"candidates":[{"finishReason":"MALFORMED_FUNCTION_CALL","content":{"parts":[{"thoughtSignature":"test-signature"}]}}]}}` + "\n\n", http.StatusBadGateway},
		{"error after parts", `data: {"response":{"candidates":[{"content":{"parts":[{"functionCall":{"name":"read","args":{}}}]}}]}}` + "\n\n" + `data: {"response":{"candidates":[{"finishReason":"MALFORMED_FUNCTION_CALL"}]}}` + "\n\n", http.StatusBadGateway},
		{"error before usage", `data: {"response":{"candidates":[{"finishReason":"MALFORMED_FUNCTION_CALL"}]}}` + "\n\n" + `data: {"response":{"usageMetadata":{"promptTokenCount":1}}}` + "\n\n", http.StatusBadGateway},
		{"normal tool", `data: {"response":{"candidates":[{"finishReason":"STOP","content":{"parts":[{"functionCall":{"name":"read","args":{}}}]}}]}}` + "\n\n", http.StatusOK},
		{"normal text", `data: {"response":{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"hello"}]}}]}}` + "\n\n", http.StatusOK},
	} {
		for _, protocol := range []string{"responses", "chat"} {
			t.Run(protocol+"/"+tc.name, func(t *testing.T) {
				svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
				c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", nil)
				resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tc.frames))}
				var err error
				if protocol == "chat" {
					_, err = svc.handleChatCompletionsNonStreamingFromAntigravity(c, resp, time.Now(), "test-model")
				} else {
					_, err = svc.handleResponsesNonStreamingFromAntigravity(c, resp, time.Now(), "test-model", apicompat.ResponsesClientToolMapping{})
				}
				require.Equal(t, tc.status, recorder.Code)
				if tc.status == http.StatusBadGateway {
					require.Error(t, err)
					require.Contains(t, recorder.Body.String(), `"type":"malformed_function_call"`)
					require.Contains(t, recorder.Body.String(), "MALFORMED_FUNCTION_CALL")
					require.NotContains(t, recorder.Body.String(), `"status":"completed"`)
				} else {
					require.NoError(t, err)
					if protocol == "responses" {
						require.Contains(t, recorder.Body.String(), `"status":"completed"`)
					} else {
						require.Contains(t, recorder.Body.String(), `"choices"`)
					}
				}
			})
		}
	}
}

type malformedDrainBody struct {
	*strings.Reader
	eof atomic.Bool
}

func (b *malformedDrainBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if err == io.EOF {
		b.eof.Store(true)
	}
	return n, err
}
func (*malformedDrainBody) Close() error { return nil }

func TestAntigravityMalformedForwardRetainsUsageAndDrains(t *testing.T) {
	usage := `"usageMetadata":{"promptTokenCount":17,"candidatesTokenCount":3,"thoughtsTokenCount":2,"cachedContentTokenCount":4}`
	malformed := `"candidates":[{"finishReason":"MALFORMED_FUNCTION_CALL"}]`
	frame := func(fields string) string { return `data: {"response":{` + fields + `}}` + "\n\n" }
	for _, order := range []string{"before", "same", "after"} {
		frames := frame(malformed)
		switch order {
		case "before":
			frames = frame(usage) + frames
		case "same":
			frames = frame(malformed + "," + usage)
		case "after":
			frames += strings.Repeat(frame(`"responseId":"drain-padding"`), 64) + frame(usage)
		}
		for _, protocol := range []string{"chat", "responses", "claude"} {
			t.Run(protocol+"/"+order, func(t *testing.T) {
				body := &malformedDrainBody{Reader: strings.NewReader(frames)}
				upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}}}
				svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize, StreamDataIntervalTimeout: 1}, upstream)
				request := []byte(`{"model":"claude-sonnet-4-6","max_tokens":256,"messages":[{"role":"user","content":"hello"}]}`)
				if protocol == "responses" {
					request = []byte(`{"model":"claude-sonnet-4-6","input":"hello"}`)
				}
				c, recorder := newAntigravityCompatContext(http.MethodPost, "/", request)
				var result *ForwardResult
				var err error
				switch protocol {
				case "chat":
					result, err = svc.ForwardAsChatCompletions(context.Background(), c, newAntigravityCompatAccount(AccountTypeOAuth), request, nil)
				case "responses":
					result, err = svc.ForwardAsResponses(context.Background(), c, newAntigravityCompatAccount(AccountTypeOAuth), request, nil)
				case "claude":
					result, err = svc.Forward(context.Background(), c, newAntigravityCompatAccount(AccountTypeOAuth), request, false)
				}
				require.Error(t, err)
				require.ErrorIs(t, err, antigravity.ErrMalformedFunctionCall)
				var failover *UpstreamFailoverError
				require.NotErrorAs(t, err, &failover)
				require.Equal(t, http.StatusBadGateway, recorder.Code)
				require.Contains(t, recorder.Body.String(), "malformed_function_call")
				require.NotContains(t, recorder.Body.String(), `"status":"completed"`)
				require.Equal(t, 1, upstream.callCount)
				require.True(t, body.eof.Load(), "must drain the upstream after the malformed terminal")
				require.NotNil(t, result, "keep the billable partial result")
				require.Equal(t, 13, result.Usage.InputTokens)
				require.Equal(t, 5, result.Usage.OutputTokens)
				require.Equal(t, 4, result.Usage.CacheReadInputTokens)
			})
		}
	}
}

func TestAntigravityMalformedCollectorRetainsUsageOnReadFailure(t *testing.T) {
	frames := `data: {"response":{"candidates":[{"finishReason":"MALFORMED_FUNCTION_CALL"}],"usageMetadata":{"promptTokenCount":17}}}` + "\n\n"
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
	c, _ := newAntigravityCompatContext(http.MethodPost, "/", nil)
	resp := &http.Response{Body: &antigravityCompatErrorReader{data: []byte(frames), err: io.ErrUnexpectedEOF}}
	_, result, err := svc.collectClaudeStreamResponse(c, resp, time.Now(), "test-model")
	require.ErrorIs(t, err, antigravity.ErrMalformedFunctionCall)
	require.NotNil(t, result)
	require.Equal(t, 17, result.usage.InputTokens)
}
