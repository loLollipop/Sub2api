//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestModelScopeWSPassthroughSessionInheritanceAndRefusal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, cancel := context.WithCancel(ContextWithModelScope(context.Background(), ModelScopeOpenAI))
	defer cancel()
	upstream := newStagedPassthroughConn()
	svc := newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream)
	server, serverErr := startPassthroughLifecycleServer(t, ctx, svc, passthroughLifecycleAccount())
	defer server.Close()
	client := dialPassthroughLifecycleClient(t, server)
	defer client.CloseNow()
	require.Equal(t, "gpt-5.1", gjson.GetBytes(requirePassthroughUpstreamWrite(t, upstream, time.Second), "model").String())
	upstream.Send(`{"type":"response.completed","response":{"id":"first","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	_, err := readPassthroughLifecycleFrame(t, client, time.Second)
	require.NoError(t, err)
	write := func(payload string) {
		t.Helper()
		writeCtx, stop := context.WithTimeout(ctx, time.Second)
		defer stop()
		require.NoError(t, client.Write(writeCtx, coderws.MessageText, []byte(payload)))
	}
	write(`{"type":"session.update","session":{"model":"gpt-5.4"}}`)
	require.Equal(t, "gpt-5.4", gjson.GetBytes(requirePassthroughUpstreamWrite(t, upstream, time.Second), "session.model").String())
	write(`{"type":"response.create","stream":false}`)
	require.Equal(t, "gpt-5.4", gjson.GetBytes(requirePassthroughUpstreamWrite(t, upstream, time.Second), "model").String(), "omitted model inherits the allowed session")
	upstream.Send(`{"type":"response.completed","response":{"id":"second","model":"gpt-5.4","usage":{"input_tokens":1,"output_tokens":1}}}`)
	_, err = readPassthroughLifecycleFrame(t, client, time.Second)
	require.NoError(t, err)
	write(`{"type":"session.update","session":{"model":"qwen3"}}`)
	// Consume the close handshake before waiting for the relay to finish.
	_, err = readPassthroughLifecycleFrame(t, client, 3*time.Second)
	var closeErr coderws.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, coderws.StatusPolicyViolation, closeErr.Code)
	select {
	case err = <-serverErr:
		require.ErrorIs(t, err, ErrModelScopeDenied)
	case <-time.After(3 * time.Second):
		t.Fatal("cross-scope session update was not rejected")
	}
	select {
	case payload := <-upstream.writes:
		t.Fatalf("cross-scope frame reached upstream: %s", payload)
	default:
	}
}

func TestModelScopeWSPassthroughFirstFinalMappingRefused(t *testing.T) {
	ctx, cancel := context.WithCancel(ContextWithModelScope(context.Background(), ModelScopeOpenAI))
	defer cancel()
	upstream := newStagedPassthroughConn()
	svc := newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream)
	server, serverErr := startPassthroughLifecycleServerWithHooks(t, ctx, svc, passthroughLifecycleAccount(), func(*gin.Context) *OpenAIWSIngressHooks {
		return &OpenAIWSIngressHooks{MapRequestModel: func(int, string) (string, error) { return "qwen3", nil }}
	})
	defer server.Close()
	client := dialPassthroughLifecycleClient(t, server)
	defer client.CloseNow()
	select {
	case err := <-serverErr:
		require.ErrorIs(t, err, ErrModelScopeDenied)
	case <-time.After(3 * time.Second):
		t.Fatal("first-frame cross-scope mapping was not rejected")
	}
	select {
	case payload := <-upstream.writes:
		t.Fatalf("cross-scope first frame reached upstream: %s", payload)
	default:
	}
}
