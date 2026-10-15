package service

import (
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
)

// Only the unadorned Google API origin inherits AI Studio's quota semantics.
// A user-info component, alternate port, path or query is not that endpoint.
func isOfficialGeminiAPIBaseURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Scheme, "https") && u.User == nil &&
		strings.EqualFold(u.Host, strings.TrimPrefix(geminicli.AIStudioBaseURL, "https://")) &&
		strings.TrimRight(u.Path, "/") == "" && u.RawPath == "" &&
		u.RawQuery == "" && !u.ForceQuery && u.Fragment == ""
}

func (a *Account) IsGeminiThirdPartyAPIKey() bool {
	return a != nil && a.Platform == PlatformGemini && a.Type == AccountTypeAPIKey &&
		!isOfficialGeminiAPIBaseURL(a.GetCredential("base_url"))
}

func (a *Account) usesGeminiProviderQuota() bool {
	return a != nil && a.Platform == PlatformGemini &&
		(a.IsGeminiThirdPartyAPIKey() || a.IsVertexServiceAccount() || a.IsGeminiOpenAIProtocol())
}
