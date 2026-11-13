package service

import (
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/urljoin"
)

// buildAnthropicMessagesURL joins the Messages endpoint to an already validated
// base URL. Cloudflare's account REST endpoint rejects the beta query parameter;
// all other upstreams, including Cloudflare's native gateway, keep it.
func buildAnthropicMessagesURL(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return strings.TrimRight(baseURL, "/") + "/v1/messages?beta=true"
	}
	original := parsed
	parsed, err = url.Parse(urljoin.APIPath(baseURL, "/v1/messages"))
	if err != nil {
		return strings.TrimRight(baseURL, "/") + "/v1/messages?beta=true"
	}
	omitBeta := false
	if strings.EqualFold(parsed.Scheme, "https") && parsed.User == nil &&
		original.Fragment == "" && original.RawPath == "" {
		host := strings.ToLower(parsed.Host)
		parts := strings.Split(parsed.Path, "/")
		if (host == "api.cloudflare.com" || host == "api.cloudflare.com:443") &&
			len(parts) == 8 && parts[0] == "" && parts[1] == "client" && parts[2] == "v4" &&
			parts[3] == "accounts" && parts[4] != "" && parts[4] != "." && parts[4] != ".." &&
			parts[5] == "ai" && parts[6] == "v1" && parts[7] == "messages" {
			omitBeta = true
		}
	}
	// Preserve explicit beta values, including an empty value, and other query
	// parameters. Only the implicit beta=true is omitted for the REST endpoint.
	if !omitBeta && !parsed.Query().Has("beta") {
		if parsed.RawQuery != "" {
			parsed.RawQuery += "&"
		}
		parsed.RawQuery += "beta=true"
	}
	return parsed.String()
}

func buildAnthropicCountTokensURL(baseURL string) string {
	return urljoin.WithQueryParam(urljoin.APIPath(baseURL, "/v1/messages/count_tokens"), "beta", "true")
}
