//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelScopeExecutionErrorKeepsClientProtocol(t *testing.T) {
	gin.SetMode(gin.TestMode)
	openai := &OpenAIGatewayHandler{}
	gateway := &GatewayHandler{}
	for _, tc := range []struct {
		name, path, streamPrefix string
		handle                   func(*gin.Context, error, bool) bool
	}{
		{"responses", "/v1/responses", "event: response.failed\n", openai.handleModelScopeError},
		{"chat", "/v1/chat/completions", "event: error\n", openai.handleModelScopeError},
		{"openai messages", "/v1/messages", "event: error\n", openai.handleModelScopeError},
		{"messages", "/v1/messages", "data: ", gateway.handleModelScopeError},
		{"gateway chat", "/v1/chat/completions", "data: ", gateway.handleModelScopeError},
		{"gateway responses", "/v1/responses", "event: response.failed\n", gateway.handleModelScopeError},
		{"gemini", "/v1beta/models/gemini:streamGenerateContent", "data: ", handleGeminiModelScopeError},
	} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", tc.name, streaming), func(t *testing.T) {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodPost, tc.path, nil)
				if streaming {
					c.Writer.Header().Set("Content-Type", "text/event-stream")
					c.Writer.WriteHeader(http.StatusOK)
					c.Writer.Flush()
				}
				require.True(t, tc.handle(c, fmt.Errorf("wrapped: %w", service.ErrModelScopeDenied), streaming))
				if streaming {
					require.True(t, strings.HasPrefix(w.Body.String(), tc.streamPrefix), w.Body.String())
					require.Equal(t, http.StatusOK, w.Code)
				} else {
					require.Equal(t, http.StatusForbidden, w.Code)
				}
				require.Contains(t, w.Body.String(), service.ErrModelScopeDenied.Message)
				require.NotContains(t, w.Body.String(), "metadata=")
				require.NotContains(t, w.Body.String(), "Upstream request failed")
			})
		}
	}
}

func TestModelScopeExecutionErrorDoesNotConsumeUpstreamFailure(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	require.False(t, (&OpenAIGatewayHandler{}).handleModelScopeError(c, errors.New("upstream failed"), false))
	require.False(t, c.Writer.Written())
	require.False(t, shouldReportOpenAIWSProxyAccountFailure(fmt.Errorf("wrapped: %w", service.ErrModelScopeDenied)))
	require.True(t, shouldReportOpenAIWSProxyAccountFailure(errors.New("upstream websocket failed")))
}

func TestModelScopeImagesJSONKeepaliveErrorPreservesDocument(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, committed := range []bool{false, true} {
		t.Run(fmt.Sprintf("committed=%v", committed), func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
			interval := time.Hour
			if committed {
				interval = 5 * time.Millisecond
			}
			stop := service.StartOpenAIImagesJSONKeepalive(c, interval)
			defer stop()
			if committed {
				require.Eventually(t, c.Writer.Written, time.Second, time.Millisecond)
			}
			require.True(t, (&OpenAIGatewayHandler{}).handleModelScopeError(c, service.ErrModelScopeDenied, false))
			stop()
			status := http.StatusForbidden
			if committed {
				status = http.StatusOK // A heartbeat has already committed the HTTP status.
			}
			require.Equal(t, status, w.Code)
			require.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
			require.True(t, json.Valid(w.Body.Bytes()), w.Body.String())
			require.NotContains(t, w.Body.String(), "event:")
			require.NotContains(t, w.Body.String(), "data:")
			decoder := json.NewDecoder(strings.NewReader(w.Body.String()))
			var payload struct {
				Error struct {
					Type, Code, Message string
				}
			}
			require.NoError(t, decoder.Decode(&payload))
			require.ErrorIs(t, decoder.Decode(&payload), io.EOF)
			require.Equal(t, "permission_error", payload.Error.Type)
			require.Equal(t, "MODEL_SCOPE_DENIED", payload.Error.Code)
			require.Equal(t, service.ErrModelScopeDenied.Message, payload.Error.Message)
		})
	}
}

func TestModelScopeImagesJSONKeepalivePreservesCompletedDocument(t *testing.T) {
	for _, name := range []string{"document", "semantic commit"} {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
			stop := service.StartOpenAIImagesJSONKeepalive(c, time.Hour)
			defer stop()
			if name == "document" {
				c.JSON(http.StatusOK, gin.H{"data": []gin.H{{"b64_json": "aW1hZ2U="}}})
			} else {
				service.MarkResponseCommitted(c)
				require.Equal(t, -1, service.OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c))
			}
			completed := w.Body.String()
			require.True(t, (&OpenAIGatewayHandler{}).handleModelScopeError(c, service.ErrModelScopeDenied, false))
			require.Equal(t, completed, w.Body.String(), "cannot append after meaningful output or semantic commit")
		})
	}
}

func TestModelScopeLiveErrorReturnsPermission(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/live", nil)
	(&OpenAIGatewayHandler{}).writeLiveCreateError(c, service.ErrModelScopeDenied)
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "MODEL_SCOPE_DENIED")
}

func TestModelScopeSelectionChannelDenialKeepsProtocol(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"responses", "chat/completions", "messages"} {
		t.Run(endpoint, func(t *testing.T) {
			group := &service.Group{ID: 9, Platform: service.PlatformMixed, Status: service.StatusActive, Hydrated: true, RateMultiplier: 1}
			channel := service.NewChannelService(&openAIWSUsageHandlerChannelRepoStub{
				channels:       []service.Channel{{ID: 1, Status: service.StatusActive, GroupIDs: []int64{group.ID}, ModelMapping: map[string]map[string]string{service.PlatformMixed: {"gpt-5.1": "qwen3"}}}},
				groupPlatforms: map[int64]string{group.ID: service.PlatformMixed},
			}, nil, nil, nil, nil)
			mapped := channel.ResolveChannelMapping(context.Background(), group.ID, "gpt-5.1")
			require.Equal(t, "qwen3", mapped.MappedModel)
			h, upstream, key := newMixedSafetyHandler(t, group, group, service.PlatformMixed, channel)
			key.ModelScope = service.ModelScopeOpenAI
			key.RouteGroupIDs = []int64{group.ID}
			c, rec := mixedHandlerContext(key, endpoint, `{"model":"gpt-5.1","input":"hello","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`)
			switch endpoint {
			case "responses":
				h.Responses(c)
			case "chat/completions":
				h.ChatCompletions(c)
			case "messages":
				h.Messages(c)
			}
			require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), "permission_error")
			require.NotContains(t, rec.Body.String(), "Service temporarily unavailable")
			if endpoint == "messages" {
				require.Contains(t, rec.Body.String(), `"type":"error"`)
			} else {
				require.Contains(t, rec.Body.String(), "MODEL_SCOPE_DENIED")
			}
			require.Zero(t, upstream.calls, "scope denial cannot reach the upstream")
		})
	}
}

func TestModelScopeWSLaterCandidatePreservesLocalCause(t *testing.T) {
	key := &service.APIKey{ModelScope: service.ModelScopeCN}
	err := apiKeyModelCandidateCloseError(key, []string{"qwen3", "gpt-5"})
	require.ErrorIs(t, err, service.ErrModelScopeDenied)
	require.False(t, shouldReportOpenAIWSProxyAccountFailure(err), "local later-turn refusal cannot change account scoring")
	key.ModelScope = service.ModelScopeAll
	key.Group = &service.Group{ModelAllowlist: service.GroupModelAllowlist{Enabled: true, Models: []string{"qwen3"}}}
	err = apiKeyModelCandidateCloseError(key, []string{"gpt-5"})
	require.Error(t, err)
	require.NotErrorIs(t, err, service.ErrModelScopeDenied, "retain the separate group-allowlist policy")
}
