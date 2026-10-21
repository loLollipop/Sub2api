//go:build unit

package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func refusalWSClient(t *testing.T, h *OpenAIGatewayHandler, key *service.APIKey) (*coderws.Conn, <-chan struct{}) {
	t.Helper()
	done := make(chan struct{})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		defer close(done)
		c.Set(string(middleware.ContextKeyAPIKey), key)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: key.UserID, Concurrency: 1})
		c.Next()
	})
	router.GET("/openai/v1/responses", h.ResponsesWebSocket)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/openai/v1/responses", nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn, done
}

func refusalWSWrite(t *testing.T, conn *coderws.Conn, payload string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(payload)))
}

func refusalWSClosed(t *testing.T, conn *coderws.Conn, done <-chan struct{}) coderws.StatusCode {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		_, _, err := conn.Read(ctx)
		if err != nil {
			require.NotEqual(t, coderws.StatusCode(-1), coderws.CloseStatus(err), "%v", err)
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal("websocket handler did not finish")
			}
			return coderws.CloseStatus(err)
		}
	}
}

func TestGrokRefusalWSBridgeAtomicSettlementAndResume(t *testing.T) {
	for _, price := range []float64{.37, 0} {
		t.Run(fmt.Sprint(price), func(t *testing.T) {
			h, repo, up, key := newRefusalHandler(t, &price)
			conn, done := refusalWSClient(t, h, key)
			refusalWSWrite(t, conn, `{"type":"response.create","model":"grok-4","input":"refuse"}`)
			require.Equal(t, coderws.StatusPolicyViolation, refusalWSClosed(t, conn, done))
			require.Equal(t, int32(1), up.calls.Load())
			require.Equal(t, 1, repo.applyCount)
			require.Zero(t, repo.directApply, "AfterTurn must not bill an already committed refusal")
			require.Len(t, repo.jobs, 1)
			for _, job := range repo.jobs {
				require.Equal(t, int64(17), job.GroupID)
				require.Equal(t, price, job.Intent.Command.BalanceCost)
				require.Equal(t, service.MediaBillingDone, job.Status)
				require.Equal(t, service.HashUsageRequestPayload([]byte(`{"type":"response.create","model":"grok-4","input":"refuse"}`)), job.Intent.Command.RequestPayloadHash)
			}
			resumed, resumedDone := refusalWSClient(t, h, key)
			refusalWSWrite(t, resumed, `{"type":"response.create","model":"grok-4-fast","input":"refuse"}`)
			require.Equal(t, coderws.StatusPolicyViolation, refusalWSClosed(t, resumed, resumedDone))
			require.Equal(t, int32(1), up.calls.Load())
			require.Equal(t, 1, repo.applyCount)
		})
	}
}

func refusalWSSlots(h *OpenAIGatewayHandler) (*atomic.Int32, *concurrencyCacheMock) {
	admissions := &atomic.Int32{}
	cache := &concurrencyCacheMock{
		acquireUserSlotFn:    func(context.Context, int64, int, string) (bool, error) { admissions.Add(1); return true, nil },
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
	}
	h.concurrencyHelper = NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second)
	return admissions, cache
}

func TestGrokRefusalWSFirstFrameFailsBeforeAdmission(t *testing.T) {
	for _, mode := range []string{"blocked", "lookup_failure", "composite_alias_lookup_failure"} {
		t.Run(mode, func(t *testing.T) {
			h, repo, up, key := newRefusalHandler(t, nil)
			want := coderws.StatusPolicyViolation
			if mode == "blocked" {
				require.Equal(t, http.StatusForbidden, refusalHandlerRequest(h, key, "responses", "grok-4", "refuse").Code)
			} else {
				repo.lookupErr = errors.New("storage unavailable")
				want = coderws.StatusInternalError
			}
			beforeCalls := up.calls.Load()
			beforeCharges := repo.applyCount
			model := "grok-4-fast"
			if mode == "composite_alias_lookup_failure" {
				group := *key.Group
				group.Platform = service.PlatformComposite
				key.Group = &group
				key.RouteGroupIDs = nil
				model = "friendly-alias"
			}
			admissions, cache := refusalWSSlots(h)
			conn, done := refusalWSClient(t, h, key)
			refusalWSWrite(t, conn, fmt.Sprintf(`{"type":"response.create","model":%q,"input":"refuse"}`, model))
			require.Equal(t, want, refusalWSClosed(t, conn, done))
			require.Zero(t, admissions.Load())
			require.Zero(t, atomic.LoadInt32(&cache.releaseUserCalled))
			require.Equal(t, beforeCalls, up.calls.Load())
			require.Equal(t, beforeCharges, repo.applyCount)
		})
	}
}

func TestGrokRefusalWSBridgeCommitFailureStopsAndReleases(t *testing.T) {
	h, repo, up, key := newRefusalHandler(t, nil)
	repo.commitErr = errors.New("atomic commit unavailable")
	admissions, cache := refusalWSSlots(h)
	conn, done := refusalWSClient(t, h, key)
	refusalWSWrite(t, conn, `{"type":"response.create","model":"grok-4","input":"refuse"}`)
	require.Equal(t, coderws.StatusInternalError, refusalWSClosed(t, conn, done))
	require.Equal(t, int32(1), admissions.Load())
	require.Equal(t, int32(1), atomic.LoadInt32(&cache.releaseUserCalled))
	// The scheduler grants the initial account slot itself; this mock owns
	// account slots only for subsequent turns.
	require.Zero(t, atomic.LoadInt32(&cache.releaseAccountCalled))
	require.Equal(t, int32(1), up.calls.Load())
	require.Zero(t, repo.applyCount)
	require.Zero(t, repo.directApply)
	require.Empty(t, repo.jobs)
	require.Empty(t, repo.blocks)
}

const refusalWSCompleted = "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_allowed\",\"model\":\"grok-4\",\"status\":\"completed\",\"output\":[]}}\n\n"

func TestGrokRefusalWSSubsequentFrameChecksBeforeReadmission(t *testing.T) {
	for _, mode := range []string{"blocked", "lookup_failure", "new_refusal"} {
		t.Run(mode, func(t *testing.T) {
			h, repo, up, key := newRefusalHandler(t, nil)
			repo.allowOrdinaryUsage = true
			if mode == "blocked" {
				require.Equal(t, http.StatusForbidden, refusalHandlerRequest(h, key, "responses", "grok-4", "refuse").Code)
			}
			firstCall := up.calls.Load() + 1
			up.responseForCall = func(call int32) (int, string, string) {
				if call == firstCall {
					return http.StatusOK, "text/event-stream", refusalWSCompleted
				}
				return http.StatusForbidden, "application/json", `{"error":{"code":"content_policy_violation","message":"explicit content refusal"}}`
			}
			admissions, cache := refusalWSSlots(h)
			conn, done := refusalWSClient(t, h, key)
			refusalWSWrite(t, conn, `{"type":"response.create","model":"grok-4","input":"allowed"}`)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, event, err := conn.Read(ctx)
			cancel()
			require.NoError(t, err)
			require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())
			want := coderws.StatusPolicyViolation
			if mode == "lookup_failure" {
				repo.mu.Lock()
				repo.lookupErr = errors.New("database offline")
				repo.mu.Unlock()
				want = coderws.StatusInternalError
			}
			// Omitted model inherits the client-facing session model, while the
			// identity and digest come from this exact second client frame.
			second := `{"type":"response.create","input":"refuse"}`
			refusalWSWrite(t, conn, second)
			require.Equal(t, want, refusalWSClosed(t, conn, done))
			expectedAdmissions := int32(1)
			expectedCalls := firstCall
			if mode == "new_refusal" {
				expectedAdmissions++
				expectedCalls++
			}
			require.Equal(t, expectedAdmissions, admissions.Load())
			require.Equal(t, expectedAdmissions, atomic.LoadInt32(&cache.releaseUserCalled))
			require.Equal(t, expectedAdmissions-1, atomic.LoadInt32(&cache.releaseAccountCalled))
			require.Equal(t, expectedCalls, up.calls.Load())
			require.Equal(t, 1, repo.directApply, "only the initial successful turn may enter ordinary accounting")
			if mode == "new_refusal" {
				require.Len(t, repo.jobs, 1)
				for _, job := range repo.jobs {
					require.Equal(t, service.HashUsageRequestPayload([]byte(second)), job.Intent.Command.RequestPayloadHash)
					require.Equal(t, "grok-4", job.Intent.Usage.Model)
					require.Equal(t, int64(17), job.GroupID)
				}
			} else if mode == "lookup_failure" {
				require.Empty(t, repo.jobs)
			}
		})
	}
}

func TestGrokRefusalWSBridgeExcludesNonHTTP403AndGlobalIP(t *testing.T) {
	for _, tc := range []struct {
		name              string
		status            int
		contentType, body string
	}{
		{"non_403", 400, "application/json", `{"error":{"status":403,"message":"I'm sorry, I can't help with that request. Failed check: SAFETY_CHECK_TYPE"}}`},
		{"global_ip", 403, "application/json", `{"error":{"code":"content_policy_violation","message":"blocked by global ip blacklist. I'm sorry, I can't help with that request. Failed check: SAFETY_CHECK_TYPE"}}`},
		{"sse_embedded_403", 200, "text/event-stream", "data: {\"type\":\"error\",\"status\":403,\"error\":{\"message\":\"I'm sorry, I can't help with that request. Failed check: SAFETY_CHECK_TYPE\"}}\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, repo, up, key := newRefusalHandler(t, nil)
			up.status, up.contentType, up.body = tc.status, tc.contentType, tc.body
			conn, done := refusalWSClient(t, h, key)
			refusalWSWrite(t, conn, `{"type":"response.create","model":"grok-4","input":"refuse"}`)
			_ = refusalWSClosed(t, conn, done)
			require.Empty(t, repo.jobs)
			require.Empty(t, repo.blocks)
			require.Zero(t, repo.applyCount)
			require.Zero(t, repo.directApply)
		})
	}
}
