//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func geminiCachedContentRecoveryBinding(t *testing.T, account *Account, now time.Time) *GeminiCachedContent {
	t.Helper()
	model, err := GeminiCachedContentUpstreamModelResource(account, "gemini-a")
	require.NoError(t, err)
	record := &GeminiCachedContent{AccountID: account.ID, PublicID: "abc", UpstreamModel: model, ExpireTime: now.Add(time.Hour)}
	freezeGeminiCachedContentProtocolBinding(t, record, account)
	return record
}

func geminiCachedContentRecoveryEntry(record *GeminiCachedContent) map[string]any {
	name := record.UpstreamName
	if name == "" {
		name = "cachedContents/created"
		if strings.HasPrefix(record.UpstreamModel, "projects/") {
			name = strings.Split(record.UpstreamModel, "/publishers/")[0] + "/cachedContents/created"
		}
	}
	return map[string]any{"name": name, "model": record.UpstreamModel,
		"displayName": "s2a-" + record.PublicID, "expireTime": record.ExpireTime.Format(time.RFC3339Nano),
		"usageMetadata": map[string]any{"totalTokenCount": 2048}, "contents": []any{map[string]any{"text": "never returned"}}}
}

func geminiCachedContentRecoveryJSON(value any) string {
	b, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func geminiCachedContentRecoveryResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}
}

func requireGeminiCachedContentRecoveryUncertain(t *testing.T, out *GeminiCachedContentUpstream, err error) {
	t.Helper()
	require.Nil(t, out)
	require.ErrorIs(t, err, ErrGeminiCachedContentOutcomeUncertain)
	require.NotErrorIs(t, err, ErrGeminiCachedContentRequestNotSent)
	var recovery *GeminiCachedContentRecoveryReadError
	require.ErrorAs(t, err, &recovery)
	require.Nil(t, errors.Unwrap(recovery), "a failed recovery read cannot classify the original POST")
	require.NotContains(t, err.Error(), "never returned")
}

func TestGeminiCachedContentRecoveryReadDestinationsAndCompleteConfirmation(t *testing.T) {
	now := time.Now().UTC()
	custom := geminiCachedContentProtocolAccount()
	custom.Credentials["base_url"], custom.Credentials[GeminiExplicitCacheUpstreamCredentialKey] = "https://cache.example.invalid/relay", true
	vertex := geminiCachedContentProtocolVertex()
	vertex.Credentials["vertex_model_locations"] = map[string]any{"gemini-a": "global"}
	for _, tc := range []struct {
		name, collection, key, auth string
		account                     *Account
	}{
		{"studio", "https://generativelanguage.googleapis.com/v1beta/cachedContents", "cache-test-key", "", geminiCachedContentProtocolAccount()},
		{"custom", "https://cache.example.invalid/relay/v1beta/cachedContents", "cache-test-key", "", custom},
		{"vertex", "https://aiplatform.googleapis.com/v1/projects/cache-project/locations/global/cachedContents", "", "Bearer cache-access-token", vertex},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := geminiCachedContentRecoveryBinding(t, tc.account, now)
			entry := geminiCachedContentRecoveryEntry(record)
			other := geminiCachedContentRecoveryEntry(record)
			other["displayName"] = "unrelated-cache"
			token := "opaque /?+&=%"
			var deadline time.Time
			h := &geminiCachedContentProtocolHTTP{}
			h.do = func(r *http.Request, _ string, id int64, _ int) (*http.Response, error) {
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, tc.account.ID, id)
				require.Equal(t, tc.key, r.Header.Get("x-goog-api-key"))
				require.Equal(t, tc.auth, r.Header.Get("Authorization"))
				require.True(t, HTTPUpstreamRedirectsDisabled(r.Context()))
				require.True(t, r.Body == nil || r.Body == http.NoBody)
				currentDeadline, ok := r.Context().Deadline()
				require.True(t, ok)
				require.LessOrEqual(t, time.Until(currentDeadline), geminiCachedContentRecoveryReadTimeout)
				if deadline.IsZero() {
					deadline = currentDeadline
				} else {
					require.False(t, currentDeadline.After(deadline), "pages and GET cannot restart the total budget")
				}
				switch h.calls {
				case 1:
					require.Equal(t, tc.collection+"?pageSize=100", r.URL.String())
					return geminiCachedContentRecoveryResponse(geminiCachedContentRecoveryJSON(map[string]any{"cachedContents": []any{entry}, "nextPageToken": token})), nil
				case 2:
					require.Equal(t, tc.collection+"?"+url.Values{"pageSize": {"100"}, "pageToken": {token}}.Encode(), r.URL.String())
					return geminiCachedContentRecoveryResponse(geminiCachedContentRecoveryJSON(map[string]any{"cachedContents": []any{other}})), nil
				default:
					require.Equal(t, tc.collection+"/created", r.URL.String())
					return geminiCachedContentRecoveryResponse(geminiCachedContentRecoveryJSON(entry)), nil
				}
			}
			out, err := geminiCachedContentProtocolService(h, now).DiscoverCreatedUpstream(context.Background(), tc.account, record)
			require.NoError(t, err)
			require.Equal(t, entry["name"], out.Name)
			require.Equal(t, record.UpstreamModel, out.Model)
			require.Equal(t, int64(2048), out.TotalTokenCount)
			require.Equal(t, record.UpstreamEndpointIdentity, out.EndpointIdentity)
			require.Equal(t, 3, h.calls)
			require.Empty(t, record.UpstreamName, "discovery does not persist or mutate the pending binding")
		})
	}
}

func TestGeminiCachedContentRecoveryReadChangedBindingNeverDispatches(t *testing.T) {
	now := time.Now().UTC()
	for _, change := range []string{"endpoint", "key", "account", "identity", "model", "expired", "public id", "custom opt-out", "vertex principal", "vertex project", "vertex location", "vertex auth"} {
		for _, operation := range []string{"get", "discover"} {
			t.Run(change+"/"+operation, func(t *testing.T) {
				a := geminiCachedContentProtocolAccount()
				if strings.HasPrefix(change, "vertex") {
					a = geminiCachedContentProtocolVertex()
				}
				if change == "custom opt-out" {
					a.Credentials["base_url"], a.Credentials[GeminiExplicitCacheUpstreamCredentialKey] = "https://cache.example.invalid", true
				}
				record := geminiCachedContentRecoveryBinding(t, a, now)
				if operation == "get" {
					record.UpstreamName = geminiCachedContentRecoveryEntry(record)["name"].(string)
				}
				switch change {
				case "endpoint":
					a.Credentials["base_url"], a.Credentials[GeminiExplicitCacheUpstreamCredentialKey] = "https://changed.invalid", true
				case "key":
					a.Credentials["api_key"] = "rotated"
				case "account":
					record.AccountID++
				case "identity":
					record.UpstreamEndpointIdentity = ""
				case "model":
					record.UpstreamModel = "models/../gemini-a"
				case "expired":
					record.ExpireTime = now
				case "public id":
					record.PublicID = "../abc"
				case "custom opt-out":
					a.Credentials[GeminiExplicitCacheUpstreamCredentialKey] = false
				case "vertex principal":
					a.Credentials["service_account_json"] = `{"project_id":"cache-project","client_email":"changed@example.invalid","private_key":"unused"}`
				case "vertex project":
					a.Credentials["project_id"] = "other-project"
				case "vertex location":
					a.Credentials["location"] = "global"
				case "vertex auth":
					a.Credentials["auth_mode"] = "apikey"
				}
				h := &geminiCachedContentProtocolHTTP{do: func(*http.Request, string, int64, int) (*http.Response, error) {
					t.Fatal("changed binding must not use credentials or network")
					return nil, nil
				}}
				s := geminiCachedContentProtocolService(h, now)
				var out *GeminiCachedContentUpstream
				var err error
				if operation == "get" {
					out, err = s.GetUpstream(context.Background(), a, record)
				} else {
					out, err = s.DiscoverCreatedUpstream(context.Background(), a, record)
				}
				requireGeminiCachedContentRecoveryUncertain(t, out, err)
				require.Zero(t, h.calls)
			})
		}
	}
}

func TestGeminiCachedContentRecoveryReadRejectsIncompleteScans(t *testing.T) {
	now := time.Now().UTC()
	for _, scenario := range []string{"none", "duplicate", "later duplicate", "invalid JSON", "duplicate JSON key", "missing array", "null array", "invalid entry", "malformed metadata", "later invalid JSON", "later failure", "cycle", "page limit", "entry limit", "token limit", "control token", "wrong token type", "oversized response", "redirect", "not found"} {
		t.Run(scenario, func(t *testing.T) {
			a := geminiCachedContentProtocolAccount()
			record := geminiCachedContentRecoveryBinding(t, a, now)
			entry := geminiCachedContentRecoveryEntry(record)
			h := &geminiCachedContentProtocolHTTP{}
			h.do = func(r *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "/v1beta/cachedContents", r.URL.Path, "failed scans never confirm an early match")
				page := map[string]any{"cachedContents": []any{entry}}
				switch scenario {
				case "none":
					page["cachedContents"] = []any{}
				case "duplicate":
					page["cachedContents"] = []any{entry, entry}
				case "later duplicate", "later invalid JSON", "later failure":
					if h.calls == 1 {
						page["nextPageToken"] = "second"
					} else if scenario == "later invalid JSON" {
						return geminiCachedContentRecoveryResponse(`{broken`), nil
					} else if scenario == "later failure" {
						return nil, io.ErrUnexpectedEOF
					}
				case "invalid JSON":
					return geminiCachedContentRecoveryResponse(`{broken`), nil
				case "duplicate JSON key":
					return geminiCachedContentRecoveryResponse(`{"cachedContents":[],"cachedContents":[]}`), nil
				case "missing array":
					delete(page, "cachedContents")
				case "null array":
					page["cachedContents"] = nil
				case "invalid entry":
					page["cachedContents"] = []any{entry, nil}
				case "malformed metadata":
					page["cachedContents"] = []any{entry, map[string]any{}}
				case "cycle":
					page["nextPageToken"] = "same"
					if h.calls > 1 {
						page["cachedContents"] = []any{}
					}
				case "page limit":
					page["nextPageToken"] = strings.Repeat("n", h.calls)
					if h.calls > 1 {
						page["cachedContents"] = []any{}
					}
				case "entry limit":
					entries := make([]any, 101)
					for i := range entries {
						entries[i] = entry
					}
					page["cachedContents"] = entries
				case "token limit":
					page["nextPageToken"] = strings.Repeat("x", 2049)
				case "control token":
					page["nextPageToken"] = "next\npage"
				case "wrong token type":
					page["nextPageToken"] = 42
				case "oversized response":
					return geminiCachedContentRecoveryResponse(strings.Repeat("x", geminiCachedContentResponseMaxBytes+1)), nil
				case "redirect", "not found":
					status := http.StatusTemporaryRedirect
					if scenario == "not found" {
						status = http.StatusNotFound
					}
					return &http.Response{StatusCode: status, Header: http.Header{"Location": {"https://evil.invalid"}}, Body: io.NopCloser(strings.NewReader(`{"contents":"never returned"}`))}, nil
				}
				return geminiCachedContentRecoveryResponse(geminiCachedContentRecoveryJSON(page)), nil
			}
			out, err := geminiCachedContentProtocolService(h, now).DiscoverCreatedUpstream(context.Background(), a, record)
			requireGeminiCachedContentRecoveryUncertain(t, out, err)
			if scenario == "page limit" {
				require.Equal(t, 10, h.calls)
			} else {
				require.LessOrEqual(t, h.calls, 2)
			}
		})
	}
}

func TestGeminiCachedContentRecoveryReadAcceptsExactBounds(t *testing.T) {
	now := time.Now().UTC()
	a := geminiCachedContentProtocolAccount()
	record := geminiCachedContentRecoveryBinding(t, a, now)
	entry := geminiCachedContentRecoveryEntry(record)
	h := &geminiCachedContentProtocolHTTP{}
	h.do = func(r *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		require.Equal(t, http.MethodGet, r.Method)
		if r.URL.RawQuery == "" {
			return geminiCachedContentRecoveryResponse(geminiCachedContentRecoveryJSON(entry)), nil
		}
		require.LessOrEqual(t, h.calls, 10)
		if h.calls > 1 {
			require.Len(t, r.URL.Query().Get("pageToken"), 2048)
		}
		entries := make([]any, 100)
		for i := range entries {
			other := geminiCachedContentRecoveryEntry(record)
			other["name"] = "cachedContents/other" + strconv.Itoa(h.calls*100+i)
			other["displayName"] = "unrelated-cache"
			other["model"] = "models/other-model"
			other["expireTime"] = now.Add(-time.Second).Format(time.RFC3339Nano)
			entries[i] = other
		}
		page := map[string]any{"cachedContents": entries}
		if h.calls == 10 {
			entries[99] = entry
		} else {
			page["nextPageToken"] = strings.Repeat("x", 2047) + strconv.Itoa(h.calls)
		}
		return geminiCachedContentRecoveryResponse(geminiCachedContentRecoveryJSON(page)), nil
	}
	out, err := geminiCachedContentProtocolService(h, now).DiscoverCreatedUpstream(context.Background(), a, record)
	require.NoError(t, err)
	require.Equal(t, entry["name"], out.Name)
	require.Equal(t, 11, h.calls, "1000 entries over ten pages, then one confirmation")
}

func TestGeminiCachedContentRecoveryReadMetadataValidation(t *testing.T) {
	now := time.Now().UTC()
	for _, invalid := range []string{"name", "model", "tag", "tag type", "zero tokens", "fractional tokens", "overflow tokens", "missing tokens", "expiry", "expired", "late expiry"} {
		for _, operation := range []string{"get", "discover", "confirm"} {
			t.Run(invalid+"/"+operation, func(t *testing.T) {
				a := geminiCachedContentProtocolAccount()
				record := geminiCachedContentRecoveryBinding(t, a, now)
				entry := geminiCachedContentRecoveryEntry(record)
				good := geminiCachedContentRecoveryJSON(entry)
				if operation == "get" {
					record.UpstreamName = entry["name"].(string)
				}
				switch invalid {
				case "name":
					entry["name"] = "projects/other/locations/global/cachedContents/created"
				case "model":
					entry["model"] = "models/other"
				case "tag":
					entry["displayName"] = "s2a-other"
				case "tag type":
					entry["displayName"] = map[string]any{"bad": true}
				case "zero tokens":
					entry["usageMetadata"] = map[string]any{"totalTokenCount": 0}
				case "fractional tokens":
					entry["usageMetadata"] = map[string]any{"totalTokenCount": 1.5}
				case "overflow tokens":
					entry["usageMetadata"] = map[string]any{"totalTokenCount": json.Number("9223372036854775808")}
				case "missing tokens":
					delete(entry, "usageMetadata")
				case "expiry":
					entry["expireTime"] = "bad"
				case "expired":
					entry["expireTime"] = now.Format(time.RFC3339Nano)
				case "late expiry":
					entry["expireTime"] = record.ExpireTime.Add(time.Second).Format(time.RFC3339Nano)
				}
				h := &geminiCachedContentProtocolHTTP{do: func(r *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
					require.Equal(t, http.MethodGet, r.Method)
					if r.URL.RawQuery != "" {
						if operation == "confirm" {
							return geminiCachedContentRecoveryResponse(`{"cachedContents":[` + good + `]}`), nil
						}
						return geminiCachedContentRecoveryResponse(geminiCachedContentRecoveryJSON(map[string]any{"cachedContents": []any{entry}})), nil
					}
					return geminiCachedContentRecoveryResponse(geminiCachedContentRecoveryJSON(entry)), nil
				}}
				s := geminiCachedContentProtocolService(h, now)
				var out *GeminiCachedContentUpstream
				var err error
				if operation == "get" {
					out, err = s.GetUpstream(context.Background(), a, record)
				} else {
					out, err = s.DiscoverCreatedUpstream(context.Background(), a, record)
				}
				requireGeminiCachedContentRecoveryUncertain(t, out, err)
			})
		}
	}
}

func TestGeminiCachedContentRecoveryReadConfirmationMustAgree(t *testing.T) {
	now := time.Now().UTC()
	for _, change := range []string{"name", "tokens", "expiration"} {
		t.Run(change, func(t *testing.T) {
			a := geminiCachedContentProtocolAccount()
			record := geminiCachedContentRecoveryBinding(t, a, now)
			entry := geminiCachedContentRecoveryEntry(record)
			h := &geminiCachedContentProtocolHTTP{do: func(r *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
				require.Equal(t, http.MethodGet, r.Method)
				if r.URL.RawQuery != "" {
					return geminiCachedContentRecoveryResponse(geminiCachedContentRecoveryJSON(map[string]any{"cachedContents": []any{entry}})), nil
				}
				switch change {
				case "name":
					entry["name"] = "cachedContents/other"
				case "tokens":
					entry["usageMetadata"] = map[string]any{"totalTokenCount": 1024}
				case "expiration":
					entry["expireTime"] = record.ExpireTime.Add(-time.Second).Format(time.RFC3339Nano)
				}
				return geminiCachedContentRecoveryResponse(geminiCachedContentRecoveryJSON(entry)), nil
			}}
			out, err := geminiCachedContentProtocolService(h, now).DiscoverCreatedUpstream(context.Background(), a, record)
			requireGeminiCachedContentRecoveryUncertain(t, out, err)
			require.Equal(t, 2, h.calls)
		})
	}
}

func TestGeminiCachedContentRecoveryReadCancellationRemainsUncertain(t *testing.T) {
	now := time.Now().UTC()
	for _, phase := range []string{"before", "dispatch", "later page", "confirmation", "total deadline"} {
		t.Run(phase, func(t *testing.T) {
			a := geminiCachedContentProtocolAccount()
			record := geminiCachedContentRecoveryBinding(t, a, now)
			entry := geminiCachedContentRecoveryEntry(record)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if phase == "before" {
				cancel()
			}
			if phase == "total deadline" {
				var deadlineCancel context.CancelFunc
				ctx, deadlineCancel = context.WithTimeout(ctx, 10*time.Millisecond)
				defer deadlineCancel()
			}
			h := &geminiCachedContentProtocolHTTP{}
			h.do = func(r *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
				require.Equal(t, http.MethodGet, r.Method)
				if h.calls == 1 && (phase == "later page" || phase == "confirmation") {
					page := map[string]any{"cachedContents": []any{entry}}
					if phase == "later page" {
						page["nextPageToken"] = "second"
					}
					return geminiCachedContentRecoveryResponse(geminiCachedContentRecoveryJSON(page)), nil
				}
				if phase == "total deadline" {
					<-r.Context().Done()
				} else {
					cancel()
				}
				require.Error(t, r.Context().Err(), "reads retain caller cancellation after dispatch")
				return nil, r.Context().Err()
			}
			out, err := geminiCachedContentProtocolService(h, now).DiscoverCreatedUpstream(ctx, a, record)
			requireGeminiCachedContentRecoveryUncertain(t, out, err)
			if phase == "before" {
				require.Zero(t, h.calls)
			}
		})
	}
}

func TestGeminiCachedContentRecoveryReadCredentialBoundary(t *testing.T) {
	now := time.Now().UTC()
	for _, account := range []*Account{geminiCachedContentProtocolAccount(), geminiCachedContentProtocolVertex()} {
		h := &geminiCachedContentProtocolHTTP{do: func(*http.Request, string, int64, int) (*http.Response, error) {
			t.Fatal("invalid read destination must not dispatch")
			return nil, nil
		}}
		s := geminiCachedContentProtocolService(h, now)
		base, err := s.apiBase(account)
		require.NoError(t, err)
		collection, err := geminiCachedContentCollectionURL(account, base, "gemini-a")
		require.NoError(t, err)
		for _, target := range []string{
			collection, collection + "?pageSize=1", collection + "?pageSize=0100", collection + "?pageSize=100&pageSize=100",
			collection + "?pageSize=100&key=secret", collection + "?pageSize=100&pageToken=", collection + "?pageSize=100&pageToken=%0A",
			collection + "?pageToken=next&pageSize=100", collection + "?pageSize=100&pageToken=" + strings.Repeat("x", 2049),
			collection + "/created?pageSize=100", collection + "/../cachedContents?pageSize=100", collection + "/%63reated",
			collection + "/created?key=secret", collection + "?pageSize=100#fragment", "https://evil.invalid/v1beta/cachedContents?pageSize=100",
		} {
			_, err := s.doUpstream(context.Background(), account, http.MethodGet, target, nil, time.Time{}, time.Second)
			require.ErrorIs(t, err, ErrGeminiCachedContentRequestNotSent, target)
		}
		require.Zero(t, h.calls)
	}
}

type geminiCachedContentRecoveryCanceledTokenCache struct {
	GeminiTokenCache
}

func (geminiCachedContentRecoveryCanceledTokenCache) GetAccessToken(ctx context.Context, _ string) (string, error) {
	<-ctx.Done()
	// Even a dependency returning a cached token after cancellation must not
	// allow the caller to dispatch the read.
	return "cache-access-token", nil
}

func TestGeminiCachedContentRecoveryReadVertexAuthCancellation(t *testing.T) {
	for _, operation := range []string{"get", "discover"} {
		t.Run(operation, func(t *testing.T) {
			now := time.Now().UTC()
			a := geminiCachedContentProtocolVertex()
			record := geminiCachedContentRecoveryBinding(t, a, now)
			if operation == "get" {
				record.UpstreamName = geminiCachedContentRecoveryEntry(record)["name"].(string)
			}
			h := &geminiCachedContentProtocolHTTP{do: func(*http.Request, string, int64, int) (*http.Response, error) {
				t.Fatal("canceled authentication must not dispatch a read")
				return nil, nil
			}}
			s := geminiCachedContentProtocolService(h, now)
			s.compat.tokenProvider.tokenCache = geminiCachedContentRecoveryCanceledTokenCache{}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			var out *GeminiCachedContentUpstream
			var err error
			if operation == "get" {
				out, err = s.GetUpstream(ctx, a, record)
			} else {
				out, err = s.DiscoverCreatedUpstream(ctx, a, record)
			}
			requireGeminiCachedContentRecoveryUncertain(t, out, err)
			require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
			require.Zero(t, h.calls)
		})
	}
}

type geminiCachedContentRecoveryCanceledBody struct {
	ctx    context.Context
	cancel context.CancelFunc
	closed bool
}

func (b *geminiCachedContentRecoveryCanceledBody) Read([]byte) (int, error) {
	b.cancel()
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}

func (b *geminiCachedContentRecoveryCanceledBody) Close() error {
	b.closed = true
	return nil
}

func TestGeminiCachedContentRecoveryReadBodyCancellation(t *testing.T) {
	for _, a := range []*Account{geminiCachedContentProtocolAccount(), geminiCachedContentProtocolVertex()} {
		for _, operation := range []string{"get", "discover"} {
			t.Run(a.Type+"/"+operation, func(t *testing.T) {
				now := time.Now().UTC()
				record := geminiCachedContentRecoveryBinding(t, a, now)
				if operation == "get" {
					record.UpstreamName = geminiCachedContentRecoveryEntry(record)["name"].(string)
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				body := &geminiCachedContentRecoveryCanceledBody{cancel: cancel}
				h := &geminiCachedContentProtocolHTTP{do: func(r *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
					body.ctx = r.Context()
					return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
				}}
				s := geminiCachedContentProtocolService(h, now)
				var out *GeminiCachedContentUpstream
				var err error
				if operation == "get" {
					out, err = s.GetUpstream(ctx, a, record)
				} else {
					out, err = s.DiscoverCreatedUpstream(ctx, a, record)
				}
				requireGeminiCachedContentRecoveryUncertain(t, out, err)
				require.ErrorIs(t, ctx.Err(), context.Canceled)
				require.True(t, body.closed)
				require.Equal(t, 1, h.calls)
			})
		}
	}
}
