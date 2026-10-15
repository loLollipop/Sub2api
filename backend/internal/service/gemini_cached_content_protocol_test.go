//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type geminiCachedContentProtocolHTTP struct {
	calls int
	do    func(*http.Request, string, int64, int) (*http.Response, error)
}

func (h *geminiCachedContentProtocolHTTP) Do(r *http.Request, proxy string, id int64, concurrency int) (*http.Response, error) {
	h.calls++
	return h.do(r, proxy, id, concurrency)
}

func (h *geminiCachedContentProtocolHTTP) DoWithTLS(r *http.Request, proxy string, id int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return h.Do(r, proxy, id, concurrency)
}

func geminiCachedContentProtocolAccount() *Account {
	return &Account{ID: 71, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 3,
		Credentials: map[string]any{"api_key": "cache-test-key"}}
}

func geminiCachedContentProtocolVertex() *Account {
	return &Account{ID: 72, Platform: PlatformGemini, Type: AccountTypeServiceAccount, Credentials: map[string]any{
		"project_id": "cache-project", "location": "us-central1", "base_url": "",
		"service_account_json": `{"project_id":"cache-project","client_email":"test@example.invalid","private_key":"unused-with-cached-token"}`,
	}}
}

func freezeGeminiCachedContentProtocolBinding(t *testing.T, record *GeminiCachedContent, account *Account) {
	t.Helper()
	identity, err := GeminiCachedContentEndpointIdentity(account, record.UpstreamModelID())
	require.NoError(t, err)
	record.UpstreamEndpointIdentity = identity
}

func geminiCachedContentProtocolService(h *geminiCachedContentProtocolHTTP, now time.Time) *GeminiCachedContentService {
	cfg := &config.Config{}
	s := NewGeminiCachedContentService(nil, &GeminiMessagesCompatService{cfg: cfg, httpUpstream: h,
		tokenProvider: &GeminiTokenProvider{tokenCache: &fakeGeminiTokenCache{token: "cache-access-token"}}}, 2*time.Hour)
	s.now = func() time.Time { return now }
	return s
}

func geminiCachedContentProtocolResponse(name, model string, expiry time.Time) string {
	b, _ := json.Marshal(map[string]any{"name": name, "model": model, "expireTime": expiry.Format(time.RFC3339Nano),
		"usageMetadata": map[string]any{"totalTokenCount": 2048}})
	return string(b)
}

func TestGeminiCachedContentProtocolRequestValidation(t *testing.T) {
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	for _, body := range []string{
		`null`, `[]`, `{} {}`, `{"model":"gemini-a","model":"gemini-b"}`,
		`{"model":"gemini-a","contents":[{"parts":[{"text":"a","text":"b"}]}]}`,
		`{"model":null}`, `{"model":12}`, `{"model":"models/../x"}`, `{"model":"models/models/gemini-a"}`,
		`{"model":"gemini-a","ttl":null}`, `{"model":"gemini-a","expireTime":null}`,
		`{"model":"gemini-a","ttl":"1s","expire_time":"2027-01-01T00:00:00Z"}`,
		`{"model":"gemini-a","expireTime":"2026-10-15T01:00:00Z","expire_time":"2026-10-15T01:00:00Z"}`,
		`{"model":"gemini-a","displayName":null}`, `{"model":"gemini-a","ttl":"7200.1s"}`,
		`{"model":"gemini-a","expireTime":"9999-01-01T00:00:00Z"}`,
	} {
		t.Run(body, func(t *testing.T) {
			_, err := ParseGeminiCachedContentCreateRequest([]byte(body), now, 2*time.Hour)
			require.Error(t, err)
		})
	}
	req, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"models/gemini-a","ttl":"1.000000001s","display_name":"test","contents":[{"parts":[{"text":"hello"}]}],"custom":9007199254740993}`), now, 2*time.Hour)
	require.NoError(t, err)
	require.Equal(t, 2*time.Second, req.TTL)
	require.Equal(t, "gemini-a", req.Model)
	require.Equal(t, json.Number("9007199254740993"), req.payload["custom"])
	_, err = ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-a"}`), now, 500*time.Millisecond)
	require.Error(t, err)
	for _, ttl := range []string{"NaNs", "Infs", "1e3s", "-1s", "+1s", ".1s", "0s", "1.0000000001s", "9223372037s", "999999999999999999999s", "1ms"} {
		_, err := parseGoogleDurationSeconds(ttl)
		require.Error(t, err, ttl)
	}
	max, err := parseGoogleDurationSeconds("9223372036s")
	require.NoError(t, err)
	require.Positive(t, max)
	_, err = parseGoogleDurationSeconds("9223372036.000000001s")
	require.Error(t, err)
	_, err = ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-a"}`), now, -time.Second)
	require.Error(t, err)
}

func TestGeminiCachedContentProtocolPatchValidation(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct{ body, mask string }{
		{`{"ttl":"1s","model":"x"}`, "ttl"}, {`{"ttl":"1s","display_name":"x"}`, "ttl"},
		{`{"ttl":null}`, "ttl"}, {`{"ttl":"1s","ttl":"2s"}`, "ttl"}, {`{}`, "ttl"},
		{`{"ttl":"1s"}`, "*"}, {`{"ttl":"1s"}`, "ttl,,ttl"}, {`{"ttl":"1s"}`, "expireTime"},
		{`{"ttl":"1s","name":12}`, "ttl"}, {`{"ttl":"1s","name":"https://evil.invalid/cache"}`, "ttl"},
	} {
		_, err := ParseGeminiCachedContentPatchRequest([]byte(tc.body), tc.mask, now, time.Hour)
		require.Error(t, err, "%s %s", tc.body, tc.mask)
	}
	patch, err := ParseGeminiCachedContentPatchRequest([]byte(`{"ttl":"1.1s","name":"cachedContents/abc"}`), "ttl", now, time.Hour)
	require.NoError(t, err)
	require.Equal(t, 2*time.Second, patch.TTL)
	require.Equal(t, "cachedContents/abc", patch.Name)
	patch, err = ParseGeminiCachedContentPatchRequest([]byte(fmt.Sprintf(`{"expire_time":%q}`, now.Add(5*time.Second).Format(time.RFC3339Nano))), "expire_time", now, time.Hour)
	require.NoError(t, err)
	require.Equal(t, 5*time.Second, patch.TTL)
	require.True(t, now.Add(5*time.Second).Equal(patch.ExpireTime))
}

func TestGeminiCachedContentProtocolAbsoluteExpirationSurvivesDispatchDelay(t *testing.T) {
	parsedAt := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	expiresAt := parsedAt.Add(time.Minute + 100*time.Millisecond)
	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		for _, delay := range []time.Duration{20 * time.Second, 2 * time.Minute} {
			t.Run(fmt.Sprintf("%s/%s", method, delay), func(t *testing.T) {
				body := []byte(fmt.Sprintf(`{"expireTime":%q}`, expiresAt.Format(time.RFC3339Nano)))
				create, err := ParseGeminiCachedContentCreateRequest(append([]byte(`{"model":"gemini-a",`), body[1:]...), parsedAt, time.Hour)
				require.NoError(t, err)
				patch, err := ParseGeminiCachedContentPatchRequest(body, "expireTime", parsedAt, time.Hour)
				require.NoError(t, err)
				h := &geminiCachedContentProtocolHTTP{do: func(r *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
					payload, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					require.NotContains(t, string(payload), `"ttl"`)
					require.Contains(t, string(payload), expiresAt.Format(time.RFC3339Nano))
					if method == http.MethodPatch {
						require.Equal(t, "updateMask=expireTime", r.URL.RawQuery)
					}
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(geminiCachedContentProtocolResponse("cachedContents/created", "models/gemini-a", expiresAt)))}, nil
				}}
				s := geminiCachedContentProtocolService(h, parsedAt.Add(delay))
				a := geminiCachedContentProtocolAccount()
				var result *GeminiCachedContentUpstream
				if method == http.MethodPost {
					result, err = s.CreateUpstream(context.Background(), a, create, "gemini-a", "abc", create.TTL)
				} else {
					record := &GeminiCachedContent{PublicID: "abc", AccountID: a.ID, UpstreamName: "cachedContents/created", UpstreamModel: "models/gemini-a"}
					freezeGeminiCachedContentProtocolBinding(t, record, a)
					result, err = s.PatchUpstream(context.Background(), a, record, patch)
				}
				if delay > time.Minute {
					require.ErrorIs(t, err, ErrGeminiCachedContentRequestNotSent)
					require.Zero(t, h.calls)
				} else {
					require.NoError(t, err)
					require.True(t, result.ExpireTime.Equal(expiresAt))
					require.Equal(t, 1, h.calls)
				}
			})
		}
	}
}

func TestGeminiCachedContentProtocolAbsoluteExpirationRecheckedAfterAuth(t *testing.T) {
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		h := &geminiCachedContentProtocolHTTP{do: func(*http.Request, string, int64, int) (*http.Response, error) {
			t.Fatal("expiration passed during request preparation")
			return nil, nil
		}}
		s := geminiCachedContentProtocolService(h, now)
		clockReads := 0
		s.now = func() time.Time {
			clockReads++
			if clockReads == 1 {
				return now
			}
			return now.Add(2 * time.Minute)
		}
		a := geminiCachedContentProtocolAccount()
		body := []byte(fmt.Sprintf(`{"expireTime":%q}`, now.Add(time.Minute).Format(time.RFC3339Nano)))
		var err error
		if method == http.MethodPost {
			req, parseErr := ParseGeminiCachedContentCreateRequest(append([]byte(`{"model":"gemini-a",`), body[1:]...), now, time.Hour)
			require.NoError(t, parseErr)
			_, err = s.CreateUpstream(context.Background(), a, req, "gemini-a", "abc", req.TTL)
		} else {
			req, parseErr := ParseGeminiCachedContentPatchRequest(body, "expireTime", now, time.Hour)
			require.NoError(t, parseErr)
			record := &GeminiCachedContent{AccountID: a.ID, UpstreamName: "cachedContents/created", UpstreamModel: "models/gemini-a"}
			freezeGeminiCachedContentProtocolBinding(t, record, a)
			_, err = s.PatchUpstream(context.Background(), a, record, req)
		}
		require.ErrorIs(t, err, ErrGeminiCachedContentRequestNotSent)
		require.Zero(t, h.calls)
	}
}

func TestGeminiCachedContentProtocolNotFoundClassifiesResourceNotEndpoint(t *testing.T) {
	for _, gateway := range []bool{false, true} {
		for _, tc := range []struct {
			status int
			body   string
			want   bool
		}{
			{404, `{"error":{"code":404,"status":"NOT_FOUND","message":"CachedContent not found"}}`, true},
			{404, `{"error":{"code":404,"status":"NOT_FOUND","message":"Cached content does not exist"}}`, true},
			{403, `{"error":{"code":403,"message":"CachedContent not found"}}`, true},
			{404, `{"error":{"code":404,"status":"NOT_FOUND","message":"CachedContent endpoint not found"}}`, false},
			{404, `{"error":{"code":404,"status":"NOT_FOUND","message":"Method not found"}}`, false},
			{404, `{"error":{"code":404,"status":"NOT_FOUND","message":"Route /v1beta/cachedContents not found"}}`, false},
			{404, `{"error":{"code":404,"status":"NOT_FOUND","message":"Model not found"}}`, false},
			{404, `{"error":{"code":404,"status":"PERMISSION_DENIED","message":"CachedContent not found"}}`, false},
			{404, `{"error":{"code":404,"status":"NOT_FOUND","status":"OK","message":"CachedContent not found"}}`, false},
			{404, `not found`, false},
			{401, `{"error":{"code":401,"message":"CachedContent not found"}}`, false},
		} {
			err := &GeminiCachedContentUpstreamError{StatusCode: tc.status, Body: []byte(tc.body), UpstreamGateway: gateway}
			require.Equal(t, tc.want, err.NotFound(), "%t %d %s", gateway, tc.status, tc.body)
		}
	}
}

func TestGeminiCachedContentProtocolReferences(t *testing.T) {
	for _, body := range []string{
		`{"cachedContent":"cachedContents/abc","cachedContent":"cachedContents/def"}`,
		`{"cachedContent":null}`, `{"cachedContent":12}`, `{"cachedContent":""}`,
		`{"cached_content":"cachedContents/abc"}`, `{"generate_content_request":{"cachedContent":"cachedContents/abc"}}`,
		`{"cachedContent":"cachedContents/abc","generateContentRequest":{"cachedContent":"cachedContents/def"}}`,
		`{"generateContentRequest":{"cachedContent":"a","cached\u0043ontent":"b"}}`,
		`{"generateContentRequest":null}`, `{"cachedContent":"a"} garbage`,
	} {
		_, err := FindGeminiCachedContentReference([]byte(body))
		require.Error(t, err, body)
	}
	body := []byte(`{"cachedContent":"cachedContents/abc","generateContentRequest":{"cachedContent":"cachedContents/abc"},"value":9007199254740993}`)
	ref, err := FindGeminiCachedContentReference(body)
	require.NoError(t, err)
	require.Len(t, ref.Paths, 2)
	out, err := RewriteGeminiCachedContentReference(body, ref, "cachedContents/upstream")
	require.NoError(t, err)
	require.Equal(t, 2, strings.Count(string(out), "cachedContents/upstream"))
	require.Contains(t, string(out), "9007199254740993")
	ref.Paths = []string{"value"}
	_, err = RewriteGeminiCachedContentReference(body, ref, "cachedContents/upstream")
	require.Error(t, err)
}

func TestGeminiCachedContentProtocolAccountAndTTLSafety(t *testing.T) {
	a := geminiCachedContentProtocolAccount()
	require.True(t, IsGeminiExplicitCacheAccount(a))
	a.Credentials["base_url"] = "https://cache.example.invalid/relay"
	require.False(t, IsGeminiExplicitCacheAccount(a))
	a.Credentials[GeminiExplicitCacheUpstreamCredentialKey] = true
	require.True(t, IsGeminiExplicitCacheAccount(a))
	a.Credentials["api_protocol"] = APIProtocolChatCompletions
	require.False(t, IsGeminiExplicitCacheAccount(a))
	delete(a.Credentials, "api_protocol")
	for _, value := range []any{math.NaN(), math.Inf(1), -1, "1e30", "9223372037", float64(1e30), "0", "0.5"} {
		a.Credentials[GeminiExplicitCacheUpstreamMaxTTLCredentialKey] = value
		require.Zero(t, a.GeminiExplicitCacheUpstreamMaxTTL())
		require.False(t, IsGeminiExplicitCacheAccount(a), "%v", value)
	}
	a.Credentials[GeminiExplicitCacheUpstreamMaxTTLCredentialKey] = json.Number("45")
	require.Equal(t, 45*time.Second, a.GeminiExplicitCacheUpstreamMaxTTL())
	req, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-a"}`), time.Now(), time.Hour)
	require.NoError(t, err)
	ttl, ok := req.TTLForAccount(a)
	require.True(t, ok)
	require.Equal(t, 45*time.Second, ttl)
	req, err = ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-a","ttl":"46s"}`), time.Now(), time.Hour)
	require.NoError(t, err)
	_, ok = req.TTLForAccount(a)
	require.False(t, ok)
	vertex := geminiCachedContentProtocolVertex()
	require.True(t, IsGeminiExplicitCacheAccount(vertex))
	vertex.Credentials["auth_mode"] = "apikey"
	require.False(t, IsGeminiExplicitCacheAccount(vertex))
	for _, raw := range []string{"https://user@generativelanguage.googleapis.com", "https://generativelanguage.googleapis.com:443", "https://generativelanguage.googleapis.com/path", "https://generativelanguage.googleapis.com?x=1", "https://generativelanguage.googleapis.com#x"} {
		a := geminiCachedContentProtocolAccount()
		a.Credentials["base_url"] = raw
		require.False(t, IsGeminiExplicitCacheAccount(a), raw)
	}
	id, err := NewGeminiCachedContentPublicID()
	require.NoError(t, err)
	require.Len(t, id, 40)
	parsed, valid := ParseGeminiCachedContentPublicID("cachedContents/" + id)
	require.True(t, valid)
	require.Equal(t, id, parsed)
	parsed, valid = ParseGeminiCachedContentPublicID("cachedContents/../other")
	require.False(t, valid)
	require.Empty(t, parsed)
}

func TestGeminiCachedContentProtocolCredentialDestinations(t *testing.T) {
	now := time.Now().UTC()
	for _, tc := range []struct {
		name, target, resource, model, auth, key string
		account                                  *Account
	}{
		{"studio", "https://generativelanguage.googleapis.com/v1beta/cachedContents", "cachedContents/created", "models/gemini-a", "", "cache-test-key", geminiCachedContentProtocolAccount()},
		{"vertex", "https://us-central1-aiplatform.googleapis.com/v1/projects/cache-project/locations/us-central1/cachedContents", "projects/cache-project/locations/us-central1/cachedContents/created", "projects/cache-project/locations/us-central1/publishers/google/models/gemini-a", "Bearer cache-access-token", "", geminiCachedContentProtocolVertex()},
		{"third-party", "https://cache.example.invalid/relay/v1beta/cachedContents", "cachedContents/created", "models/gemini-a", "", "cache-test-key", func() *Account {
			a := geminiCachedContentProtocolAccount()
			a.Credentials["base_url"] = "https://cache.example.invalid/relay"
			a.Credentials[GeminiExplicitCacheUpstreamCredentialKey] = true
			return a
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &geminiCachedContentProtocolHTTP{do: func(r *http.Request, proxy string, id int64, concurrency int) (*http.Response, error) {
				require.Equal(t, tc.target, r.URL.String())
				require.Equal(t, tc.key, r.Header.Get("x-goog-api-key"))
				require.Equal(t, tc.auth, r.Header.Get("Authorization"))
				require.Empty(t, r.URL.RawQuery)
				require.Equal(t, tc.account.ID, id)
				require.Equal(t, tc.account.Concurrency, concurrency)
				require.Equal(t, "http://127.0.0.1:8888", proxy)
				require.True(t, HTTPUpstreamRedirectsDisabled(r.Context()))
				deadline, ok := r.Context().Deadline()
				require.True(t, ok)
				require.LessOrEqual(t, time.Until(deadline), geminiCachedContentUpstreamTimeout)
				payload, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.Contains(t, string(payload), `"model":"`+tc.model+`"`)
				require.Contains(t, string(payload), `"ttl":"60s"`)
				require.Contains(t, string(payload), `"displayName":"s2a-abc"`)
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(geminiCachedContentProtocolResponse(tc.resource, tc.model, now.Add(time.Minute))))}, nil
			}}
			proxyID := int64(1)
			tc.account.ProxyID, tc.account.Proxy = &proxyID, &Proxy{Protocol: "http", Host: "127.0.0.1", Port: 8888}
			s := geminiCachedContentProtocolService(h, now)
			req, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-a","ttl":"60s"}`), now, time.Hour)
			require.NoError(t, err)
			out, err := s.CreateUpstream(context.Background(), tc.account, req, "gemini-a", "abc", time.Minute)
			require.NoError(t, err)
			require.Equal(t, tc.resource, out.Name)
			identity, identityErr := GeminiCachedContentEndpointIdentity(tc.account, "gemini-a")
			require.NoError(t, identityErr)
			require.Equal(t, identity, out.EndpointIdentity)
			require.Equal(t, int64(2048), out.TotalTokenCount)
			require.Equal(t, 1, h.calls)
		})
	}
}

func TestGeminiCachedContentProtocolRejectsDestinationsBeforeSend(t *testing.T) {
	now := time.Now()
	for _, base := range []string{"https://user@cache.example.invalid", "https://cache.example.invalid/?key=secret", "https://cache.example.invalid/#fragment", "https://cache.example.invalid/../other", "https://cache.example.invalid/%2e%2e", "//cache.example.invalid", "https://cache.example.invalid/relay//other"} {
		t.Run(base, func(t *testing.T) {
			a := geminiCachedContentProtocolAccount()
			a.Credentials["base_url"], a.Credentials[GeminiExplicitCacheUpstreamCredentialKey] = base, true
			h := &geminiCachedContentProtocolHTTP{do: func(*http.Request, string, int64, int) (*http.Response, error) {
				t.Fatal("must not dispatch")
				return nil, nil
			}}
			s := geminiCachedContentProtocolService(h, now)
			req, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-a"}`), now, time.Hour)
			require.NoError(t, err)
			_, err = s.CreateUpstream(context.Background(), a, req, "gemini-a", "abc", time.Minute)
			require.ErrorIs(t, err, ErrGeminiCachedContentRequestNotSent)
			require.Zero(t, h.calls)
		})
	}
	for _, a := range []*Account{nil, {ID: 1, Platform: PlatformGemini, Type: AccountTypeOAuth}, func() *Account {
		a := geminiCachedContentProtocolVertex()
		a.Credentials["auth_mode"] = "apikey"
		return a
	}(), func() *Account {
		a := geminiCachedContentProtocolVertex()
		a.Credentials["base_url"] = "https://other.example.invalid"
		return a
	}()} {
		s := geminiCachedContentProtocolService(nil, now)
		req, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-a"}`), now, time.Hour)
		require.NoError(t, err)
		_, err = s.CreateUpstream(context.Background(), a, req, "gemini-a", "abc", time.Minute)
		require.ErrorIs(t, err, ErrGeminiCachedContentRequestNotSent)
	}
}

func TestGeminiCachedContentProtocolMalformedRecovery(t *testing.T) {
	now := time.Now().UTC()
	good := geminiCachedContentProtocolResponse("cachedContents/created", "models/gemini-a", now.Add(time.Minute))
	for _, tc := range []struct{ name, body, recoverable string }{
		{"syntax", `{bad`, ""}, {"null", `null`, ""}, {"duplicate", `{"name":"cachedContents/one","name":"cachedContents/two"}`, ""},
		{"wrong-kind", geminiCachedContentProtocolResponse("projects/cache-project/locations/us-central1/cachedContents/created", "models/gemini-a", now.Add(time.Minute)), ""},
		{"bad-token-count", strings.ReplaceAll(good, `2048`, `0`), "cachedContents/created"},
		{"bad-expiry", strings.ReplaceAll(good, now.Add(time.Minute).Format(time.RFC3339Nano), "nonsense"), "cachedContents/created"},
		{"late-expiry", geminiCachedContentProtocolResponse("cachedContents/created", "models/gemini-a", now.Add(2*time.Minute)), "cachedContents/created"},
		{"wrong-model", geminiCachedContentProtocolResponse("cachedContents/created", "models/gemini-other", now.Add(time.Minute)), "cachedContents/created"},
		{"expired", geminiCachedContentProtocolResponse("cachedContents/created", "models/gemini-a", now.Add(-time.Second)), "cachedContents/created"},
		{"fractional-count", strings.ReplaceAll(good, `2048`, `1.5`), "cachedContents/created"},
		{"overflow-count", strings.ReplaceAll(good, `2048`, `9223372036854775808`), "cachedContents/created"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &geminiCachedContentProtocolHTTP{do: func(*http.Request, string, int64, int) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			}}
			s := geminiCachedContentProtocolService(h, now)
			req, _ := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-a"}`), now, time.Hour)
			_, err := s.CreateUpstream(context.Background(), geminiCachedContentProtocolAccount(), req, "gemini-a", "abc", time.Minute)
			var malformed *GeminiCachedContentMalformedResponseError
			require.ErrorAs(t, err, &malformed)
			require.Equal(t, tc.recoverable, malformed.Name)
			require.ErrorIs(t, err, ErrGeminiCachedContentOutcomeUncertain)
			require.NotErrorIs(t, err, ErrGeminiCachedContentRequestNotSent)
			require.Equal(t, 1, h.calls)
		})
	}
}

func TestGeminiCachedContentProtocolMutationAccountBinding(t *testing.T) {
	now := time.Now().UTC()
	a := geminiCachedContentProtocolVertex()
	a.Credentials["vertex_model_locations"] = map[string]any{"gemini-a": "global"}
	for _, name := range []string{
		"projects/other-project/locations/global/cachedContents/created",
		"projects/cache-project/locations/us-central1/cachedContents/created",
		"https://evil.invalid/cachedContents/created",
	} {
		s := geminiCachedContentProtocolService(nil, now)
		record := &GeminiCachedContent{AccountID: a.ID, UpstreamName: name, UpstreamModel: "projects/cache-project/locations/global/publishers/google/models/gemini-a"}
		require.ErrorIs(t, s.DeleteUpstream(context.Background(), a, record), ErrGeminiCachedContentRequestNotSent)
	}
	name := "projects/cache-project/locations/global/cachedContents/created"
	model := "projects/cache-project/locations/global/publishers/google/models/gemini-a"
	record := &GeminiCachedContent{AccountID: a.ID, UpstreamName: name, UpstreamModel: model}
	freezeGeminiCachedContentProtocolBinding(t, record, a)
	h := &geminiCachedContentProtocolHTTP{do: func(r *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		require.Equal(t, "aiplatform.googleapis.com", r.URL.Host)
		require.Equal(t, "/v1/"+name, r.URL.Path)
		if r.Method == http.MethodPatch {
			require.Equal(t, "updateMask=ttl", r.URL.RawQuery)
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(geminiCachedContentProtocolResponse(name, model, now.Add(time.Minute))))}, nil
		}
		require.Equal(t, http.MethodDelete, r.Method)
		return &http.Response{StatusCode: 204, Body: http.NoBody}, nil
	}}
	s := geminiCachedContentProtocolService(h, now)
	out, err := s.PatchUpstream(context.Background(), a, record, &GeminiCachedContentPatchRequest{TTL: time.Minute})
	require.NoError(t, err)
	require.Equal(t, now.Add(time.Minute), out.ExpireTime)
	require.NoError(t, s.DeleteUpstream(context.Background(), a, record))
	record.AccountID++
	require.ErrorIs(t, s.DeleteUpstream(context.Background(), a, record), ErrGeminiCachedContentRequestNotSent)
	require.Equal(t, 2, h.calls)
}

func TestGeminiCachedContentProtocolVertexResponseScopeAndPatchRecovery(t *testing.T) {
	now := time.Now().UTC()
	a := geminiCachedContentProtocolVertex()
	model := "projects/cache-project/locations/us-central1/publishers/google/models/gemini-a"
	for _, resource := range []string{
		"cachedContents/created", "projects/other-project/locations/us-central1/cachedContents/created",
		"projects/cache-project/locations/europe-west4/cachedContents/created",
	} {
		h := &geminiCachedContentProtocolHTTP{do: func(*http.Request, string, int64, int) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(geminiCachedContentProtocolResponse(resource, model, now.Add(time.Minute))))}, nil
		}}
		s := geminiCachedContentProtocolService(h, now)
		req, _ := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-a"}`), now, time.Hour)
		_, err := s.CreateUpstream(context.Background(), a, req, "gemini-a", "abc", time.Minute)
		var malformed *GeminiCachedContentMalformedResponseError
		require.ErrorAs(t, err, &malformed)
		require.Empty(t, malformed.Name, "another resource scope is never eligible for cleanup")
		require.Equal(t, 1, h.calls)
	}
	resource := "projects/cache-project/locations/us-central1/cachedContents/created"
	record := &GeminiCachedContent{AccountID: a.ID, UpstreamName: resource, UpstreamModel: model}
	freezeGeminiCachedContentProtocolBinding(t, record, a)
	h := &geminiCachedContentProtocolHTTP{do: func(*http.Request, string, int64, int) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"name":"` + resource + `","model":"` + model + `","expireTime":null}`))}, nil
	}}
	s := geminiCachedContentProtocolService(h, now)
	_, err := s.PatchUpstream(context.Background(), a, record, &GeminiCachedContentPatchRequest{TTL: time.Minute})
	var malformed *GeminiCachedContentMalformedResponseError
	require.ErrorAs(t, err, &malformed)
	require.Equal(t, resource, malformed.Name)
	require.ErrorIs(t, err, ErrGeminiCachedContentOutcomeUncertain)
	require.Equal(t, 1, h.calls)
}

func TestGeminiCachedContentProtocolMutationTTLRevalidation(t *testing.T) {
	now := time.Now()
	a := geminiCachedContentProtocolAccount()
	a.Credentials[GeminiExplicitCacheUpstreamCredentialKey] = true
	a.Credentials[GeminiExplicitCacheUpstreamMaxTTLCredentialKey] = 60
	h := &geminiCachedContentProtocolHTTP{do: func(*http.Request, string, int64, int) (*http.Response, error) {
		t.Fatal("invalid ttl dispatched")
		return nil, nil
	}}
	s := geminiCachedContentProtocolService(h, now)
	req, _ := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-a","ttl":"60s"}`), now, time.Hour)
	for _, ttl := range []time.Duration{0, -1, 61 * time.Second, time.Duration(math.MaxInt64), time.Hour} {
		_, err := s.CreateUpstream(context.Background(), a, req, "gemini-a", "abc", ttl)
		require.ErrorIs(t, err, ErrGeminiCachedContentRequestNotSent)
	}
	_, err := s.CreateUpstream(context.Background(), a, req, "gemini-a", "abc", 59*time.Second)
	require.ErrorIs(t, err, ErrGeminiCachedContentRequestNotSent, "explicit request ttl cannot silently change")
	record := &GeminiCachedContent{AccountID: a.ID, UpstreamName: "cachedContents/created", UpstreamModel: "models/gemini-a"}
	freezeGeminiCachedContentProtocolBinding(t, record, a)
	_, err = s.PatchUpstream(context.Background(), a, record, &GeminiCachedContentPatchRequest{TTL: 60*time.Second + time.Nanosecond})
	require.ErrorIs(t, err, ErrGeminiCachedContentRequestNotSent, "ceil rounding cannot exceed account cap")
	s.maxTTL = -1
	require.Zero(t, s.MaxTTLForAccount(a))
	require.Zero(t, h.calls)
}

func TestGeminiCachedContentProtocolAuthBoundaryAndNoRetry(t *testing.T) {
	now := time.Now().UTC()
	for _, a := range []*Account{geminiCachedContentProtocolAccount(), geminiCachedContentProtocolVertex()} {
		h := &geminiCachedContentProtocolHTTP{do: func(*http.Request, string, int64, int) (*http.Response, error) {
			t.Fatal("credentials escaped account cache endpoint")
			return nil, nil
		}}
		s := geminiCachedContentProtocolService(h, now)
		for _, target := range []string{
			"https://other.example.invalid/v1beta/cachedContents",
			"https://generativelanguage.googleapis.com/v1beta/cachedContents?key=secret",
			"https://aiplatform.googleapis.com/v1/projects/other/locations/global/cachedContents",
			"https://europe-west4-aiplatform.googleapis.com/v1/projects/cache-project/locations/europe-west4/cachedContents",
			"https://us-central1-aiplatform.googleapis.com/evil/projects/cache-project/locations/us-central1/cachedContents",
		} {
			_, err := s.doUpstream(context.Background(), a, http.MethodPost, target, []byte(`{}`), time.Time{}, time.Second)
			require.ErrorIs(t, err, ErrGeminiCachedContentRequestNotSent)
		}
		require.Zero(t, h.calls)
	}
	for _, status := range []int{307, 403, 429, 500} {
		h := &geminiCachedContentProtocolHTTP{do: func(r *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
			require.True(t, HTTPUpstreamRedirectsDisabled(r.Context()))
			return &http.Response{StatusCode: status, Header: http.Header{"Location": {"https://other.example.invalid"}}, Body: io.NopCloser(strings.NewReader(`{"error":"rejected"}`))}, nil
		}}
		s := geminiCachedContentProtocolService(h, now)
		record := &GeminiCachedContent{AccountID: 71, UpstreamName: "cachedContents/created", UpstreamModel: "models/gemini-a"}
		freezeGeminiCachedContentProtocolBinding(t, record, geminiCachedContentProtocolAccount())
		err := s.DeleteUpstream(context.Background(), geminiCachedContentProtocolAccount(), record)
		var upstream *GeminiCachedContentUpstreamError
		require.ErrorAs(t, err, &upstream)
		require.Equal(t, status, upstream.StatusCode)
		require.Equal(t, status == 500, errors.Is(err, ErrGeminiCachedContentOutcomeUncertain))
		require.Equal(t, 1, h.calls)
	}
	// A token provider without credentials and an absent URL policy both fail
	// before cache dispatch; no token refresh network call can occur here.
	a := geminiCachedContentProtocolVertex()
	a.Credentials["service_account_json"] = "malformed"
	h := &geminiCachedContentProtocolHTTP{do: func(*http.Request, string, int64, int) (*http.Response, error) {
		t.Fatal("missing auth dispatched")
		return nil, nil
	}}
	s := geminiCachedContentProtocolService(h, now)
	record := &GeminiCachedContent{AccountID: a.ID, UpstreamName: "projects/cache-project/locations/us-central1/cachedContents/created", UpstreamModel: "projects/cache-project/locations/us-central1/publishers/google/models/gemini-a"}
	require.ErrorIs(t, s.DeleteUpstream(context.Background(), a, record), ErrGeminiCachedContentRequestNotSent)
	s.compat.cfg = nil
	require.ErrorIs(t, s.DeleteUpstream(context.Background(), a, record), ErrGeminiCachedContentRequestNotSent)
	require.Zero(t, h.calls)
}

func TestGeminiCachedContentProtocolTransportSafetyAndCancellation(t *testing.T) {
	now := time.Now().UTC()
	a := geminiCachedContentProtocolAccount()
	record := &GeminiCachedContent{AccountID: a.ID, UpstreamName: "cachedContents/created", UpstreamModel: "models/gemini-a"}
	freezeGeminiCachedContentProtocolBinding(t, record, a)
	for _, tc := range []struct {
		name     string
		response *http.Response
		err      error
	}{
		{"nil-response", nil, nil}, {"nil-body", &http.Response{StatusCode: 200}, nil},
		{"oversized", &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", geminiCachedContentResponseMaxBytes+1)))}, nil},
		{"transport", nil, io.ErrUnexpectedEOF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &geminiCachedContentProtocolHTTP{do: func(*http.Request, string, int64, int) (*http.Response, error) { return tc.response, tc.err }}
			s := geminiCachedContentProtocolService(h, now)
			err := s.DeleteUpstream(context.Background(), a, record)
			require.ErrorIs(t, err, ErrGeminiCachedContentOutcomeUncertain)
			require.Equal(t, 1, h.calls)
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h := &geminiCachedContentProtocolHTTP{do: func(*http.Request, string, int64, int) (*http.Response, error) {
		t.Fatal("pre-canceled request dispatched")
		return nil, nil
	}}
	s := geminiCachedContentProtocolService(h, now)
	require.ErrorIs(t, s.DeleteUpstream(ctx, a, record), ErrGeminiCachedContentRequestNotSent)
	require.Zero(t, h.calls)
	ctx, cancel = context.WithCancel(context.Background())
	h.do = func(r *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		cancel()
		require.NoError(t, r.Context().Err(), "dispatched mutations retain a bounded completion context")
		return &http.Response{StatusCode: 204, Body: http.NoBody}, nil
	}
	require.NoError(t, s.DeleteUpstream(ctx, a, record))
	h.do = func(r *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	}
	_, err := s.doUpstream(context.Background(), a, http.MethodDelete, "https://generativelanguage.googleapis.com/v1beta/cachedContents/created", nil, time.Time{}, 10*time.Millisecond)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(t, err, ErrGeminiCachedContentOutcomeUncertain)
	require.False(t, errors.Is(err, ErrGeminiCachedContentRequestNotSent))
}
