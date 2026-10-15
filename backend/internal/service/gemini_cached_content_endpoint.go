package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
)

// GeminiCachedContentEndpointIdentity freezes the cache collection and credential
// principal, not an access token. Persist only this digest, never its input. An
// API-key rotation conservatively invalidates old bindings because a new key can
// belong to another project. Service-account key rotation for the same principal
// does not invalidate a binding. URL policy still runs at dispatch time.
func GeminiCachedContentEndpointIdentity(account *Account, mappedModel string) (string, error) {
	if !IsGeminiExplicitCacheAccount(account) {
		return "", errors.New("unsupported explicit cache endpoint identity")
	}
	var base, principal string
	if account.Type == AccountTypeAPIKey {
		base = account.GetCredential("base_url")
		if isOfficialGeminiAPIBaseURL(base) {
			base = geminicli.AIStudioBaseURL
		}
		var err error
		base, err = geminiCachedContentCleanBase(base)
		if err != nil {
			return "", err
		}
		principal = strings.TrimSpace(account.GetCredential("api_key"))
	} else {
		key, err := parseVertexServiceAccountKey(account)
		if err != nil {
			return "", errors.New("invalid explicit cache service-account identity")
		}
		principal = strings.TrimSpace(key.ClientEmail) + "\x00" + strings.TrimSpace(key.ProjectID)
	}
	collection, err := geminiCachedContentCollectionURL(account, base, mappedModel)
	if err != nil {
		return "", err
	}
	// A structured encoding prevents collisions between component boundaries.
	encoded, err := json.Marshal(struct {
		Type       string
		Collection string
		Principal  string
	}{account.Type, collection, principal})
	if err != nil {
		return "", errors.New("cannot encode explicit cache endpoint identity")
	}
	digest := sha256.Sum256(encoded)
	return "v1:" + hex.EncodeToString(digest[:]), nil
}

// Legacy bindings without a frozen identity remain inaccessible; never infer
// their original endpoint from today's mutable account configuration.
func (c *GeminiCachedContent) HasFrozenEndpoint() bool {
	if c == nil || len(c.UpstreamEndpointIdentity) != 67 || !strings.HasPrefix(c.UpstreamEndpointIdentity, "v1:") || c.UpstreamEndpointIdentity != strings.ToLower(c.UpstreamEndpointIdentity) {
		return false
	}
	decoded, err := hex.DecodeString(c.UpstreamEndpointIdentity[3:])
	return err == nil && len(decoded) == sha256.Size
}

func (c *GeminiCachedContent) MatchesEndpoint(account *Account) bool {
	if !c.HasFrozenEndpoint() || account == nil || account.ID != c.AccountID {
		return false
	}
	identity, err := GeminiCachedContentEndpointIdentity(account, c.UpstreamModelID())
	return err == nil && identity == c.UpstreamEndpointIdentity
}
