//go:build unit

package service

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGeminiCachedContentEndpointIdentityFreezesDestinationAndPrincipal(t *testing.T) {
	for _, change := range []string{"none", "official spelling", "base URL", "base path", "credential", "protocol", "missing identity"} {
		t.Run(change, func(t *testing.T) {
			a := geminiCachedContentProtocolAccount()
			record := &GeminiCachedContent{AccountID: a.ID, UpstreamName: "cachedContents/cache", UpstreamModel: "models/gemini-a"}
			freezeGeminiCachedContentProtocolBinding(t, record, a)
			require.NotContains(t, record.UpstreamEndpointIdentity, "cache-test-key")
			require.True(t, record.HasFrozenEndpoint())
			switch change {
			case "official spelling":
				a.Credentials["base_url"] = "https://generativelanguage.googleapis.com/"
			case "base URL":
				a.Credentials["base_url"] = "https://other.invalid"
				a.Credentials[GeminiExplicitCacheUpstreamCredentialKey] = true
			case "base path":
				a.Credentials["base_url"] = "https://generativelanguage.googleapis.com/other"
				a.Credentials[GeminiExplicitCacheUpstreamCredentialKey] = true
			case "credential":
				a.Credentials["api_key"] = "rotated-key"
			case "protocol":
				a.Credentials["api_protocol"] = APIProtocolChatCompletions
			case "missing identity":
				record.UpstreamEndpointIdentity = ""
			}
			want := change == "none" || change == "official spelling"
			require.Equal(t, want, record.MatchesEndpoint(a))
		})
	}
}

func TestGeminiCachedContentEndpointVertexScopeAndKeyRotation(t *testing.T) {
	for _, change := range []string{"key rotation", "principal", "credential project", "resource project", "location", "model location", "APIkey"} {
		t.Run(change, func(t *testing.T) {
			a := geminiCachedContentProtocolVertex()
			record := &GeminiCachedContent{AccountID: a.ID, UpstreamName: "projects/cache-project/locations/us-central1/cachedContents/cache",
				UpstreamModel: "projects/cache-project/locations/us-central1/publishers/google/models/gemini-a"}
			freezeGeminiCachedContentProtocolBinding(t, record, a)
			switch change {
			case "key rotation":
				a.Credentials["service_account_json"] = `{"project_id":"cache-project","client_email":"test@example.invalid","private_key":"rotated-private-key"}`
			case "principal":
				a.Credentials["service_account_json"] = `{"project_id":"cache-project","client_email":"other@example.invalid","private_key":"unused"}`
			case "credential project":
				a.Credentials["service_account_json"] = `{"project_id":"other-project","client_email":"test@example.invalid","private_key":"unused"}`
			case "resource project":
				a.Credentials["project_id"] = "other-project"
			case "location":
				a.Credentials["location"] = "global"
			case "model location":
				a.Credentials["vertex_model_locations"] = map[string]any{"gemini-a": "global"}
			case "APIkey":
				a.Credentials["auth_mode"] = vertexAuthModeAPIKey
			}
			require.Equal(t, change == "key rotation", record.MatchesEndpoint(a))
		})
	}
}

func TestGeminiCachedContentEndpointChangedNeverDispatchesMutation(t *testing.T) {
	for _, operation := range []string{"patch", "delete"} {
		t.Run(operation, func(t *testing.T) {
			a := geminiCachedContentProtocolAccount()
			record := &GeminiCachedContent{AccountID: a.ID, UpstreamName: "cachedContents/cache", UpstreamModel: "models/gemini-a"}
			freezeGeminiCachedContentProtocolBinding(t, record, a)
			a.Credentials["base_url"] = "https://new-endpoint.invalid"
			a.Credentials[GeminiExplicitCacheUpstreamCredentialKey] = true
			h := &geminiCachedContentProtocolHTTP{do: func(*http.Request, string, int64, int) (*http.Response, error) {
				t.Fatal("old cache must not use new endpoint credentials")
				return nil, nil
			}}
			svc := geminiCachedContentProtocolService(h, time.Now())
			var err error
			if operation == "patch" {
				_, err = svc.PatchUpstream(context.Background(), a, record, &GeminiCachedContentPatchRequest{TTL: time.Minute})
			} else {
				err = svc.DeleteUpstream(context.Background(), a, record)
			}
			require.ErrorIs(t, err, ErrGeminiCachedContentRequestNotSent)
			require.Zero(t, h.calls)
		})
	}
}

func TestGeminiCachedContentEndpointRejectsInvalidPersistedIdentity(t *testing.T) {
	for _, identity := range []string{"", "v1:secret", "v2:" + strings.Repeat("a", 64), "v1:" + strings.Repeat("A", 64), "v1:" + strings.Repeat("g", 64)} {
		require.False(t, (&GeminiCachedContent{UpstreamEndpointIdentity: identity}).HasFrozenEndpoint())
	}
}
