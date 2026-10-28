package typesafe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSystemOneEndpointAcceptsOriginAndVersionedBases(t *testing.T) {
	for _, tc := range []struct{ base, path string }{
		{"https://example.com", "/v1/systemone"},
		{"https://example.com/", "/v1/systemone"},
		{"https://example.com/v1", "/v1/systemone"},
		{"https://example.com/v1/", "/v1/systemone"},
		{"https://example.com/relay", "/relay/v1/systemone"},
		{"https://example.com/relay/v1", "/relay/v1/systemone"},
	} {
		t.Run(tc.base, func(t *testing.T) {
			req, err := NewSystemOneRequest(context.Background(), tc.base, "fixture", []byte(`{}`))
			require.NoError(t, err)
			require.Equal(t, tc.path, req.URL.Path)
			require.Equal(t, "Bearer fixture", req.Header.Get("Authorization"))
			require.Equal(t, http.MethodPost, req.Method)
		})
	}
}

func TestEvaluateContract(t *testing.T) {
	questions := map[string]Question{"sexual": {Type: "noul", Instructions: "Evaluate sexual content"}, "illicit": {Type: "noul", Instructions: "Evaluate illegal assistance"}}
	for _, tc := range []struct {
		name, response string
		valid          bool
	}{
		{"valid", `{"model":"jev-1.13.0","answers":{"sexual":{"type":"noul","noul":0},"illicit":{"type":"noul","noul":1}}}`, true},
		{"missing_answer", `{"model":"jev-1.13.0","answers":{"sexual":{"type":"noul","noul":0}}}`, false},
		{"missing_probability", `{"model":"jev-1.13.0","answers":{"sexual":{"type":"noul"},"illicit":{"type":"noul","noul":0}}}`, false},
		{"negative", `{"model":"jev-1.13.0","answers":{"sexual":{"type":"noul","noul":-0.1},"illicit":{"type":"noul","noul":0}}}`, false},
		{"above_one", `{"model":"jev-1.13.0","answers":{"sexual":{"type":"noul","noul":1.1},"illicit":{"type":"noul","noul":0}}}`, false},
		{"wrong_type", `{"model":"jev-1.13.0","answers":{"sexual":{"type":"score","noul":0},"illicit":{"type":"noul","noul":0}}}`, false},
		{"missing_model", `{"answers":{"sexual":{"type":"noul","noul":0},"illicit":{"type":"noul","noul":1}}}`, true},
		{"blank_model", `{"model":"  ","answers":{"sexual":{"type":"noul","noul":0},"illicit":{"type":"noul","noul":1}}}`, true},
		{"echoed_key_model", `{"model":"test-secret","answers":{"sexual":{"type":"noul","noul":0},"illicit":{"type":"noul","noul":1}}}`, true},
		{"wrapped_key_model", `{"model":"jev-test-secret-result","answers":{"sexual":{"type":"noul","noul":0},"illicit":{"type":"noul","noul":1}}}`, true},
		{"echoed_text_model", `{"model":"sample","answers":{"sexual":{"type":"noul","noul":0},"illicit":{"type":"noul","noul":1}}}`, true},
		{"echoed_url_model", `{"model":"https://private.example/secret","answers":{"sexual":{"type":"noul","noul":0},"illicit":{"type":"noul","noul":1}}}`, true},
		{"invalid_json", `{bad`, false},
		{"null_answer", `{"model":"jev-1","answers":{"sexual":{"type":"noul","noul":null},"illicit":{"type":"noul","noul":0}}}`, false},
		{"nonfinite", `{"model":"jev-1","answers":{"sexual":{"type":"noul","noul":1e999},"illicit":{"type":"noul","noul":0}}}`, false},
		{"trailing_json", `{"model":"jev-1","answers":{"sexual":{"type":"noul","noul":0},"illicit":{"type":"noul","noul":0}}}{}`, false},
		{"oversize", `{"model":"jev-1","answers":{"sexual":{"type":"noul","noul":0},"illicit":{"type":"noul","noul":0}},"padding":"` + strings.Repeat("x", 1<<20) + `"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/v1/systemone", r.URL.Path)
				require.Equal(t, "Bearer test-secret", r.Header.Get("Authorization"))
				var input Request
				require.NoError(t, json.NewDecoder(r.Body).Decode(&input))
				require.Equal(t, questions, input.Questions)
				require.Equal(t, "sample", input.State)
				_, err := w.Write([]byte(tc.response))
				require.NoError(t, err)
			}))
			defer server.Close()
			result, status, err := Evaluate(context.Background(), server.Client(), server.URL, "test-secret", Request{Model: "jev-latest", State: "sample", Questions: questions})
			require.Equal(t, 200, status)
			if tc.valid {
				require.NoError(t, err)
				if strings.HasPrefix(tc.name, "echoed_") || strings.HasPrefix(tc.name, "wrapped_") || tc.name == "missing_model" || tc.name == "blank_model" {
					require.Empty(t, result.Model)
				} else {
					require.Equal(t, "jev-1.13.0", result.Model)
				}
				require.Equal(t, 1.0, result.Scores["illicit"])
			} else {
				require.Error(t, err)
				require.Nil(t, result)
				require.NotContains(t, err.Error(), "test-secret")
				require.NotContains(t, err.Error(), "sample")
				require.NotContains(t, err.Error(), "private.example")
			}
		})
	}
}

func TestEvaluateRedirectDoesNotForwardCredentials(t *testing.T) {
	var forwarded bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded = true }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	_, status, err := Evaluate(context.Background(), server.Client(), server.URL, "private-secret", Request{})
	require.Equal(t, http.StatusTemporaryRedirect, status)
	require.Error(t, err)
	require.False(t, forwarded)
}

func TestEvaluateHTTPAndTimeoutDoNotExposePayload(t *testing.T) {
	for _, status := range []int{401, 429, 500, 529} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, err := w.Write([]byte("private request text and test-secret"))
			require.NoError(t, err)
		}))
		_, code, err := Evaluate(context.Background(), server.Client(), server.URL, "test-secret", Request{})
		require.Equal(t, status, code)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "private")
		require.NotContains(t, err.Error(), "test-secret")
		server.Close()
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, _, err := Evaluate(ctx, server.Client(), server.URL, "test-secret", Request{})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.False(t, strings.Contains(err.Error(), "test-secret"))
}
