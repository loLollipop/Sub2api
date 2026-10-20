package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestV226QueuedMessagesGate(t *testing.T) {
	for _, slotType := range []string{"user", "account", "message_queue", "rpm"} {
		t.Run(slotType, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			stop := service.StartOpenAIStreamSSEKeepaliveUntilUpstream(c, time.Millisecond)
			defer stop()
			started := false
			switch slotType {
			case "rpm":
				queue := service.NewUserMessageQueueService(nil, nil, &config.UserMessageQueueConfig{MinDelayMs: 50, MaxDelayMs: 50})
				h := NewUserMsgQueueHelper(queue, SSEPingFormatClaude, time.Millisecond)
				err := h.ThrottleWithPing(c, 1, 1, true, &started, 15*time.Millisecond, zap.NewNop())
				require.Error(t, err)
			case "message_queue":
				ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Millisecond)
				defer cancel()
				h := &UserMsgQueueHelper{pingFormat: SSEPingFormatClaude, pingInterval: time.Millisecond}
				_, err := h.waitForLockWithPing(c, ctx, 1, 1, true, &started, zap.NewNop())
				require.Error(t, err)
			default:
				h := NewConcurrencyHelper(nil, SSEPingFormatClaude, time.Millisecond)
				_, err := h.waitForSlotWithPingTimeout(c, slotType, 1, 1, 15*time.Millisecond, true, &started, false)
				require.Error(t, err)
			}
			require.False(t, started)
			require.False(t, c.Writer.Written())
		})
	}
}

func TestV226MessagesRetryRegatesKeepalive(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
		stop := service.StartOpenAIStreamSSEKeepaliveUntilUpstream(c, time.Second)
		defer stop()
		before := service.OpenAICompactKeepaliveAdjustedWrittenSize(c)
		// The first attempt is SSE but fails before committing any output.
		service.ArmOpenAIStreamSSEKeepalive(c)
		require.True(t, openAIForwardMayFailover(c, before, &service.UpstreamFailoverError{}))
		// Account selection/backoff for the second attempt must be gated again.
		time.Sleep(2 * time.Second)
		n, err, handled := service.WriteOpenAIStreamSSEHeartbeat(c, []byte(": queued\n\n"))
		require.NoError(t, err)
		require.True(t, handled)
		require.Zero(t, n)
		require.False(t, c.Writer.Written())
		// Native stream adapters may stage headers before returning a retryable
		// SSE error; that suspends the timer but must not bypass the retry gate.
		service.ArmOpenAIStreamSSEKeepalive(c)
		c.Header("Content-Type", "text/event-stream")
		require.True(t, openAIForwardMayFailover(c, before, &service.UpstreamFailoverError{}))
		n, err, handled = service.WriteOpenAIStreamSSEHeartbeat(c, []byte(": queued\n\n"))
		require.NoError(t, err)
		require.True(t, handled)
		require.Zero(t, n)
		require.False(t, c.Writer.Written())
		c.JSON(http.StatusBadRequest, gin.H{"error": "second attempt rejected"})
		require.Equal(t, http.StatusBadRequest, c.Writer.Status())
	})
}

func TestV226FailoverAfterOrdinaryCommitAndCompactException(t *testing.T) {
	for _, compact := range []bool{false, true} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		var stop func()
		if compact {
			service.MarkOpenAICompactClientStream(c)
			stop = service.StartOpenAICompactSSEKeepalive(c, time.Hour)
		} else {
			stop = service.StartOpenAIStreamSSEKeepaliveUntilUpstream(c, time.Hour)
			service.ArmOpenAIStreamSSEKeepalive(c)
		}
		before := service.OpenAICompactKeepaliveAdjustedWrittenSize(c)
		require.True(t, openAIForwardMayFailover(c, before, &service.UpstreamFailoverError{}))
		// A fresh successful SSE attempt reopens the gate after retry admission.
		service.ArmOpenAIStreamSSEKeepalive(c)
		n, err, handled := service.WriteOpenAIStreamSSEHeartbeat(c, []byte(": ping\n\n"))
		require.NoError(t, err)
		require.True(t, handled)
		recordGatewayStreamHeartbeat(c, n)
		require.Equal(t, compact, openAIForwardMayFailover(c, before, &service.UpstreamFailoverError{SafeToFailoverAfterWrite: true}))
		stop()
	}
}
