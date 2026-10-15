package service

import (
	"fmt"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"net/http"
	"slices"
	"strings"
)

const (
	protocolRulesCredentialKey   = "protocol_rules"
	maxProtocolRules             = 64
	maxProtocolRulePatternLength = 128
)

// ProtocolRule is one model-pattern → native protocol mapping.
// Pattern is an exact ID or a suffix glob (foo* / *). First match wins.
type ProtocolRule struct {
	Pattern  string `json:"pattern"`
	Protocol string `json:"protocol"`
	// Protocols 是命中模型支持的全部原生协议，首项即 Protocol（首选）；为空表示只支持
	// Protocol。入站协议在其中时同协议直通，否则走首选协议（见 resolveModelRoutedProtocolFor）。
	Protocols []string `json:"protocols,omitempty"`
}

// normalizeProtocolSet 返回以 preferred 开头、去重且只含原生协议的协议集合。
func normalizeProtocolSet(preferred string, protocols []string) []string {
	set := []string{preferred}
	for _, protocol := range protocols {
		if !isNativeUpstreamProtocol(protocol) || slices.Contains(set, protocol) {
			continue
		}
		set = append(set, protocol)
	}
	return set
}

func isNativeUpstreamProtocol(protocol string) bool {
	switch protocol {
	case APIProtocolChatCompletions, APIProtocolAnthropic, APIProtocolResponses:
		return true
	default:
		return false
	}
}

func protocolRulePatternMatches(pattern, model string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	model = strings.ToLower(strings.TrimSpace(model))
	if pattern == "" || model == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(model, strings.TrimSuffix(pattern, "*"))
	}
	return model == pattern
}

// matchProtocolRules 返回首条命中规则的首选协议；未命中时为 Chat Completions。
func matchProtocolRules(model string, rules []ProtocolRule) string {
	if set, ok := matchProtocolRuleSet(model, rules); ok {
		return set[0]
	}
	return APIProtocolChatCompletions
}

// matchProtocolRuleSet 返回首条命中规则支持的协议集合（首项为首选）；未命中时 ok=false。
func matchProtocolRuleSet(model string, rules []ProtocolRule) ([]string, bool) {
	for _, rule := range rules {
		if !isNativeUpstreamProtocol(rule.Protocol) {
			continue
		}
		if protocolRulePatternMatches(rule.Pattern, model) {
			return normalizeProtocolSet(rule.Protocol, rule.Protocols), true
		}
	}
	return nil, false
}

func (a *Account) configuredProtocolRules() ([]ProtocolRule, bool) {
	if a == nil || a.Credentials == nil {
		return nil, false
	}
	raw, ok := a.Credentials[protocolRulesCredentialKey]
	if !ok || raw == nil {
		return nil, false
	}
	rules, err := parseProtocolRules(raw)
	if err != nil {
		return nil, false
	}
	return rules, true
}

func parseProtocolRules(raw any) ([]ProtocolRule, error) {
	items, err := protocolRuleItems(raw)
	if err != nil {
		return nil, err
	}
	if len(items) > maxProtocolRules {
		return nil, fmt.Errorf("protocol_rules supports at most %d entries", maxProtocolRules)
	}
	rules := make([]ProtocolRule, 0, len(items))
	for i, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("protocol_rules[%d] must be an object", i)
		}
		pattern, _ := entry["pattern"].(string)
		protocol, _ := entry["protocol"].(string)
		pattern, err := normalizeProtocolRulePattern(pattern)
		if err != nil {
			return nil, fmt.Errorf("protocol_rules[%d]: %w", i, err)
		}
		protocols, err := protocolRuleProtocols(entry["protocols"])
		if err != nil {
			return nil, fmt.Errorf("protocol_rules[%d]: %w", i, err)
		}
		protocol = strings.TrimSpace(protocol)
		if protocol == "" && len(protocols) > 0 {
			protocol = protocols[0]
		}
		if !isNativeUpstreamProtocol(protocol) {
			return nil, fmt.Errorf("protocol_rules[%d]: protocol must be chat_completions, anthropic, or responses", i)
		}
		rule := ProtocolRule{Pattern: pattern, Protocol: protocol}
		if set := normalizeProtocolSet(protocol, protocols); len(set) > 1 {
			rule.Protocols = set
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// protocolRuleProtocols 解析规则的 protocols 列表（可缺省）；其中每一项都须为原生协议。
func protocolRuleProtocols(raw any) ([]string, error) {
	if raw == nil {
		return nil, nil
	}
	var items []any
	switch values := raw.(type) {
	case []any:
		items = values
	case []string:
		for _, value := range values {
			items = append(items, value)
		}
	default:
		return nil, fmt.Errorf("protocols must be an array")
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		protocol, _ := item.(string)
		protocol = strings.TrimSpace(protocol)
		if !isNativeUpstreamProtocol(protocol) {
			return nil, fmt.Errorf("protocols must only contain chat_completions, anthropic, or responses")
		}
		out = append(out, protocol)
	}
	return out, nil
}

func protocolRuleItems(raw any) ([]any, error) {
	switch items := raw.(type) {
	case []any:
		return items, nil
	case []map[string]any:
		out := make([]any, 0, len(items))
		for _, item := range items {
			out = append(out, item)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("protocol_rules must be an array")
	}
}

func normalizeProtocolRulePattern(pattern string) (string, error) {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}
	if len(pattern) > maxProtocolRulePatternLength {
		return "", fmt.Errorf("pattern is too long")
	}
	if strings.ContainsAny(pattern, " \t") {
		return "", fmt.Errorf("pattern must not contain whitespace")
	}
	star := strings.Count(pattern, "*")
	if star > 1 || (star == 1 && !strings.HasSuffix(pattern, "*")) {
		return "", fmt.Errorf("pattern may use a single trailing * wildcard")
	}
	return pattern, nil
}

// NormalizeProtocolRulesCredentials 校验并原地规范化 credentials.protocol_rules。
// 未携带该字段时为 no-op，使旧账号继续使用内置默认表。
func NormalizeProtocolRulesCredentials(credentials map[string]any) error {
	if credentials == nil {
		return nil
	}
	raw, ok := credentials[protocolRulesCredentialKey]
	if !ok || raw == nil {
		return nil
	}
	rules, err := parseProtocolRules(raw)
	if err != nil {
		return infraerrors.New(http.StatusBadRequest, "INVALID_PROVIDER_PROTOCOL_RULES", err.Error())
	}
	encoded := make([]any, 0, len(rules))
	for _, rule := range rules {
		item := map[string]any{
			"pattern":  rule.Pattern,
			"protocol": rule.Protocol,
		}
		if len(rule.Protocols) > 1 {
			protocols := make([]any, 0, len(rule.Protocols))
			for _, protocol := range rule.Protocols {
				protocols = append(protocols, protocol)
			}
			item["protocols"] = protocols
		}
		encoded = append(encoded, item)
	}
	credentials[protocolRulesCredentialKey] = encoded
	return nil
}
