package handler

import (
	"context"
	"runtime"
	"testing"
	"time"
	"weak"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestQueuedUsageTaskReleasesParentContextBeforeExecution(t *testing.T) {
	type bodyKey struct{}
	task, bodyRef := func() (service.UsageRecordTask, weak.Pointer[byte]) {
		body := make([]byte, 1<<20)
		parent := context.WithValue(context.Background(), bodyKey{}, body)
		parent = context.WithValue(parent, ctxkey.RequestID, "request-before-reuse")
		wrapped, _ := wrapUsageRecordTaskContext(parent, func(ctx context.Context) {
			require.Nil(t, ctx.Value(bodyKey{}))
			require.Equal(t, "request-before-reuse", ctx.Value(ctxkey.RequestID))
			require.ErrorIs(t, ctx.Err(), context.Canceled)
		})
		return wrapped, weak.Make(&body[0])
	}()
	require.Eventually(t, func() bool {
		runtime.GC()
		return bodyRef.Value() == nil
	}, 3*time.Second, 5*time.Millisecond, "queued billing task still holds the request body")
	worker, cancel := context.WithCancel(context.Background())
	cancel()
	task(worker)
}

func TestSubmitUsageRecordTaskCopiesRequestContext(t *testing.T) {
	parent := context.WithValue(context.Background(), ctxkey.ClientRequestID, "client-request-123")
	parent = context.WithValue(parent, ctxkey.RequestID, "request-456")

	var gotClientRequestID string
	var gotRequestID string
	h := &GatewayHandler{}
	h.submitUsageRecordTask(parent, func(ctx context.Context) {
		gotClientRequestID, _ = ctx.Value(ctxkey.ClientRequestID).(string)
		gotRequestID, _ = ctx.Value(ctxkey.RequestID).(string)
	})

	require.Equal(t, "client-request-123", gotClientRequestID)
	require.Equal(t, "request-456", gotRequestID)
}

func TestOpenAISubmitUsageRecordTaskCopiesRequestContext(t *testing.T) {
	parent := context.WithValue(context.Background(), ctxkey.ClientRequestID, "openai-client-request-123")
	parent = context.WithValue(parent, ctxkey.RequestID, "openai-request-456")

	var gotClientRequestID string
	var gotRequestID string
	h := &OpenAIGatewayHandler{}
	h.submitUsageRecordTask(parent, func(ctx context.Context) {
		gotClientRequestID, _ = ctx.Value(ctxkey.ClientRequestID).(string)
		gotRequestID, _ = ctx.Value(ctxkey.RequestID).(string)
	})

	require.Equal(t, "openai-client-request-123", gotClientRequestID)
	require.Equal(t, "openai-request-456", gotRequestID)
}
