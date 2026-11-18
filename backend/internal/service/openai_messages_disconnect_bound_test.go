//go:build unit

package service

import (
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAnthropicMessagesDisconnectDrainIsBoundedWithContinuousHeartbeats(t *testing.T) {
	rec, c, _ := newPreContentContext()
	c.Writer = &openAICompatFailingWriter{ResponseWriter: c.Writer, failAfter: 0}
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	upstreamDone := make(chan struct{})
	go func() {
		defer close(upstreamDone)
		defer writer.Close()
		for _, frame := range []string{
			preContentCreatedFrame,
			"data: {\"type\":\"response.in_progress\",\"response\":{\"usage\":{\"input_tokens\":3,\"output_tokens\":0}}}\n\n",
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n",
		} {
			if _, err := io.WriteString(writer, frame); err != nil {
				return
			}
		}
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			if _, err := fmt.Fprint(writer, ": heartbeat\n\n"); err != nil {
				return
			}
		}
	}()
	response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: reader}
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 1}}}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	type outcome struct {
		result *OpenAIForwardResult
		err    error
	}
	finished := make(chan outcome, 1)
	go func() {
		result, err := svc.handleAnthropicStreamingResponse(response, c, account, "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now())
		finished <- outcome{result, err}
	}()
	select {
	case got := <-finished:
		require.Error(t, got.err)
		require.NotNil(t, got.result)
		require.True(t, got.result.ClientDisconnect)
		require.Equal(t, 3, got.result.Usage.InputTokens)
		require.Empty(t, rec.Body.String())
		_, hasOpsError := c.Get(OpsUpstreamErrorsKey)
		require.False(t, hasOpsError)
	case <-time.After(3 * time.Second):
		t.Fatal("client disconnect must stop draining despite continuous upstream heartbeats")
	}
	select {
	case <-upstreamDone:
	case <-time.After(time.Second):
		t.Fatal("disconnect timeout must close the upstream body")
	}
}
