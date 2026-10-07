package service

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// NormalizePurchaseSubscriptionProducts returns an independent map of configured
// fixed CNY tiers. Blank values remove a tier; the legacy URL is never a fallback.
func NormalizePurchaseSubscriptionProducts(products map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(products))
	if len(products) > 5 {
		return nil, fmt.Errorf("at most five product URLs are allowed")
	}
	for tier, raw := range products {
		switch tier {
		case "10", "20", "30", "50", "100":
		default:
			return nil, fmt.Errorf("unsupported product tier %q", tier)
		}
		for _, r := range raw {
			if unicode.IsControl(r) || r == '\\' {
				return nil, fmt.Errorf("product %s URL contains invalid characters", tier)
			}
		}
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !validPurchaseProductURL(raw) {
			return nil, fmt.Errorf("product %s requires an absolute http(s) URL without credentials and with a valid host and port (max 2048 characters)", tier)
		}
		out[tier] = raw
	}
	return out, nil
}

func validPurchaseProductURL(raw string) bool {
	if len(raw) > 2048 || strings.IndexFunc(raw, unicode.IsSpace) >= 0 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Opaque != "" {
		return false
	}
	// Product fragments are allowed and preserved; the shared validator disallows
	// fragments for redirect URLs, so validate the rest of the absolute URL.
	if config.ValidateAbsoluteHTTPURL(strings.SplitN(raw, "#", 2)[0]) != nil {
		return false
	}
	host := u.Hostname()
	if host == "" || strings.HasSuffix(u.Host, ":") {
		return false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	if strings.ContainsAny(u.Host, "[]:") && (strings.Contains(host, ":") || strings.HasPrefix(u.Host, "[")) {
		return strings.HasPrefix(u.Host, "[") && strings.Contains(host, ":") && net.ParseIP(host) != nil
	}
	if net.ParseIP(host) != nil {
		return true
	}
	host = strings.TrimSuffix(host, ".")
	if len(host) > 253 || strings.Trim(host, "0123456789.") == "" {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' {
				return false
			}
		}
	}
	return true
}

func parsePurchaseSubscriptionProducts(raw string) map[string]string {
	var products map[string]string
	if json.Unmarshal([]byte(raw), &products) != nil {
		return map[string]string{}
	}
	normalized, err := NormalizePurchaseSubscriptionProducts(products)
	if err != nil {
		return map[string]string{}
	}
	return normalized
}
