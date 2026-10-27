package responseheaders

import (
	"mime"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/util/clientprivacy"
)

// defaultAllowed 定义允许透传的响应头白名单
// 注意：以下头部由 Go HTTP 包自动处理，不应手动设置：
//   - content-length: 由 ResponseWriter 根据实际写入数据自动设置
//   - transfer-encoding: 由 HTTP 库根据需要自动添加/移除
//   - connection: 由 HTTP 库管理连接复用
var defaultAllowed = map[string]struct{}{
	"content-type":                   {},
	"content-encoding":               {},
	"content-language":               {},
	"cache-control":                  {},
	"etag":                           {},
	"last-modified":                  {},
	"expires":                        {},
	"vary":                           {},
	"date":                           {},
	"x-request-id":                   {},
	"x-ratelimit-limit-requests":     {},
	"x-ratelimit-limit-tokens":       {},
	"x-ratelimit-remaining-requests": {},
	"x-ratelimit-remaining-tokens":   {},
	"x-ratelimit-reset-requests":     {},
	"x-ratelimit-reset-tokens":       {},
	"retry-after":                    {},
	// Codex uses this response header to avoid estimating reasoning tokens a
	// second time when upstream usage already includes them.
	"x-reasoning-included": {},
}

// hopByHopHeaders 是跳过的 hop-by-hop 头部，这些头部由 HTTP 库自动处理
var hopByHopHeaders = map[string]struct{}{
	"content-length":    {},
	"transfer-encoding": {},
	"connection":        {},
}

// These headers can expose upstream routing or credentials. Operators cannot
// opt them back in with AdditionalAllowed.
func privateHeader(name string) bool {
	// HTTP field names may use underscores; they must not bypass the same
	// credential boundary when configured in AdditionalAllowed.
	name = strings.ReplaceAll(name, "_", "-")
	switch name {
	case "location", "content-location", "www-authenticate", "proxy-authenticate",
		"authentication-info", "proxy-authentication-info", "authorization",
		"proxy-authorization", "cookie", "set-cookie", "set-cookie2", "key", "token":
		return true
	}
	return strings.Contains(name, "api-key") || strings.Contains(name, "apikey") ||
		strings.Contains(name, "authorization") || strings.Contains(name, "cookie") ||
		strings.Contains(name, "secret") || strings.Contains(name, "credential") ||
		strings.HasSuffix(name, "-key") || strings.HasSuffix(name, "-token")
}

type CompiledHeaderFilter struct {
	allowed     map[string]struct{}
	forceRemove map[string]struct{}
}

var defaultCompiledHeaderFilter = CompileHeaderFilter(config.ResponseHeaderConfig{})

func CompileHeaderFilter(cfg config.ResponseHeaderConfig) *CompiledHeaderFilter {
	allowed := make(map[string]struct{}, len(defaultAllowed)+len(cfg.AdditionalAllowed))
	for key := range defaultAllowed {
		allowed[key] = struct{}{}
	}
	// 关闭时只使用默认白名单，additional/force_remove 不生效
	if cfg.Enabled {
		for _, key := range cfg.AdditionalAllowed {
			normalized := strings.ToLower(strings.TrimSpace(key))
			if normalized == "" {
				continue
			}
			allowed[normalized] = struct{}{}
		}
	}

	forceRemove := map[string]struct{}{}
	if cfg.Enabled {
		forceRemove = make(map[string]struct{}, len(cfg.ForceRemove))
		for _, key := range cfg.ForceRemove {
			normalized := strings.ToLower(strings.TrimSpace(key))
			if normalized == "" {
				continue
			}
			forceRemove[normalized] = struct{}{}
		}
	}

	return &CompiledHeaderFilter{
		allowed:     allowed,
		forceRemove: forceRemove,
	}
}

func FilterHeaders(src http.Header, filter *CompiledHeaderFilter) http.Header {
	if filter == nil {
		filter = defaultCompiledHeaderFilter
	}

	filtered := make(http.Header, len(src))
	for key, values := range src {
		lower := strings.ToLower(key)
		if privateHeader(lower) {
			continue
		}
		if _, blocked := filter.forceRemove[lower]; blocked {
			continue
		}
		if _, ok := filter.allowed[lower]; !ok {
			continue
		}
		// 跳过 hop-by-hop 头部，这些由 HTTP 库自动处理
		if _, isHopByHop := hopByHopHeaders[lower]; isHopByHop {
			continue
		}
		for _, value := range values {
			// A header whose value needs redaction is optional metadata, not a
			// safe channel for a partially rewritten upstream diagnostic.
			if lower == "content-type" {
				if ContentType(value, "") == "" {
					continue
				}
			} else if !SafeValue(value) {
				continue
			}
			filtered.Add(key, value)
		}
	}
	return filtered
}

func WriteFilteredHeaders(dst http.Header, src http.Header, filter *CompiledHeaderFilter) {
	filtered := FilterHeaders(src, filter)
	for key, values := range filtered {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

// SafeValue also protects headers copied manually outside FilterHeaders.
// Opaque protocol state such as X-Codex-Turn-State has a separate path.
func SafeValue(value string) bool {
	return !strings.ContainsAny(value, "\r\n\x00") && clientprivacy.Text(value) == value
}

// ContentType protects explicit writes that otherwise restore the raw upstream
// value after header filtering. Valid, safe types retain their exact bytes.
func ContentType(value, fallback string) string {
	if value == "" || strings.ContainsAny(value, "\r\n\x00") {
		return fallback
	}
	mediaType, params, err := mime.ParseMediaType(value)
	if err != nil || !strings.Contains(mediaType, "/") {
		return fallback
	}
	// Dots in vendor media types (application/vnd.vendor+json) are protocol
	// syntax, not a hostname diagnostic. Inspect parameters separately instead
	// of applying a free-form error redactor to the entire media type.
	for name, parameter := range params {
		if privateHeader(strings.ToLower(name)) || !SafeValue(name+"="+parameter) {
			return fallback
		}
	}
	return value
}
