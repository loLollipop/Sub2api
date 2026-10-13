//go:build unit

package handler

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type passthroughHeartbeatUpstream struct {
	service.HTTPUpstream
	body  io.ReadCloser
	calls int
}

func (u *passthroughHeartbeatUpstream) Do(*http.Request, string, int64, int) (*http.Response, error) {
	u.calls++
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: u.body}, nil
}

func TestResponsesPassthroughHeartbeatHandoff(t *testing.T) {
	// Constructors start process-lifetime cache/harvester goroutines. Keep those
	// outside the virtual-clock bubble; only this request belongs to its lifetime.
	upstream := &passthroughHeartbeatUpstream{}
	h := newOpenAIResponsesFailoverTestHandler(t, upstream, func(accounts []service.Account, cfg *config.Config) {
		cfg.Gateway.StreamKeepaliveInterval = 1
		cfg.Gateway.StreamDataIntervalTimeout = 5
		for i := range accounts {
			accounts[i].Type = service.AccountTypeAPIKey
			accounts[i].Credentials = map[string]any{"api_key": "test-key"}
			accounts[i].Extra = map[string]any{"openai_passthrough": true, "openai_responses_supported": true}
		}
	})
	synctest.Test(t, func(t *testing.T) {
		reader, writer := io.Pipe()
		defer reader.Close()
		defer writer.Close()
		upstream.body = reader
		c, recorder := newOpenAIResponsesFailoverTestContext(t, nil)
		c.Request.Body = io.NopCloser(strings.NewReader(`{"model":"gpt-5.1","input":"hello","stream":true}`))
		done := make(chan struct{})
		go func() { defer close(done); h.Responses(c) }()
		synctest.Wait()
		require.Equal(t, 1, upstream.calls)
		time.Sleep(2 * time.Second)
		synctest.Wait()
		require.Contains(t, recorder.Body.String(), ": keepalive\n\n")
		require.Equal(t, -1, service.OpenAICompactKeepaliveAdjustedWrittenSize(c))
		_, err := io.WriteString(writer, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_domestic\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}]}}\n\n")
		require.NoError(t, err)
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Fatal("terminal response did not release handler")
		}
		require.Equal(t, 1, upstream.calls)
		require.Equal(t, 1, strings.Count(recorder.Body.String(), `"type":"response.completed"`))
		require.NotContains(t, recorder.Body.String(), "response.failed")
	})
}
