package handler

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type opsAnthropicCompatUpstream struct {
	service.HTTPUpstream
	body io.ReadCloser
}

func (u *opsAnthropicCompatUpstream) DoWithTLS(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: u.body}, nil
}

type opsAnthropicCompatReadError struct{}

func (opsAnthropicCompatReadError) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// Run the real compatibility converter inside Ops middleware: testing the
// marker or a handwritten error frame alone misses terminal-frame recognition.
func TestOpsAnthropicCompatCCFailureRecordsTerminalStatus(t *testing.T) {
	const prefix = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_ops\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-sonnet-4-6\",\"usage\":{\"input_tokens\":11}}}\n\n"
	for _, test := range []struct {
		name   string
		status int
	}{
		{"missing_terminal", http.StatusBadGateway},
		{"read_error", http.StatusBadGateway},
		{"line_limit", http.StatusBadGateway},
		{"idle_timeout", http.StatusBadGateway},
		{"explicit_upstream_error", http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: 1024, StreamDataIntervalTimeout: 1}}
			var body io.ReadCloser
			switch test.name {
			case "read_error":
				body = io.NopCloser(io.MultiReader(strings.NewReader(prefix), opsAnthropicCompatReadError{}))
			case "line_limit":
				body = io.NopCloser(strings.NewReader(prefix + "data: " + strings.Repeat("x", 4096) + "\n\n"))
			case "idle_timeout":
				reader, writer := io.Pipe()
				t.Cleanup(func() { _ = writer.Close(); _ = reader.Close() })
				go func() { _, _ = io.WriteString(writer, prefix) }()
				body = reader
			case "explicit_upstream_error":
				body = io.NopCloser(strings.NewReader(prefix + "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"api_error\",\"message\":\"upstream failed\",\"status_code\":503}}\n\n"))
			default:
				body = io.NopCloser(strings.NewReader(prefix))
			}
			svc := service.NewGatewayService(
				nil, nil, nil, nil, nil, nil, nil, nil,
				cfg, nil, nil, nil, nil, nil, nil,
				&opsAnthropicCompatUpstream{body: body}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			)
			account := &service.Account{
				ID: 9451, Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "test-key"},
			}
			entry := captureOpsStreamStatusFidelity(t, "/v1/chat/completions", func(c *gin.Context) {
				_, err := svc.ForwardAsChatCompletions(context.Background(), c, account,
					[]byte(`{"model":"claude-sonnet-4-6","stream":true,"messages":[{"role":"user","content":"hello"}]}`), nil)
				require.Error(t, err)
			})
			require.Equal(t, test.status, entry.StatusCode)
			require.NotNil(t, entry.UpstreamStatusCode)
			require.Equal(t, test.status, *entry.UpstreamStatusCode)
		})
	}
}
