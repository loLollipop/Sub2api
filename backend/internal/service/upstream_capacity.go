package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

// IsUpstreamWAFBody reports Cloudflare / browser-integrity blocks. These are
// request-identity failures, not supplier capacity cooling. Valid JSON is
// inspected only on error fields so echoed request text cannot trip the
// matcher.
func IsUpstreamWAFBody(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	if isCloudflareBotBlockResponse(body) {
		return true
	}

	for _, path := range []string{
		"error.message",
		"response.error.message",
		"message",
		"error.code",
		"response.error.code",
		"code",
	} {
		if isUpstreamWAFMessage(gjson.GetBytes(body, path).String()) {
			return true
		}
	}
	if !gjson.ValidBytes(body) {
		return isUpstreamWAFMessage(string(body))
	}
	return false
}

// isCloudflareBotBlockResponse identifies the specific Cloudflare 1010 block
// across platforms without broadening generic WAF exemptions.
func isCloudflareBotBlockResponse(body []byte) bool {
	// Inspect only the error envelope, never echoed request/tool parameters.
	for _, path := range []string{"@this", "error", "response.error"} {
		node := gjson.GetBytes(body, path)
		if !node.IsObject() {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(node.Get("error_name").String()), "browser_signature_banned") {
			return true
		}
		if node.Get("cloudflare_error").Type == gjson.True {
			for _, key := range []string{"error_code", "code"} {
				code := node.Get(key)
				if (code.Type == gjson.Number && code.Num == 1010) ||
					(code.Type == gjson.String && strings.TrimSpace(code.Str) == "1010") {
					return true
				}
			}
		}
	}
	for _, path := range []string{"error.message", "response.error.message", "message", "error.code", "response.error.code", "code"} {
		if isCloudflare1010Message(gjson.GetBytes(body, path).String()) {
			return true
		}
	}
	return !gjson.ValidBytes(body) && isCloudflare1010Message(string(body))
}

func isCloudflare1010Message(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "error code: 1010") || strings.Contains(lower, "error 1010")
}

func isUpstreamWAFMessage(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}
	for _, marker := range []string{
		"error code: 1010",
		"error 1010",
		"your request was blocked",
		"request was blocked",
		"window._cf_chl_opt",
		"just a moment",
		"challenge-platform",
		"__cf_chl_",
		"cf-mitigated",
		"attention required! | cloudflare",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// IsUpstreamCapacityCoolingBody reports request-scoped upstream capacity
// failures that should be presented as retryable service unavailability rather
// than as a credential/access failure. Several OpenAI-compatible providers use
// HTTP 403 or a FORBIDDEN code for a group whose suppliers are cooling down.
func IsUpstreamCapacityCoolingBody(body []byte) bool {
	if IsUpstreamWAFBody(body) {
		return false
	}
	if len(body) == 0 {
		return false
	}
	for _, path := range []string{
		"error.message",
		"response.error.message",
		"message",
		"error.code",
		"response.error.code",
		"code",
	} {
		if isUpstreamCapacityCoolingMessage(gjson.GetBytes(body, path).String()) {
			return true
		}
	}
	// Non-JSON providers commonly return a plain-text capacity response.
	if !gjson.ValidBytes(body) {
		return isUpstreamCapacityCoolingMessage(string(body))
	}
	return false
}

func isUpstreamCapacityCoolingMessage(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}
	if isOpenAICapacityShedMessage(lower) {
		return true
	}
	for _, marker := range []string{
		"cooling",
		"all candidates failed",
		"候选供应商均请求失败",
		"支持该模型的货源均",
		"货源均在冷却中",
		"稍后重试",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return strings.Contains(lower, "all providers") && strings.Contains(lower, "failed")
}
