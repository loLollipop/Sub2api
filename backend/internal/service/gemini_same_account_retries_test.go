//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeminiMaxAttemptsPerAccount(t *testing.T) {
	var nilService *GeminiMessagesCompatService
	require.Equal(t, 5, nilService.geminiMaxAttemptsPerAccount())
	require.Equal(t, 5, (&GeminiMessagesCompatService{}).geminiMaxAttemptsPerAccount())
	for _, retries := range []int{0, 1, 4, 10} {
		svc := &GeminiMessagesCompatService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxSameAccountRetriesGemini: retries}}}
		require.Equal(t, retries+1, svc.geminiMaxAttemptsPerAccount())
	}
}

type geminiRetryTransportStub struct {
	HTTPUpstream
	calls          int
	signatureFirst bool
	response       geminiSequenceUpstreamStub
}

func (u *geminiRetryTransportStub) Do(req *http.Request, proxy string, accountID int64, concurrency int) (*http.Response, error) {
	u.calls++
	if u.signatureFirst && u.calls == 1 {
		return u.response.Do(req, proxy, accountID, concurrency)
	}
	return nil, errors.New("synthetic upstream connection failure")
}

func TestGeminiForward_TransportRetriesFollowConfig(t *testing.T) {
	for _, retries := range []int{0, 1} {
		for _, fw := range geminiSingleCheckForwarders {
			t.Run(fmt.Sprintf("%s/retries=%d", fw.name, retries), func(t *testing.T) {
				upstream := &geminiRetryTransportStub{}
				svc := &GeminiMessagesCompatService{httpUpstream: upstream, cfg: &config.Config{Gateway: config.GatewayConfig{MaxSameAccountRetriesGemini: retries}}}
				err, c := fw.forward(t, svc, geminiSameAccountRetryTestAccount())
				require.Error(t, err)
				require.Equal(t, http.StatusBadGateway, c.Writer.Status())
				require.Equal(t, retries+1, upstream.calls)
			})
		}
	}
}

func TestGeminiClaudeCompat_SignatureDowngradeDoesNotConsumeTransportRetries(t *testing.T) {
	for _, retries := range []int{0, 1} {
		t.Run(fmt.Sprintf("retries=%d", retries), func(t *testing.T) {
			upstream := &geminiRetryTransportStub{signatureFirst: true, response: geminiSequenceUpstreamStub{responses: []geminiSequenceUpstreamResponse{{status: http.StatusBadRequest, body: geminiRetryTestSignatureBody}}}}
			svc := &GeminiMessagesCompatService{httpUpstream: upstream, cfg: &config.Config{Gateway: config.GatewayConfig{MaxSameAccountRetriesGemini: retries}}}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			body := []byte(`{"model":"gemini-2.5-flash","max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
			result, err := svc.Forward(context.Background(), c, geminiSameAccountRetryTestAccount(), body)
			require.Error(t, err)
			require.Nil(t, result)
			require.Equal(t, http.StatusBadGateway, c.Writer.Status())
			require.Equal(t, retries+2, upstream.calls)
		})
	}
}

const (
	geminiRetryTest503Body       = `{"error":{"code":503,"message":"The model is overloaded.","status":"UNAVAILABLE"}}`
	geminiRetryTestSignatureBody = `{"error":{"code":400,"message":"Function call is missing a thought_signature in functionCall parts.","status":"INVALID_ARGUMENT"}}`
	geminiRetryTestOKBody        = `{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":1,"totalTokenCount":4}}`
)

func geminiSameAccountRetryTestAccount() *Account {
	return &Account{
		ID:          840,
		Platform:    PlatformGemini,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key"},
	}
}

func TestGeminiForward_SameAccountRetriesFollowConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, retries := range []int{0, 1} {
		for _, fw := range geminiSingleCheckForwarders {
			t.Run(fmt.Sprintf("%s/retries=%d", fw.name, retries), func(t *testing.T) {
				svc, upstream, _ := newGeminiSingleCheckService(geminiSequenceUpstreamResponse{status: http.StatusServiceUnavailable, body: geminiRetryTest503Body})
				svc.cfg.Gateway.MaxSameAccountRetriesGemini = retries

				err, _ := fw.forward(t, svc, geminiSameAccountRetryTestAccount())

				var failoverErr *UpstreamFailoverError
				require.True(t, errors.As(err, &failoverErr), "同号重试耗尽应换号，got %T: %v", err, err)
				require.Equal(t, http.StatusServiceUnavailable, failoverErr.StatusCode)
				require.Equal(t, retries+1, upstream.calls)
			})
		}
	}
}

func TestGeminiClaudeCompat_SignatureDowngradeDoesNotConsumeSameAccountRetries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	forward := func(svc *GeminiMessagesCompatService) (*ForwardResult, error) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		body := []byte(`{"model":"gemini-2.5-flash","max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
		return svc.Forward(context.Background(), c, geminiSameAccountRetryTestAccount(), body)
	}

	t.Run("both downgrade stages run without same-account retries", func(t *testing.T) {
		svc, upstream, _ := newGeminiSingleCheckService(
			geminiSequenceUpstreamResponse{status: http.StatusBadRequest, body: geminiRetryTestSignatureBody},
			geminiSequenceUpstreamResponse{status: http.StatusBadRequest, body: geminiRetryTestSignatureBody},
			geminiSequenceUpstreamResponse{status: http.StatusOK, body: geminiRetryTestOKBody},
		)
		svc.cfg.Gateway.MaxSameAccountRetriesGemini = 0

		result, err := forward(svc)

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, 3, upstream.calls)
	})

	t.Run("same-account retries are counted apart from downgrades", func(t *testing.T) {
		svc, upstream, _ := newGeminiSingleCheckService(
			geminiSequenceUpstreamResponse{status: http.StatusServiceUnavailable, body: geminiRetryTest503Body},
			geminiSequenceUpstreamResponse{status: http.StatusBadRequest, body: geminiRetryTestSignatureBody},
			geminiSequenceUpstreamResponse{status: http.StatusBadRequest, body: geminiRetryTestSignatureBody},
			geminiSequenceUpstreamResponse{status: http.StatusServiceUnavailable, body: geminiRetryTest503Body},
		)
		svc.cfg.Gateway.MaxSameAccountRetriesGemini = 1

		_, err := forward(svc)

		var failoverErr *UpstreamFailoverError
		require.True(t, errors.As(err, &failoverErr), "同号重试耗尽应换号，got %T: %v", err, err)
		require.Equal(t, http.StatusServiceUnavailable, failoverErr.StatusCode)
		require.Equal(t, 4, upstream.calls)
	})
}
