package service

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/typesafe"
	"strings"
)

func (a *Account) IsTypeSafe() bool { return a != nil && a.Platform == PlatformTypeSafe }

func (a *Account) GetTypeSafeBaseURL() string {
	if !a.IsTypeSafe() || a.Type != AccountTypeAPIKey {
		return ""
	}
	base := strings.TrimRight(strings.TrimSpace(a.GetCredential("base_url")), "/")
	if base == "" {
		return typesafe.DefaultBaseURL
	}
	if strings.HasSuffix(strings.ToLower(base), "/v1") {
		base = base[:len(base)-3]
	}
	if base == "" {
		return typesafe.DefaultBaseURL
	}
	return base
}

func (a *Account) GetTypeSafeAPIKey() string {
	if !a.IsTypeSafe() || a.Type != AccountTypeAPIKey {
		return ""
	}
	return strings.TrimSpace(a.GetCredential("api_key"))
}
