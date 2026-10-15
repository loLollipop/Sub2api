//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGeminiQuotaEndpointIdentity(t *testing.T) {
	for _, raw := range []string{"", "https://generativelanguage.googleapis.com", "https://GENERATIVELANGUAGE.GOOGLEAPIS.COM/"} {
		require.True(t, isOfficialGeminiAPIBaseURL(raw), raw)
		a := &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": raw}}
		require.Equal(t, GeminiTierAIStudioFree, geminiQuotaTierKeyForAccount(a))
	}
	for _, raw := range []string{"http://generativelanguage.googleapis.com", "https://generativelanguage.googleapis.com:443", "https://key@generativelanguage.googleapis.com", "https://generativelanguage.googleapis.com/v1beta", "https://generativelanguage.googleapis.com?", "https://generativelanguage.googleapis.com#secret", "https://generativelanguage.googleapis.com.evil.test", "https://relay.test"} {
		a := &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": raw, "tier_id": GeminiTierAIStudioFree}}
		require.False(t, isOfficialGeminiAPIBaseURL(raw), raw)
		require.True(t, a.IsGeminiThirdPartyAPIKey(), raw)
		require.Empty(t, geminiQuotaTierKeyForAccount(a), raw)
	}
	for _, a := range []*Account{vertexModelSyncTestAccount(nil), geminiVertexAPIKeyTestAccount("")} {
		require.Empty(t, geminiQuotaTierKeyForAccount(a))
	}
}

type geminiProviderLimitRepo struct {
	AccountRepository
	until time.Time
}

func (r *geminiProviderLimitRepo) SetRateLimited(_ context.Context, _ int64, until time.Time) error {
	r.until = until
	return nil
}

func TestGeminiProvider429NeverInfersDailyQuota(t *testing.T) {
	for _, account := range []*Account{
		{ID: 1, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://relay.test"}},
		vertexModelSyncTestAccount(nil), geminiVertexAPIKeyTestAccount(""),
	} {
		for _, tc := range []struct {
			name, body string
			headers    http.Header
			seconds    int
		}{
			{"daily text", `{"error":{"message":"requests per day quota exceeded"}}`, nil, defaultRateLimit429CooldownSeconds},
			{"Vertex retry", `{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"17.2s"}]}}`, nil, 18},
			{"header", `{"error":{"message":"quota per day"}}`, http.Header{"Retry-After": {"12"}}, 12},
			{"negative delay", `{"error":{"details":[{"retryDelay":"-10s"}]}}`, nil, defaultRateLimit429CooldownSeconds},
		} {
			t.Run(tc.name+account.Type, func(t *testing.T) {
				repo := &geminiProviderLimitRepo{}
				svc := &GeminiMessagesCompatService{accountRepo: repo}
				now := time.Now()
				svc.handleGeminiUpstreamError(context.Background(), account, 429, tc.headers, []byte(tc.body))
				delay := time.Duration(tc.seconds) * time.Second
				if account.IsVertexServiceAccount() && tc.seconds == defaultRateLimit429CooldownSeconds {
					delay = geminiVertexFallbackCooldown
				}
				require.WithinDuration(t, now.Add(delay), repo.until, 2*time.Second)
			})
		}
	}
}

type failingGeminiModelReader struct{}

func (failingGeminiModelReader) Read([]byte) (int, error) { return 0, errors.New("broken model body") }
func (failingGeminiModelReader) Close() error             { return nil }

func TestForwardListModelsRejectsTruncatedRead(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{}, Body: failingGeminiModelReader{}}}
	svc := newGeminiModelsEndpointsTestService(nil, upstream)
	_, err := svc.ForwardListModels(context.Background(), &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test"}})
	require.Error(t, err)
	require.ErrorContains(t, err, "broken model body")
}

func TestProjectGeminiCatalogUsesRealMappedTargetsOnly(t *testing.T) {
	a := &Account{Credentials: map[string]any{"model_mapping": map[string]any{"alias": "gemini-live", "invented": "gemini-missing"}}}
	res, err := projectGeminiAccountCatalog(context.Background(), &UpstreamHTTPResult{StatusCode: 200, Body: []byte(`{"models":[{"name":"models/gemini-live","limits":{"input":123}},{"name":"models/gemini-other"}],"nextPageToken":"next"}`)}, a)
	require.NoError(t, err)
	require.JSONEq(t, `{"models":[{"name":"models/alias","limits":{"input":123}}],"nextPageToken":"next"}`, string(res.Body))
	_, err = listVertexGeminiPublisherModels("global", func(int, string) ([]byte, error) {
		return []byte(`{"publisherModels":[{"name":"publishers/google/models/gemini-evil/../x"}]}`), nil
	})
	require.Error(t, err)
}

func TestProjectGeminiCatalogChecksSelectedAccountsMappedScope(t *testing.T) {
	ctx := ContextWithModelScope(context.Background(), ModelScopeOther)
	a := &Account{Platform: PlatformGemini, Credentials: map[string]any{"model_mapping": map[string]any{
		"gemini-public": "gpt-5.4", "gemini-valid": "gemini-live",
	}}}
	res, err := projectGeminiAccountCatalog(ctx, &UpstreamHTTPResult{StatusCode: 200, Body: []byte(`{"models":[{"name":"models/gpt-5.4"},{"name":"models/gemini-live"}]}`)}, a)
	require.NoError(t, err)
	require.JSONEq(t, `{"models":[{"name":"models/gemini-valid"}]}`, string(res.Body))
	// No account mapping is not permission to expose cross-family entries.
	res, err = projectGeminiAccountCatalog(ctx, &UpstreamHTTPResult{StatusCode: 200, Body: []byte(`{"models":[{"name":"models/gpt-5.4"},{"name":"models/gemini-live"}]}`)}, &Account{Platform: PlatformGemini})
	require.NoError(t, err)
	require.JSONEq(t, `{"models":[{"name":"models/gemini-live"}]}`, string(res.Body))
	upstream := &httpUpstreamRecorder{}
	svc := newGeminiModelsEndpointsTestService([]Account{*a}, upstream)
	_, err = svc.SelectAccountForModelsEndpoints(ctx, nil, "gemini-public")
	require.Error(t, err)
	_, err = svc.ForwardGetModel(ctx, a, "gemini-public")
	require.ErrorIs(t, err, ErrModelScopeDenied)
}
