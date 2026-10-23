package service

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIRequestScopedRetryDelayBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, seconds, millis, body string
		want                        time.Duration
	}{
		{name: "fraction", seconds: "0.25", want: 250 * time.Millisecond},
		{name: "milliseconds", millis: "125", want: 125 * time.Millisecond},
		{name: "cap_seconds", seconds: "1e300", want: 8 * time.Second},
		{name: "cap_milliseconds", millis: "1e300", want: 8 * time.Second},
		{name: "invalid", seconds: "NaN", millis: "+Inf"},
		{name: "negative", seconds: "-2", millis: "-500"},
		{name: "body", body: `{"response":{"error":{"headers":{"retry-after-ms":"500"}}}}`, want: 500 * time.Millisecond},
		{name: "header_priority", seconds: "1", millis: "100", want: time.Second},
		{name: "date", seconds: time.Now().Add(time.Hour).UTC().Format(http.TimeFormat), want: 8 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := make(http.Header)
			h.Set("Retry-After", tc.seconds)
			h.Set("Retry-After-Ms", tc.millis)
			require.Equal(t, tc.want, openAIRequestScopedRetryDelay(h, []byte(tc.body)))
		})
	}
}

func TestOpenAIEmptyFramesAndObservedUsageReplayBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, native := range []bool{true, false} {
		for _, observed := range []bool{true, false} {
			t.Run(map[bool]string{true: "native", false: "passthrough"}[native]+map[bool]string{true: "_usage", false: "_empty"}[observed], func(t *testing.T) {
				stream := ""
				for _, payload := range []string{
					`{"type":"response.created","response":{"id":"resp_sync"}}`,
					`{"type":"response.output_text.delta","delta":""}`,
					`{"type":"response.output_text.done","text":""}`,
					`{"type":"response.refusal.done","refusal":""}`,
					`{"type":"response.content_part.done","part":{"type":"output_text","text":""}}`,
					`{"type":"response.reasoning_summary_part.done","part":{"type":"summary_text","text":""}}`,
				} {
					stream += "data: " + payload + "\n\n"
				}
				usage := ""
				if observed {
					usage = `,"usage":{"input_tokens":12,"output_tokens":0}`
				}
				stream += `data: {"type":"response.failed","response":{"id":"resp_sync","status":"failed","error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded."}` + usage + "}}\n\n"
				svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
				resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(stream))}
				account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
				var err error
				var gotTokens int
				if native {
					result, runErr := svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "model", "model")
					err = runErr
					if result != nil && result.usage != nil {
						gotTokens = result.usage.InputTokens
					}
				} else {
					result, runErr := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "model", "model")
					err = runErr
					if result != nil && result.usage != nil {
						gotTokens = result.usage.InputTokens
					}
				}
				require.Error(t, err)
				var failover *UpstreamFailoverError
				require.Equal(t, !observed, errors.As(err, &failover))
				if observed {
					require.Equal(t, 12, gotTokens)
				} else {
					require.False(t, c.Writer.Written())
					require.Empty(t, rec.Body.String())
				}
			})
		}
	}
}

func TestOpenAIEmptyFrameDetectionKeepsUnknownContent(t *testing.T) {
	for _, event := range []string{"response.output_text.delta", "response.output_text.done", "response.refusal.done"} {
		for _, payload := range []string{`{}`, `{"delta":null,"text":null,"refusal":null}`, `{"delta":{},"text":{},"refusal":{}}`} {
			require.True(t, openAIStreamDataStartsClientOutput(payload, event), "%s %s", event, payload)
		}
	}
	require.True(t, openAIStreamDataStartsClientOutput(`{"part":{"type":"output_text","text":null}}`, "response.content_part.done"))
	require.True(t, openAIStreamDataStartsClientOutput(`{"type":"response.function_call_arguments.done","arguments":""}`, "response.function_call_arguments.done"))
}

type upstreamSyncTrackingBody struct {
	*strings.Reader
	closed bool
}

func (b *upstreamSyncTrackingBody) Close() error {
	b.closed = true
	return nil
}

func TestOpenAIEmptyStreamBufferLimitPreservesUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, native := range []bool{true, false} {
		for _, observed := range []bool{true, false} {
			t.Run(map[bool]string{true: "native", false: "passthrough"}[native]+map[bool]string{true: "_usage", false: "_empty"}[observed], func(t *testing.T) {
				prefix := ""
				if observed {
					prefix = "data: {\"type\":\"response.in_progress\",\"response\":{\"usage\":{\"input_tokens\":12}}}\n\n"
				}
				frame := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"\",\"padding\":\"" + strings.Repeat("x", 32*1024) + "\"}\n\n"
				body := &upstreamSyncTrackingBody{Reader: strings.NewReader(prefix + strings.Repeat(frame, 320))}
				svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body}
				account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
				var err error
				var gotTokens int
				if native {
					result, runErr := svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "model", "model")
					err, gotTokens = runErr, result.usage.InputTokens
				} else {
					result, runErr := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "model", "model")
					err, gotTokens = runErr, result.usage.InputTokens
				}
				require.Error(t, err)
				var failover *UpstreamFailoverError
				require.Equal(t, native && !observed, errors.As(err, &failover))
				if observed {
					require.Equal(t, 12, gotTokens)
				} else {
					require.Zero(t, gotTokens)
				}
				require.True(t, body.closed)
				require.Positive(t, body.Len(), "stop at the buffer limit instead of consuming the whole stream")
				require.False(t, c.Writer.Written())
				require.Empty(t, rec.Body.String())
			})
		}
	}
}

func TestOpenAIEmptyStreamDisconnectRetainsObservedUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, native := range []bool{true, false} {
		for _, observed := range []bool{true, false} {
			for _, readError := range []bool{true, false} {
				t.Run(map[bool]string{true: "native", false: "passthrough"}[native]+map[bool]string{true: "_usage", false: "_empty"}[observed]+map[bool]string{true: "_read_error", false: "_eof"}[readError], func(t *testing.T) {
					usage := ""
					if observed {
						usage = `,"usage":{"input_tokens":12}`
					}
					stream := `data: {"type":"response.created","response":{"id":"resp_partial"` + usage + "}}\n\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"\"}\n\n"
					var reader io.Reader = strings.NewReader(stream)
					if readError {
						reader = io.MultiReader(reader, iotest.ErrReader(io.ErrUnexpectedEOF))
					}
					svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
					resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(reader)}
					account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
					var err error
					var gotTokens int
					if native {
						result, runErr := svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "model", "model")
						err, gotTokens = runErr, result.usage.InputTokens
					} else {
						result, runErr := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "model", "model")
						err, gotTokens = runErr, result.usage.InputTokens
					}
					require.Error(t, err)
					var failover *UpstreamFailoverError
					require.Equal(t, !observed, errors.As(err, &failover))
					if observed {
						require.Equal(t, 12, gotTokens)
						preserved, preservedErr := preserveOpenAIStreamingResultOnError(&OpenAIForwardResult{Usage: OpenAIUsage{InputTokens: gotTokens}}, err)
						require.Same(t, err, preservedErr)
						require.NotNil(t, preserved)
					}
					require.False(t, c.Writer.Written())
					require.Empty(t, rec.Body.String())
				})
			}
		}
	}
}

func TestOpenAIEmptyStreamTimeoutRetainsObservedUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, native := range []bool{true, false} {
		t.Run(map[bool]string{true: "native", false: "passthrough"}[native], func(t *testing.T) {
			reader, writer := io.Pipe()
			defer writer.Close()
			go func() {
				_, _ = io.WriteString(writer, "data: {\"type\":\"response.created\",\"response\":{\"usage\":{\"input_tokens\":12}}}\n\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"\"}\n\n")
			}()
			cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
			if native {
				cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 1
			} else {
				cfg.Gateway.StreamDataIntervalTimeout = 1
			}
			svc := &OpenAIGatewayService{cfg: cfg}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: reader}
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
			var err error
			var gotTokens int
			if native {
				result, runErr := svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "model", "model")
				err, gotTokens = runErr, result.usage.InputTokens
			} else {
				result, runErr := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "model", "model")
				err, gotTokens = runErr, result.usage.InputTokens
			}
			require.Error(t, err)
			var failover *UpstreamFailoverError
			require.False(t, errors.As(err, &failover))
			require.Equal(t, 12, gotTokens)
			require.False(t, c.Writer.Written())
			require.Empty(t, rec.Body.String())
		})
	}
}

func TestOpenAIObservedUsageSurvivesOverflowAndLateErrorUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, native := range []bool{true, false} {
		for _, overflow := range []bool{true, false} {
			t.Run(map[bool]string{true: "native", false: "passthrough"}[native]+map[bool]string{true: "_overflow_frame", false: "_late_usage"}[overflow], func(t *testing.T) {
				stream := ""
				if overflow {
					stream = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"\",\"padding\":\"" + strings.Repeat("x", openAIFirstOutputStageMaxBytes-1024) + "\"}\n\n"
				} else {
					stream = "data: {\"type\":\"error\",\"error\":{\"code\":\"context_length_exceeded\",\"message\":\"context window exceeded\"}}\n\n"
				}
				stream += "data: {\"type\":\"response.output_text.delta\",\"delta\":\"\",\"usage\":{\"input_tokens\":12},\"padding\":\"" + strings.Repeat("x", 2048) + "\"}\n\n"
				svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(stream))}
				account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
				var err error
				var gotTokens int
				var nonBillable bool
				if native {
					result, runErr := svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "model", "model")
					err, gotTokens, nonBillable = runErr, result.usage.InputTokens, result.nonBillableUpstreamError
				} else {
					result, runErr := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "model", "model")
					err, gotTokens, nonBillable = runErr, result.usage.InputTokens, result.nonBillableUpstreamError
				}
				require.Error(t, err)
				var failover *UpstreamFailoverError
				require.False(t, errors.As(err, &failover))
				require.Equal(t, 12, gotTokens)
				require.False(t, nonBillable)
				if overflow {
					require.ErrorIs(t, err, errOpenAIFirstOutputStageLimit)
					require.False(t, c.Writer.Written())
					require.Empty(t, rec.Body.String())
				}
			})
		}
	}
}
