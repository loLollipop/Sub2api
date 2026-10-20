package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Notifications let the test wait for actual downstream output without reading
// httptest.ResponseRecorder concurrently with the stream owner.
type releaseV227StreamWriter struct {
	gin.ResponseWriter
	writes   chan string
	failPing bool
}

func (w *releaseV227StreamWriter) Write(p []byte) (int, error) {
	if w.failPing && bytes.Contains(p, []byte("event: ping")) {
		w.writes <- string(p)
		return 0, errors.New("client disconnected")
	}
	n, err := w.ResponseWriter.Write(p)
	w.writes <- string(p[:n])
	return n, err
}

func releaseV227StreamContext() (*gin.Context, *httptest.ResponseRecorder, *releaseV227StreamWriter) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	w := &releaseV227StreamWriter{ResponseWriter: c.Writer, writes: make(chan string, 128)}
	c.Writer = w
	return c, rec, w
}

func releaseV227WaitForWrite(t *testing.T, writes <-chan string, want string) {
	t.Helper()
	timeout := time.NewTimer(4 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case p := <-writes:
			if strings.Contains(p, want) {
				return
			}
		case <-timeout.C:
			t.Fatalf("timed out waiting for downstream %q", want)
		}
	}
}

// A stopped manager must hand owner heartbeats back to the normal writer once
// the provider has armed readiness. A closed gate instead swallows the probe.
func releaseV227RequireGate(t *testing.T, c *gin.Context, ready bool) {
	t.Helper()
	n, err, handled := WriteOpenAIStreamSSEHeartbeat(c, []byte(": readiness probe\n\n"))
	require.NoError(t, err)
	require.Zero(t, n)
	require.Equal(t, !ready, handled)
}

func releaseV227BedrockFrame(event string) []byte {
	payload := []byte(`{"bytes":"` + base64.StdEncoding.EncodeToString([]byte(event)) + `"}`)
	var headers bytes.Buffer
	_ = headers.WriteByte(byte(len(":event-type")))
	_, _ = headers.WriteString(":event-type")
	_ = headers.WriteByte(7)
	_ = binary.Write(&headers, binary.BigEndian, uint16(len("chunk")))
	_, _ = headers.WriteString("chunk")
	var frame bytes.Buffer
	_ = binary.Write(&frame, binary.BigEndian, uint32(16+headers.Len()+len(payload)))
	_ = binary.Write(&frame, binary.BigEndian, uint32(headers.Len()))
	_ = binary.Write(&frame, binary.BigEndian, crc32.ChecksumIEEE(frame.Bytes()))
	_, _ = frame.Write(headers.Bytes())
	_, _ = frame.Write(payload)
	_ = binary.Write(&frame, binary.BigEndian, crc32.ChecksumIEEE(frame.Bytes()))
	return frame.Bytes()
}

func TestOpenAIStreamV227BedrockKeepaliveBeforeFirstFrame(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, disconnected := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "disconnected drains usage"}[disconnected], func(t *testing.T) {
			c, rec, w := releaseV227StreamContext()
			w.failPing = disconnected
			stop := StartOpenAIStreamSSEKeepaliveUntilUpstream(c, 5*time.Millisecond)
			defer stop()
			svc := &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{StreamKeepaliveInterval: 1}}}
			reader, writer := io.Pipe()
			defer reader.Close()
			defer writer.Close()
			type outcome struct {
				result *streamingResult
				err    error
			}
			done := make(chan outcome, 1)
			go func() {
				result, err := svc.handleBedrockStreamingResponse(context.Background(), &http.Response{
					StatusCode: http.StatusOK, Header: http.Header{}, Body: reader,
				}, c, &Account{ID: 1}, time.Now(), "claude-sonnet-4-5")
				done <- outcome{result, err}
			}()

			// Only release the first upstream frame after a real owner ping.
			releaseV227WaitForWrite(t, w.writes, "event: ping")
			_, err := writer.Write(releaseV227BedrockFrame(`{"type":"message_delta","usage":{"input_tokens":3,"output_tokens":7}}`))
			require.NoError(t, err)
			_, err = writer.Write(releaseV227BedrockFrame(`{"type":"message_stop"}`))
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			select {
			case got := <-done:
				require.NoError(t, got.err)
				require.Equal(t, disconnected, got.result.clientDisconnect)
				require.Equal(t, 3, got.result.usage.InputTokens)
				require.Equal(t, 7, got.result.usage.OutputTokens)
				require.NotNil(t, got.result.firstTokenMs)
				require.GreaterOrEqual(t, *got.result.firstTokenMs, 900)
			case <-time.After(4 * time.Second):
				t.Fatal("Bedrock stream did not finish")
			}
			releaseV227RequireGate(t, c, true)
			if !disconnected {
				require.Equal(t, http.StatusOK, rec.Code)
				require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
				require.Contains(t, rec.Body.String(), "event: message_stop")
			}
		})
	}
}

func TestOpenAIStreamV227BedrockKeepaliveDoesNotMaskIdleTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _, w := releaseV227StreamContext()
	stop := StartOpenAIStreamSSEKeepaliveUntilUpstream(c, 5*time.Millisecond)
	defer stop()
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	svc := &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		StreamKeepaliveInterval: 1, StreamDataIntervalTimeout: 2,
	}}}
	done := make(chan error, 1)
	go func() {
		result, err := svc.handleBedrockStreamingResponse(context.Background(), &http.Response{
			StatusCode: http.StatusOK, Header: http.Header{}, Body: reader,
		}, c, &Account{ID: 1}, time.Now(), "claude-sonnet-4-5")
		if result != nil && (result.firstTokenMs != nil || result.clientDisconnect) {
			done <- errors.New("idle pings changed first-token or disconnect state")
			return
		}
		done <- err
	}()
	releaseV227WaitForWrite(t, w.writes, "event: ping")
	select {
	case err := <-done:
		require.EqualError(t, err, "stream data interval timeout")
	case <-time.After(4 * time.Second):
		t.Fatal("owner keepalive masked the upstream idle watchdog")
	}
}

func TestOpenAIStreamV227ProviderSuccessOwnsStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize, StreamKeepaliveInterval: 1}}
	ag := newAntigravityTestService(cfg)
	for _, tt := range []struct {
		name string
		want string
		run  func(*gin.Context, *http.Response) error
	}{
		{"antigravity gemini", ":\n\n", func(c *gin.Context, resp *http.Response) error {
			_, err := ag.handleGeminiStreamingResponse(c, resp, time.Now())
			return err
		}},
		{"antigravity claude", "event: ping", func(c *gin.Context, resp *http.Response) error {
			_, err := ag.handleClaudeStreamingResponse(c, resp, time.Now(), "claude-sonnet-4-5")
			return err
		}},
		{"gemini messages compat", "event: message_start", func(c *gin.Context, resp *http.Response) error {
			_, err := (&GeminiMessagesCompatService{cfg: cfg}).handleStreamingResponse(c, resp, time.Now(), "gemini-2.5-pro")
			return err
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, rec, w := releaseV227StreamContext()
			stop := StartOpenAIStreamSSEKeepaliveUntilUpstream(c, 5*time.Millisecond)
			defer stop()
			reader, writer := io.Pipe()
			defer reader.Close()
			defer writer.Close()
			done := make(chan error, 1)
			go func() {
				done <- tt.run(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: reader})
			}()
			releaseV227WaitForWrite(t, w.writes, tt.want)
			_, err := io.WriteString(writer, `data: {"response":{"candidates":[{"content":{"parts":[{"text":"hello"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":7}}}`+"\n\n")
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(4 * time.Second):
				t.Fatal("provider stream did not finish")
			}
			releaseV227RequireGate(t, c, true)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Contains(t, rec.Header().Get("Content-Type"), "text/event-stream")
		})
	}
}

func TestOpenAIStreamV227ForwardUpstreamGatesUntilSuccessfulHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, status := range []int{http.StatusOK, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c, rec, w := releaseV227StreamContext()
			stop := StartOpenAIStreamSSEKeepaliveUntilUpstream(c, 5*time.Millisecond)
			defer stop()
			reader, writer := io.Pipe()
			defer reader.Close()
			defer writer.Close()
			resp := &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: reader}
			if status >= 400 {
				resp.Header.Set("Content-Type", "application/json")
				resp.Body = io.NopCloser(strings.NewReader(`{"error":{"message":"forbidden"}}`))
			}
			upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{resp}}
			requestStarted := make(chan struct{})
			releaseHeaders := make(chan struct{}, 1)
			defer close(releaseHeaders)
			upstream.onCall = func(_ *http.Request, _ *queuedHTTPUpstreamStub) {
				close(requestStarted)
				<-releaseHeaders
			}
			svc := newAntigravityTestService(&config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize, StreamKeepaliveInterval: 1}})
			svc.httpUpstream = upstream
			done := make(chan error, 1)
			go func() {
				_, err := svc.ForwardUpstream(context.Background(), c, &Account{
					ID: 1, Name: "upstream", Credentials: map[string]any{"base_url": "https://upstream.invalid", "api_key": "test"},
				}, []byte(`{"model":"claude-sonnet-4-5","stream":true,"messages":[{"role":"user","content":"hello"}]}`))
				done <- err
			}()
			select {
			case <-requestStarted:
			case <-time.After(4 * time.Second):
				t.Fatal("upstream request did not start")
			}
			select {
			case p := <-w.writes:
				t.Fatalf("downstream committed before upstream headers: %q", p)
			case <-time.After(50 * time.Millisecond):
			}
			releaseHeaders <- struct{}{}
			if status == http.StatusOK {
				releaseV227WaitForWrite(t, w.writes, "event: ping")
				_, err := io.WriteString(writer, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
				require.NoError(t, err)
				require.NoError(t, writer.Close())
			}
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(4 * time.Second):
				t.Fatal("forward did not finish")
			}
			releaseV227RequireGate(t, c, status == http.StatusOK)
			require.Equal(t, status, rec.Code)
			if status >= 400 {
				require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
				require.JSONEq(t, `{"error":{"message":"forbidden"}}`, rec.Body.String())
			} else {
				require.Contains(t, rec.Body.String(), "event: message_stop")
			}
		})
	}
}
