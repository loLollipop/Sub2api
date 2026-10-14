package service

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	ModelScopeAll       = "all"
	ModelScopeOpenAI    = "openai"
	ModelScopeAnthropic = "anthropic"
	ModelScopeCN        = "cn"
	ModelScopeOther     = "other"
)

// Includes the domestic families already offered by the account model picker.
// Exact numbered aliases are separate so k1/k2 and m2 cannot match arbitrary
// unrelated names; pricing vendors share this same classifier.
var cnModelScopeFamilies = []string{"qwen", "qwq", "glm", "chatglm", "cogview", "cogvideo", "deepseek", "kimi", "moonshot", "minimax", "mimo", "step", "abab", "doubao", "yi", "ernie", "spark", "hunyuan"}

func bareCNModelAliasVendor(model string) string {
	if model == "k3" || model == "k3-256k" || modelFamily(model, "k2") || modelFamily(model, "k1.5") {
		return "kimi"
	}
	if strings.HasPrefix(model, "m2.") {
		return "minimax"
	}
	return ""
}

var openAIModelScopeFamilies = []string{"gpt", "chatgpt", "codex", "o1", "o3", "o4", "o5", "text-embedding", "text-moderation", "omni-moderation", "dall-e", "tts", "whisper", "babbage", "davinci"}

var ErrInvalidModelScope = infraerrors.BadRequest("INVALID_MODEL_SCOPE", "model_scope must be all, openai, anthropic, cn or other")
var ErrModelScopeDenied = infraerrors.Forbidden("MODEL_SCOPE_DENIED", "Model is not available for this API key provider category")

// Only the legacy empty value is normalized. Unknown persisted values must fail
// closed, including values written by a newer binary during a rolling upgrade.
func NormalizeModelScope(scope string) string {
	if scope == "" {
		return ModelScopeAll
	}
	return scope
}

func ValidModelScope(scope string) bool {
	switch NormalizeModelScope(scope) {
	case ModelScopeAll, ModelScopeOpenAI, ModelScopeAnthropic, ModelScopeCN, ModelScopeOther:
		return true
	}
	return false
}

// ClassifyModelScope classifies names without changing the routed model or its
// pricing identity. A namespace is trusted only for its own model families.
func ClassifyModelScope(model string) string {
	model = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(model)), "models/")
	if bareCNModelAliasVendor(model) != "" {
		return ModelScopeCN
	}
	// Canonical Bedrock Claude invocation IDs retain their provider namespace.
	for _, prefix := range []string{"anthropic.", "us.anthropic.", "eu.anthropic.", "apac.anthropic.", "global.anthropic."} {
		if strings.HasPrefix(model, prefix+"claude-") {
			return ModelScopeAnthropic
		}
	}
	if namespace, family, ok := strings.Cut(model, "/"); ok {
		if strings.Contains(family, "/") {
			return ModelScopeOther
		}
		allowed := map[string][]string{
			"openai":    openAIModelScopeFamilies,
			"chatgpt":   openAIModelScopeFamilies,
			"anthropic": {"claude", "opus", "sonnet", "haiku"},
			"qwen":      {"qwen", "qwq"}, "z-ai": {"glm", "chatglm", "cogview", "cogvideo"}, "zhipuai": {"glm", "chatglm", "cogview", "cogvideo"}, "zhipu": {"glm", "chatglm", "cogview", "cogvideo"},
			"deepseek-ai": {"deepseek"}, "deepseek": {"deepseek"},
			"moonshotai": {"kimi", "moonshot"}, "moonshot": {"kimi", "moonshot"},
			"kimi": {"kimi", "moonshot"}, "glm": {"glm", "chatglm", "cogview", "cogvideo"}, "bigmodel": {"glm", "chatglm", "cogview", "cogvideo"}, "thudm": {"glm", "chatglm", "cogview", "cogvideo"},
			"minimaxai": {"minimax"}, "minimax": {"minimax"},
			"xiaomi": {"mimo"}, "xiaomimimo": {"mimo"}, "stepfun-ai": {"step"}, "stepfun": {"step"},
			"bytedance": {"doubao"}, "doubao": {"doubao"}, "volcengine": {"doubao"},
			"01-ai": {"yi"}, "01ai": {"yi"}, "baidu": {"ernie"}, "iflytek": {"spark"}, "tencent": {"hunyuan"},
		}
		matched := false
		for _, prefix := range allowed[namespace] {
			if modelFamily(family, prefix) {
				matched = true
				break
			}
		}
		if !matched {
			if _, known := allowed[namespace]; known {
				return ""
			}
			return ModelScopeOther
		}
		model = family
	}
	for _, family := range openAIModelScopeFamilies {
		if modelFamily(model, family) {
			return ModelScopeOpenAI
		}
	}
	for _, family := range []string{"claude", "opus", "sonnet", "haiku"} {
		if modelFamily(model, family) {
			return ModelScopeAnthropic
		}
	}
	for _, family := range cnModelScopeFamilies {
		if modelFamily(model, family) {
			return ModelScopeCN
		}
	}
	return ModelScopeOther
}

func modelFamily(model, family string) bool {
	if model == family {
		return true
	}
	if !strings.HasPrefix(model, family) || len(model) <= len(family) {
		return false
	}
	c := model[len(family)]
	// o1/o3/o4/o5 are complete numbered families: o10 and o31 are not o1/o3.
	return c == '-' || c == '_' || c == '.' || (family[0] != 'o' && c >= '0' && c <= '9')
}

func ModelScopeAllows(scope, model string) bool {
	scope = NormalizeModelScope(scope)
	if scope == ModelScopeAll {
		return true
	}
	return ValidModelScope(scope) && strings.TrimSpace(model) != "" &&
		!strings.ContainsAny(model, "*?") && ClassifyModelScope(model) == scope
}

func (k *APIKey) AllowsModelScope(model string) bool {
	return k == nil || ModelScopeAllows(k.ModelScope, model)
}

func ContextWithModelScope(ctx context.Context, scope string) context.Context {
	return context.WithValue(ctx, ctxkey.APIKeyModelScope, NormalizeModelScope(scope))
}

func ModelScopeFromContext(ctx context.Context) string {
	if ctx == nil {
		return ModelScopeAll
	}
	scope, _ := ctx.Value(ctxkey.APIKeyModelScope).(string)
	return NormalizeModelScope(scope)
}

func CheckModelScope(ctx context.Context, models ...string) error {
	for _, model := range models {
		if !ModelScopeAllows(ModelScopeFromContext(ctx), model) {
			return ErrModelScopeDenied
		}
	}
	return nil
}

// FilterModelScope always creates a private slice: callers may share their
// input with the group catalogue cache and other keys.
func FilterModelScope(scope string, models []string) []string {
	result := make([]string, 0, len(models))
	for _, model := range models {
		if ModelScopeAllows(scope, model) {
			result = append(result, model)
		}
	}
	return result
}
