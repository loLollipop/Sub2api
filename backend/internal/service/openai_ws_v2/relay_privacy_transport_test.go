package openai_ws_v2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

type relayPrivacyTransport struct{ conn *coderws.Conn }

func (c relayPrivacyTransport) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	return c.conn.Read(ctx)
}
func (c relayPrivacyTransport) WriteFrame(ctx context.Context, kind coderws.MessageType, payload []byte) error {
	return c.conn.Write(ctx, kind, payload)
}
func (c relayPrivacyTransport) Close() error { return c.conn.CloseNow() }

func TestRelayPrivacyRealWebSocketPreservesFramesAndRawObservation(t *testing.T) {
	const success = `{"type":"response.output_text.delta","delta":"new_api_error (request id: Synthetic123)"}`
	const official = `{"type":"error","error":{"type":"api_error","message":"busy (request_id: 00000000-0000-4000-8000-000000000001)"}}`
	const failure = `{"type":"response.failed","response":{"id":"resp_privacy","error":{"type":"one_api_error","message":"bad input (request id: Synthetic123)"},"usage":{"input_tokens":13,"output_tokens":3}}}`
	for _, kind := range []coderws.MessageType{coderws.MessageText, coderws.MessageBinary} {
		t.Run(kind.String(), func(t *testing.T) {
			type relayEvidence struct {
				result   RelayResult
				exit     *RelayExit
				observed []string
			}
			done := make(chan relayEvidence, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer func() { _ = conn.CloseNow() }()
				upstream := newPassthroughTestFrameConn([]passthroughTestFrame{{kind, []byte(success)}, {kind, []byte(official)}, {kind, []byte(failure)}}, true)
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				var observed []string
				result, exit := Relay(ctx, relayPrivacyTransport{conn}, upstream, []byte(`{"type":"response.create","model":"gpt-5.1","input":[]}`), RelayOptions{BeforeClientWrite: func(_ coderws.MessageType, payload []byte) { observed = append(observed, string(payload)) }})
				done <- relayEvidence{result, exit, observed}
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			require.NoError(t, err)
			defer func() { _ = conn.CloseNow() }()
			for i, want := range []string{success, official, ""} {
				actualKind, payload, err := conn.Read(ctx)
				require.NoError(t, err)
				require.Equal(t, kind, actualKind)
				if i < 2 {
					require.Equal(t, want, string(payload))
				} else {
					require.Contains(t, string(payload), `"message":"bad input"`)
					require.NotContains(t, string(payload), "one_api_error")
					require.NotContains(t, string(payload), "request id:")
				}
			}
			select {
			case ev := <-done:
				require.Nil(t, ev.exit)
				require.Equal(t, []string{success, official, failure}, ev.observed)
				require.Equal(t, int64(3), ev.result.UpstreamToClientFrames)
				if kind == coderws.MessageText {
					require.Equal(t, 13, ev.result.Usage.InputTokens)
					require.Equal(t, 3, ev.result.Usage.OutputTokens)
				}
			case <-ctx.Done():
				t.Fatal("relay failed to settle terminal")
			}
		})
	}
}
