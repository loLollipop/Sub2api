package service

import "strings"

// addDomesticModelAliases augments a newly resolved catalog, never credentials or
// a published memo. Only recognized provider/family pairs are eligible. Targets
// remain byte-for-byte identical so routing and upstream billing are unchanged.
func addDomesticModelAliases(mapping map[string]string) {
	aliases := make(map[string]string)
	ambiguous := make(map[string]bool)
	for model, target := range mapping {
		provider, family, ok := strings.Cut(model, "/")
		if !ok || strings.ContainsAny(family, "/*") {
			continue
		}
		prefix := ""
		switch strings.ToLower(provider) {
		case "deepseek-ai", "deepseek":
			prefix = "deepseek-"
		case "moonshotai":
			prefix = "kimi-"
		case "z-ai", "zhipuai":
			prefix = "glm-"
		case "minimaxai":
			prefix = "minimax-"
		case "qwen":
			prefix = "qwen"
		case "xiaomi", "xiaomimimo":
			prefix = "mimo-"
		case "stepfun-ai":
			prefix = "step-"
		}
		if prefix == "" || !strings.HasPrefix(strings.ToLower(family), prefix) || len(family) <= len(prefix) {
			continue
		}
		if _, explicit := mapping[family]; explicit {
			continue
		}
		if _, exists := aliases[family]; exists {
			ambiguous[family] = true
		}
		aliases[family] = target
	}
	for alias, target := range aliases {
		if !ambiguous[alias] {
			mapping[alias] = target
		}
	}
}
