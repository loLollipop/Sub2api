package service

import (
	"net/url"
	"strings"
)

// These are also existing presets in the original MiniMax import form.
const mixedMiniMaxInternationalBaseURL = "https://api.minimax.io/v1"
const mixedMiniMaxInternationalAnthropicBaseURL = "https://api.minimax.io/anthropic"

// The import entry does not define a new provider. Recognized first-party
// endpoints reuse the existing provider's protocol/URL helpers while the real
// account stays Mixed for key scope, scheduling, metering and error handling.
// A relay is never identified by its model name or a hostname substring.
func mixedOfficialProviderView(account *Account) *Account {
	if !account.IsMixed() || account.Type != AccountTypeAPIKey {
		return nil
	}
	configured := strings.TrimSpace(account.GetCredential("base_url"))
	base, err := url.Parse(normalizeMixedBaseURL(configured))
	if err != nil || !strings.EqualFold(base.Scheme, "https") || base.User != nil ||
		base.RawQuery != "" || base.ForceQuery || base.Fragment != "" ||
		(base.Port() != "" && base.Port() != "443") || base.RawPath != "" {
		return nil
	}
	path := strings.TrimSuffix(strings.TrimRight(base.Path, "/"), "/v1")
	for _, preset := range []struct{ platform, mode, base string }{
		{PlatformDeepseek, AccountModePayG, DefaultDeepseekBaseURL},
		{PlatformDeepseek, AccountModePayG, DefaultDeepseekAnthropicBaseURL},
		{PlatformKimi, AccountModePayG, DefaultKimiPayGBaseURL},
		{PlatformKimi, AccountModePayG, DefaultKimiPayGAnthropicBaseURL},
		{PlatformKimi, AccountModeCoding, DefaultKimiCodingBaseURL},
		{PlatformKimi, AccountModeCoding, DefaultKimiCodingAnthropicBaseURL},
		{PlatformZhipu, AccountModePayG, DefaultZhipuPayGBaseURL},
		{PlatformZhipu, AccountModeCoding, DefaultZhipuCodingBaseURL},
		{PlatformZhipu, AccountModePayG, DefaultZhipuAnthropicBaseURL},
		{PlatformMiniMax, AccountModePayG, DefaultMiniMaxBaseURL},
		{PlatformMiniMax, AccountModePayG, DefaultMiniMaxAnthropicBaseURL},
		{PlatformMiniMax, AccountModePayG, mixedMiniMaxInternationalBaseURL},
		{PlatformMiniMax, AccountModePayG, mixedMiniMaxInternationalAnthropicBaseURL},
	} {
		target, _ := url.Parse(preset.base)
		presetPath := strings.TrimSuffix(strings.TrimRight(target.Path, "/"), "/v1")
		if !strings.EqualFold(base.Hostname(), target.Hostname()) || path != presetPath {
			continue
		}
		view := cloneSnapshotAccount(account)
		view.Platform = preset.platform
		if view.GetCredential("account_mode") == "" {
			view.Credentials["account_mode"] = preset.mode
		}
		if view.GetCredential("api_protocol") == "" {
			view.Credentials["api_protocol"] = APIProtocolAdaptive
		}
		if view.IsAdaptiveAPIProtocol() {
			bases, _ := view.Credentials["api_base_urls"].(map[string]any)
			if bases == nil {
				bases = make(map[string]any, 3)
			}
			for _, protocol := range []string{APIProtocolChatCompletions, APIProtocolResponses, APIProtocolAnthropic} {
				if value, _ := bases[protocol].(string); strings.TrimSpace(value) == "" {
					bases[protocol] = mixedOfficialDefaultURL(account, view.defaultCNProtocolBaseURL(protocol))
				}
			}
			view.Credentials["api_base_urls"] = bases
		}
		// Existing native helpers supply provider-specific paths, never a new
		// host. Reject native classification if stored protocol URLs would move
		// this Mixed credential to another origin; generic routing still enforces
		// the same-origin rule for explicitly configured relay endpoints.
		for _, protocol := range []string{APIProtocolChatCompletions, APIProtocolResponses, APIProtocolAnthropic} {
			values := []string{mixedOfficialDefaultURL(account, view.GetCNProtocolBaseURL(protocol))}
			if bases, ok := view.Credentials["api_base_urls"].(map[string]any); ok {
				if explicit, _ := bases[protocol].(string); strings.TrimSpace(explicit) != "" {
					values = append(values, explicit)
				}
			}
			for _, value := range values {
				candidate, err := url.Parse(value)
				if err != nil || candidate.User != nil || candidate.RawQuery != "" || candidate.ForceQuery || candidate.Fragment != "" ||
					!strings.EqualFold(candidate.Scheme, base.Scheme) || !sameMixedOfficialOrigin(base, candidate) {
					return nil
				}
			}
		}
		return &view
	}
	return nil
}

// MiniMax's original form has domestic and international presets, whereas its
// backend default constants are domestic. Derive paths from those same defaults
// while retaining the explicitly selected international credential origin.
func mixedOfficialDefaultURL(account *Account, value string) string {
	base, err := url.Parse(account.GetCredential("base_url"))
	if err != nil || !strings.EqualFold(base.Hostname(), "api.minimax.io") {
		return value
	}
	if value != DefaultMiniMaxBaseURL && value != DefaultMiniMaxAnthropicBaseURL {
		return value
	}
	target, _ := url.Parse(value)
	target.Host = base.Host
	return target.String()
}

func sameMixedOfficialOrigin(a, b *url.URL) bool {
	port := func(u *url.URL) string {
		if p := u.Port(); p != "" {
			return p
		}
		return "443"
	}
	return strings.EqualFold(a.Hostname(), b.Hostname()) && port(a) == port(b)
}

// Only protocol details use the underlying first-party platform. Never use
// this as a replacement account platform for billing or group permissions.
func (a *Account) openAIWirePlatform() string {
	if a == nil {
		return ""
	}
	if native := mixedOfficialProviderView(a); native != nil {
		return native.Platform
	}
	return a.Platform
}
