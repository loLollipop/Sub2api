package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// IsMixed identifies the isolated API-key platform. It must never imply
// IsOpenAI, IsCNProvider, OAuth, passthrough, or provider-specific discovery.
func (a *Account) IsMixed() bool { return a != nil && a.Platform == PlatformMixed }

// Strip only known wire endpoints. Preserve an explicit vendor prefix/version
// so every protocol attempt starts from the same configured origin and base.
func normalizeMixedBaseURL(base string) string {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return strings.TrimSpace(base)
	}
	path := strings.TrimRight(u.Path, "/")
	for _, endpoint := range []string{"/chat/completions", "/responses", "/messages"} {
		if strings.HasSuffix(path, endpoint) {
			u.Path = strings.TrimSuffix(path, endpoint)
			u.RawPath = ""
			break
		}
	}
	return u.String()
}

func validateMixedAccount(account *Account) error {
	if !account.IsMixed() {
		return nil
	}
	if account.Type != AccountTypeAPIKey {
		return infraerrors.BadRequest("INVALID_MIXED_ACCOUNT", "Mixed supports API-key accounts only")
	}
	if strings.TrimSpace(account.GetCredential("api_key")) == "" {
		return infraerrors.BadRequest("INVALID_MIXED_ACCOUNT", "Mixed requires an explicit api_key")
	}
	base, err := url.Parse(strings.TrimSpace(account.GetCredential("base_url")))
	if err != nil || base.Hostname() == "" || (base.Scheme != "https" && base.Scheme != "http") || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return infraerrors.BadRequest("INVALID_MIXED_ACCOUNT", "Mixed requires an explicit HTTP(S) base_url without user info, query, or fragment")
	}
	return nil
}

// Validate only the new boundary: old platform-to-platform binding rules are
// unchanged. Composite groups may contain Mixed accounts without acquiring
// the Mixed group's implicit Messages/conversion permissions.
func validateMixedGroupBindings(ctx context.Context, repo GroupRepository, account *Account, groupIDs []int64) error {
	for _, id := range groupIDs {
		if repo == nil {
			return fmt.Errorf("group repository not configured")
		}
		group, err := repo.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if group == nil {
			return ErrGroupNotFound
		}
		if (group.Platform == PlatformMixed && (!account.IsMixed() || account.Type != AccountTypeAPIKey)) ||
			(account.IsMixed() && group.Platform != PlatformMixed && group.Platform != PlatformComposite) {
			return infraerrors.BadRequest("MIXED_PLATFORM_BINDING_MISMATCH", "Mixed accounts require a Mixed or Composite group; Mixed groups accept only Mixed API-key accounts")
		}
	}
	return nil
}
