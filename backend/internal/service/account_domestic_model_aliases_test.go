package service

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDomesticModelAliasesPreserveCatalogAndTargets(t *testing.T) {
	raw := map[string]any{
		"deepseek-ai/deepseek-v4-pro-0813": "upstream/actual-model",
		"moonshotai/kimi-k2.6":             "moonshotai/kimi-k2.6",
		"z-ai/glm-5.3":                     "z-ai/glm-5.3",
		"MiniMaxAI/MiniMax-M3":             "MiniMaxAI/MiniMax-M3",
		"Qwen/Qwen3.8-27B-FP8":             "Qwen/Qwen3.8-27B-FP8",
		"xiaomi/mimo-v2-flash":             "xiaomi/mimo-v2-flash",
		"stepfun-ai/step-3.5-flash":        "stepfun-ai/step-3.5-flash",
		"custom-alias":                     "deepseek-ai/deepseek-hidden",
		"unknown/glm-5":                    "unknown/glm-5",
		"deepseek-ai/gpt-5":                "deepseek-ai/gpt-5",
		"z-ai/glm-*":                       "wildcard",
	}
	a := &Account{Platform: PlatformMixed, Type: AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": raw}}
	mapping := a.GetModelMapping()
	for key, target := range raw {
		require.Equal(t, target, mapping[key])
	}
	for alias, target := range map[string]string{
		"deepseek-v4-pro-0813": "upstream/actual-model", "kimi-k2.6": "moonshotai/kimi-k2.6",
		"glm-5.3": "z-ai/glm-5.3", "MiniMax-M3": "MiniMaxAI/MiniMax-M3",
		"Qwen3.8-27B-FP8": "Qwen/Qwen3.8-27B-FP8", "mimo-v2-flash": "xiaomi/mimo-v2-flash",
		"step-3.5-flash": "stepfun-ai/step-3.5-flash",
	} {
		require.Equal(t, target, a.GetMappedModel(alias))
		require.True(t, a.IsModelSupported(alias))
		require.NotContains(t, raw, alias, "derived aliases must not mutate persisted credentials")
	}
	for _, excluded := range []string{"deepseek-hidden", "glm-5", "gpt-5", "glm-*", "qwen3.8-27b-fp8", "deepseek-v4-pro"} {
		require.NotContains(t, mapping, excluded)
	}
	// Readers only see complete, immutable maps, including concurrent cold misses.
	var readers sync.WaitGroup
	for range 16 {
		readers.Go(func() { _ = a.GetModelMapping()["kimi-k2.6"] })
	}
	readers.Wait()
	raw["moonshotai/kimi-k2.6"] = "new-target"
	require.Equal(t, "new-target", a.GetMappedModel("kimi-k2.6"))
	require.Equal(t, "moonshotai/kimi-k2.6", mapping["kimi-k2.6"], "previous memo remains immutable")
}

func TestDomesticModelAliasesExplicitAndAmbiguous(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		raw := map[string]any{"z-ai/glm-5": "first", "zhipuai/glm-5": "second"}
		if explicit {
			raw["glm-5"] = "explicit"
		}
		a := &Account{Platform: PlatformMixed, Type: AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": raw}}
		if explicit {
			require.Equal(t, "explicit", a.GetMappedModel("glm-5"))
		} else {
			require.NotContains(t, a.GetModelMapping(), "glm-5")
		}
	}
	for _, platform := range []string{PlatformOpenAI, PlatformAnthropic, PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax} {
		a := &Account{Platform: platform, Type: AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": map[string]any{"moonshotai/kimi-k2.6": "raw"}}}
		require.NotContains(t, a.GetModelMapping(), "kimi-k2.6", platform)
	}
}
