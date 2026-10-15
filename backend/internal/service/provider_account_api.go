package service

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// Probes must use the official HTTPS origin, never a credential-bearing URL,
// alternate port, query or fragment configured by a relay.
func isOfficialProviderOrigin(u *url.URL, host string) bool {
	return u != nil && strings.EqualFold(u.Scheme, "https") && strings.EqualFold(u.Hostname(), host) &&
		u.User == nil && (u.Port() == "" || u.Port() == "443") && u.RawQuery == "" && !u.ForceQuery &&
		u.Fragment == "" && u.Opaque == "" && u.RawPath == ""
}

func isOfficialCommandCodeHost(target string) bool {
	u, err := url.Parse(strings.TrimSpace(target))
	return err == nil && isOfficialProviderOrigin(u, "api.commandcode.ai")
}

func (a *Account) providerAccountAPIOriginAllowed(host string) bool {
	values := []string{a.GetOpenAIBaseURL()}
	if base := strings.TrimSpace(a.GetCredential("base_url")); base != "" {
		values = append(values, base)
	}
	if bases, ok := a.Credentials["api_base_urls"].(map[string]any); ok {
		for _, raw := range bases {
			if base, ok := raw.(string); ok && strings.TrimSpace(base) != "" {
				values = append(values, base)
			}
		}
	}
	for _, base := range values {
		u, err := url.Parse(strings.TrimSpace(base))
		if err != nil || !isOfficialProviderOrigin(u, host) {
			return false
		}
	}
	return true
}

func cnBalanceCheckCooldown(cfg *config.Config) time.Duration {
	minutes := 10
	if cfg != nil && cfg.Gateway.CNProviders.BalanceCheckIntervalMinutes > 0 {
		minutes = cfg.Gateway.CNProviders.BalanceCheckIntervalMinutes
	}
	return time.Duration(minutes) * 2 * time.Minute
}

var providerUsageLimitResetPattern = regexp.MustCompile(`(?i)\bresets\s+in\s+`)
var providerUsageLimitDurationPartPattern = regexp.MustCompile(`(?i)^([0-9]+(?:\.[0-9]+)?)\s*(s|sec|secs|second|seconds|m|min|mins|minute|minutes|h|hr|hrs|hour|hours|d|day|days|w|week|weeks)\b`)

func parseProviderUsageLimitResetDuration(message string) time.Duration {
	resetPrefix := providerUsageLimitResetPattern.FindStringIndex(message)
	if resetPrefix == nil {
		return 0
	}

	remainder := message[resetPrefix[1]:]
	var total time.Duration
	for {
		remainder = strings.TrimSpace(remainder)
		matches := providerUsageLimitDurationPartPattern.FindStringSubmatchIndex(remainder)
		if matches == nil {
			break
		}

		value, err := strconv.ParseFloat(remainder[matches[2]:matches[3]], 64)
		if err != nil || value <= 0 {
			return 0
		}

		unit := providerUsageLimitDurationUnit(remainder[matches[4]:matches[5]])
		if unit <= 0 {
			return 0
		}

		const maxDuration = time.Duration(1<<63 - 1)
		if value >= float64(maxDuration)/float64(unit) {
			return 0
		}
		part := time.Duration(value * float64(unit))
		if part <= 0 || total > maxDuration-part {
			return 0
		}
		total += part
		remainder = remainder[matches[1]:]
	}

	return total
}

func providerUsageLimitDurationUnit(raw string) time.Duration {
	switch strings.ToLower(raw) {
	case "s", "sec", "secs", "second", "seconds":
		return time.Second
	case "m", "min", "mins", "minute", "minutes":
		return time.Minute
	case "h", "hr", "hrs", "hour", "hours":
		return time.Hour
	case "d", "day", "days":
		return 24 * time.Hour
	case "w", "week", "weeks":
		return 7 * 24 * time.Hour
	default:
		return 0
	}
}
