//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
)

// --- mock: records model-level rate limits as well as account-level penalties ---

type internal500ScopeRepoStub struct {
	internal500AccountRepoStub
	modelLimitCalls []modelLimitCall
}

type internal500WireUpstream struct {
	HTTPUpstream
	calls int
}

func (u *internal500WireUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.calls++
	return &http.Response{StatusCode: 500, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":500,"message":"Internal error encountered.","status":"INTERNAL"}}`))}, nil
}

func TestInternal500PenaltyUsesActualWebSearchFallbackModel(t *testing.T) {
	req := &antigravity.ClaudeRequest{Model: "client-alias", Messages: []antigravity.ClaudeMessage{{Role: "user", Content: json.RawMessage(`"search"`)}}}
	require.NoError(t, json.Unmarshal([]byte(`[{"type":"web_search_20250305","name":"web_search"}]`), &req.Tools))
	wire, err := antigravity.TransformClaudeToGeminiWithOptions(req, "offline-project", "gemini-3.1-flash-image", antigravity.TransformOptions{})
	require.NoError(t, err)
	repo := &internal500ScopeRepoStub{}
	cache := &mockInternal500Cache{incrementCount: 1}
	svc := &AntigravityGatewayService{accountRepo: repo, internal500Cache: cache}
	account := &Account{ID: 2, Platform: PlatformAntigravity, Credentials: map[string]any{"model_mapping": map[string]any{"client-alias": "gemini-3.1-flash-image"}}}
	upstream := &internal500WireUpstream{}
	result, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{ctx: context.Background(), account: account, body: wire, requestedModel: "client-alias", accessToken: "synthetic", action: "generateContent", httpUpstream: upstream})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NoError(t, result.resp.Body.Close())
	require.Equal(t, antigravityMaxRetries, upstream.calls)
	require.Empty(t, repo.modelLimitCalls)
	require.Len(t, cache.incrementCalls, 1)
	require.Len(t, repo.tempUnschedCalls, 1)
}

type modelLimitCall struct {
	accountID int64
	scope     string
	resetAt   time.Time
}

func (r *internal500ScopeRepoStub) SetModelRateLimit(_ context.Context, id int64, scope string, resetAt time.Time, _ ...string) error {
	r.modelLimitCalls = append(r.modelLimitCalls, modelLimitCall{accountID: id, scope: scope, resetAt: resetAt})
	return nil
}

// An INTERNAL 500 on an image model is the image endpoint failing, not the account: it must not
// take the account's text models (Argon, Flash) down with it.
func TestInternal500OnImageModelCoolsOnlyThatModel(t *testing.T) {
	repo := &internal500ScopeRepoStub{}
	cache := &mockInternal500Cache{incrementCount: 1}
	svc := &AntigravityGatewayService{accountRepo: repo, internal500Cache: cache}
	account := &Account{ID: 2, Name: "ag", Platform: PlatformAntigravity}

	before := time.Now()
	svc.handleInternal500ForModel(context.Background(), "[test]", account, "gemini-3.1-flash-image")

	require.Empty(t, cache.incrementCalls, "an image 500 must not count toward the account penalty")
	require.Empty(t, repo.tempUnschedCalls, "an image 500 must not make the account unschedulable")
	require.Empty(t, repo.setErrorCalls)
	require.Len(t, repo.modelLimitCalls, 1)
	call := repo.modelLimitCalls[0]
	require.Equal(t, "gemini-3.1-flash-image", call.scope, "only the image model is cooled, not antigravity:gemini")
	require.WithinDuration(t, before.Add(defaultInternal500ImageCooldown), call.resetAt, 2*time.Second)
}

func TestInternal500OnTextModelKeepsTheAccountPenalty(t *testing.T) {
	repo := &internal500ScopeRepoStub{}
	cache := &mockInternal500Cache{incrementCount: 1}
	svc := &AntigravityGatewayService{accountRepo: repo, internal500Cache: cache}
	account := &Account{ID: 2, Name: "ag", Platform: PlatformAntigravity}

	svc.handleInternal500ForModel(context.Background(), "[test]", account, "gemini-4-argon-high")

	require.Len(t, cache.incrementCalls, 1)
	require.Len(t, repo.tempUnschedCalls, 1)
	require.Empty(t, repo.modelLimitCalls)
}
