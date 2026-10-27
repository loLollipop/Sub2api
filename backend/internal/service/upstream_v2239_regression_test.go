//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestV2239Regression(t *testing.T) {
	t.Run("private response headers", func(t *testing.T) {
		header := http.Header{"Location": {"https://private-provider.example/login"}, "X-Request-Id": {"https://private-provider.example"}}
		filtered := responseheaders.FilterHeaders(header, nil)
		require.Empty(t, filtered.Get("Location"))
		require.Empty(t, filtered.Get("X-Request-Id"))
	})
	t.Run("terminal error remains failure", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"https://private-provider.example failed\"}}\n\n"))}
		_, err := (&GatewayService{}).handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "test")
		require.Error(t, err)
		require.NotContains(t, rec.Body.String(), "private-provider")
	})
	t.Run("brief direct DeepSeek refusal", func(t *testing.T) {
		repo := &openaiTransportAccountRepoStub{}
		svc := &OpenAIGatewayService{accountRepo: repo}
		c, _ := newOpenAITransportErrTestContext()
		before := time.Now()
		_ = svc.handleOpenAIUpstreamTransportError(context.Background(), c, &Account{ID: 1, Platform: PlatformDeepseek, Type: AccountTypeAPIKey}, syscall.ECONNREFUSED, false)
		require.Len(t, repo.tempUnschedCalls, 1)
		require.WithinDuration(t, before.Add(5*time.Second), repo.tempUnschedCalls[0].until, time.Second)
	})
}
