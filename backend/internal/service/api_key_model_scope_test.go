package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	gocache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/require"
)

func TestModelScopeClassifier(t *testing.T) {
	cases := map[string]string{
		" Models/OpenAI/GPT-5.5 ": "openai", "chatgpt-4o": "openai", "codex-mini": "openai", "o1": "openai", "o3-mini": "openai", "o4-mini": "openai", "o5": "openai",
		"o10": "other", "o31": "other", "foo/gpt-5": "other", "openai/claude-4": "", "qwen/gpt-5": "", "models/models/gpt-5": "other",
		"text-embedding-3-large": "openai", "openai/whisper-1": "openai", "tts-1-hd": "openai", "dall-e-3": "openai", "chatgpt/gpt5.4": "openai",
		"anthropic/claude-opus-4": "anthropic", "opus-4": "anthropic", "sonnet-4": "anthropic", "haiku-4": "anthropic",
		"qwen/qwen3.8-27b": "cn", "qwq-32b": "cn", "z-ai/glm-5.3": "cn", "zhipuai/glm-5": "cn", "deepseek-ai/deepseek-v4": "cn", "deepseek/deepseek-chat": "cn",
		"moonshotai/kimi-k2.6": "cn", "minimaxai/minimax-m2": "cn", "xiaomi/mimo-v2": "cn", "xiaomimimo/mimo-v2": "cn", "stepfun-ai/step-3": "cn", "k3-256k": "cn", "abab6.5-chat": "cn",
		"glmish": "other", "deepseekx": "other", "custom/deepseek-chat": "other", "z-ai/qwen3": "", "gemini-3": "other", "grok-4": "other", "opaque-final": "other",
		"chatglm_turbo": "cn", "zhipuai/chatglm_pro": "cn", "cogview-3": "cn", "thudm/cogvideo": "cn", "doubao-embedding-vision": "cn",
		"bytedance/doubao-pro-32k": "cn", "yi-large": "cn", "01-ai/yi-1.5": "cn", "ernie-4.0": "cn", "baidu/ernie-speed": "cn",
		"spark-desk": "cn", "iflytek/spark-pro": "cn", "hunyuan-turbo": "cn", "tencent/hunyuan-code": "cn", "k2-thinking": "cn", "k1.5-long": "cn", "m2.7": "cn",
		"chatglmx": "other", "cognitive": "other", "doubaox": "other", "01-ai/gpt-5": "", "unknown/doubao-pro": "other",
	}
	for model, expected := range cases {
		require.Equal(t, expected, ClassifyModelScope(model), model)
	}
	for _, model := range []string{"gpt-5", "claude-4", "qwen3", "grok-4"} {
		count := 0
		for _, scope := range []string{"openai", "anthropic", "cn", "other"} {
			if ModelScopeAllows(scope, model) {
				count++
			}
		}
		require.Equal(t, 1, count)
		require.True(t, ModelScopeAllows("", model))
		require.False(t, ModelScopeAllows("future", model))
	}
	require.False(t, ModelScopeAllows("cn", "qwen*"))
	require.ErrorIs(t, validateCreateAPIKeyRequest(CreateAPIKeyRequest{ModelScope: "CN"}), ErrInvalidModelScope)
}

func TestAPIKeyModelScopeAuthSnapshotRoundtrip(t *testing.T) {
	svc := &APIKeyService{}
	for _, scope := range []string{"", "all", "openai", "anthropic", "cn", "other", "unknown"} {
		key := &APIKey{ModelScope: scope, User: &User{ID: 1}}
		snapshot := svc.snapshotFromAPIKey(context.Background(), key)
		require.Equal(t, apiKeyAuthSnapshotVersion, snapshot.Version)
		payload, err := json.Marshal(snapshot)
		require.NoError(t, err)
		var decoded APIKeyAuthSnapshot
		require.NoError(t, json.Unmarshal(payload, &decoded))
		require.Equal(t, scope, svc.snapshotToAPIKey("secret", &decoded).ModelScope)
	}
	_, used, err := svc.applyAuthCacheEntry("secret", &APIKeyAuthCacheEntry{Snapshot: &APIKeyAuthSnapshot{Version: 25}})
	require.NoError(t, err)
	require.False(t, used)
}

type modelScopeKeyRepo struct {
	APIKeyRepository
	key    APIKey
	fields APIKeyUpdateFields
}

func (r *modelScopeKeyRepo) GetByID(context.Context, int64) (*APIKey, error) {
	copyKey := r.key
	return &copyKey, nil
}
func (r *modelScopeKeyRepo) Update(_ context.Context, key *APIKey, fields APIKeyUpdateFields) error {
	r.fields = fields
	r.key = *key
	return nil
}

func TestAPIKeyModelScopeUpdateOmittedAndExplicit(t *testing.T) {
	for _, scope := range []string{"all", "cn"} {
		repo := &modelScopeKeyRepo{key: APIKey{ID: 1, UserID: 1, ModelScope: scope}}
		svc := &APIKeyService{apiKeyRepo: repo}
		name := "renamed"
		got, err := svc.Update(context.Background(), 1, 1, UpdateAPIKeyRequest{Name: &name})
		require.NoError(t, err)
		require.Equal(t, scope, got.ModelScope)
		require.Equal(t, APIKeyUpdateFields{Name: true}, repo.fields)
		selected := "openai"
		got, err = svc.Update(context.Background(), 1, 1, UpdateAPIKeyRequest{ModelScope: &selected})
		require.NoError(t, err)
		require.Equal(t, selected, got.ModelScope)
		require.Equal(t, APIKeyUpdateFields{ModelScope: true}, repo.fields)
		invalid := "future"
		_, err = svc.Update(context.Background(), 1, 1, UpdateAPIKeyRequest{ModelScope: &invalid})
		require.ErrorIs(t, err, ErrInvalidModelScope)
	}
}

type modelScopeAccountRepo struct {
	AccountRepository
	accounts []Account
	calls    int
}

func (r *modelScopeAccountRepo) ListSchedulableByGroupID(context.Context, int64) ([]Account, error) {
	r.calls++
	return r.accounts, nil
}

func TestMixedSupportedKeyProvidersCatalogue(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mapping map[string]any
		allow   []string
		want    []string
	}{
		{"four", map[string]any{"gpt-5": "gpt-5", "claude-4": "claude-4", "qwen3": "qwen3", "gemini-3": "gemini-3", "grok-4": "grok-4"}, nil, []string{"openai", "anthropic", "cn", "other"}},
		{"partial", map[string]any{"qwen3": "qwen3"}, nil, []string{"cn"}},
		{"existing domestic families", map[string]any{"chatglm_turbo": "chatglm_turbo", "cogview-3": "cogview-3", "doubao-pro-32k": "doubao-pro-32k", "ernie-4.0": "ernie-4.0"}, nil, []string{"cn"}},
		{"empty", map[string]any{}, nil, []string{}},
		{"allowlist", map[string]any{"qwen3": "qwen3", "gpt-5": "gpt-5"}, []string{"qwen3"}, []string{"cn"}},
		{"cross mappings", map[string]any{"gpt-5": "qwen3", "claude-4": "opaque-final", "alias": "gpt-5", "qwen*": "qwen3"}, nil, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &modelScopeAccountRepo{accounts: []Account{{Platform: PlatformMixed, Credentials: map[string]any{"model_mapping": tc.mapping}}}}
			svc := &GatewayService{accountRepo: repo, modelsListCache: gocache.New(time.Minute, time.Minute), modelsListCacheTTL: time.Minute}
			group := &Group{ID: 10, Platform: PlatformMixed, ModelAllowlist: GroupModelAllowlist{Enabled: tc.allow != nil, Models: tc.allow}}
			require.Equal(t, tc.want, svc.SupportedKeyProviders(context.Background(), group))
			require.Equal(t, tc.want, svc.SupportedKeyProviders(context.Background(), group))
			require.Equal(t, 1, repo.calls, "one shared catalogue read for all four categories")
		})
	}
}

func TestModelScopeFinalOutboundGuard(t *testing.T) {
	for _, tc := range []struct {
		scope, url, body string
		denied           bool
	}{
		{"openai", "https://relay.test/v1/responses", `{"model":"gpt-5"}`, false},
		{"openai", "https://relay.test/v1/chat/completions", `{"model":"qwen3"}`, true},
		{"cn", "https://relay.test/v1/responses/compact", `{"model":"gpt-5"}`, true},
		{"cn", "https://relay.test/v1/chat/completions", `{"model":"chatglm_turbo"}`, false},
		{"other", "https://relay.test/v1/chat/completions", `{"model":"chatglm_turbo"}`, true},
		{"cn", "https://relay.test/v1/embeddings", `{"model":"doubao-embedding-vision"}`, false},
		{"other", "https://relay.test/v1/embeddings", `{"model":"doubao-embedding-vision"}`, true},
		{"cn", "https://relay.test/v1/messages/count_tokens", `{"model":"opaque-final"}`, true},
		{"anthropic", "https://relay.test/v1beta/models/gemini-3:generateContent", `{}`, true},
		{"anthropic", "https://relay.test/v1internal:generateContent", `{"model":"gemini-3","request":{}}`, true},
		{"openai", "https://relay.test/v1/responses", `{"model":"gpt-5","Model":"glm-5"}`, true},
		{"openai", "https://relay.test/v1/responses", `{"model":"gpt-5","session":{"model":"grok-4"}}`, true},
		{"all", "https://relay.test/v1/messages", `{"model":"anything"}`, false},
	} {
		req, err := http.NewRequestWithContext(ContextWithModelScope(context.Background(), tc.scope), http.MethodPost, tc.url, strings.NewReader(tc.body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		err = CheckUpstreamModelScope(req)
		if tc.denied {
			require.ErrorIs(t, err, ErrModelScopeDenied, tc.body)
		} else {
			require.NoError(t, err)
		}
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Equal(t, tc.body, string(body))
	}
}

func TestAPIKeyScopeRouteContextAndClones(t *testing.T) {
	id := int64(2)
	key := &APIKey{ModelScope: "cn", GroupID: &id, Group: &Group{ID: id}}
	clone := cloneAPIKeyWithGroupID(key, &Group{ID: 3})
	require.Equal(t, "cn", clone.ModelScope)
	ctx := ContextWithAPIKeyRoute(context.Background(), clone)
	require.Equal(t, "cn", ModelScopeFromContext(ctx))
	require.ErrorIs(t, CheckModelScope(ctx, "gpt-5"), ErrModelScopeDenied)
}

func TestModelScopeCatalogueWildcardChannelAndExactPriority(t *testing.T) {
	cat := groupModelsCatalog{scopeMappings: []modelScopeMapping{{platform: PlatformMixed, mapping: map[string]string{
		"gpt-*": "qwen3", "gpt-safe": "gpt-5", "gemini-*": "claude-4",
	}}}}
	require.Equal(t, []string{"gpt-safe"}, filterModelsForKeyScope(cat, "openai", []string{"gpt-5", "gpt-safe"}))
	require.Empty(t, filterModelsForKeyScope(cat, "other", []string{"models/gemini-3"}))
	require.Empty(t, filterModelsForKeyScopeMapped(cat, "openai", []string{"gpt-safe"}, func(string) string { return "qwen3" }))
	// A channel can point at another alias in the same family, which then
	// resolves through the account's wildcard to a different family.
	require.Empty(t, filterModelsForKeyScopeMapped(cat, "openai", []string{"gpt-safe"}, func(string) string { return "gpt-other" }))
	require.Equal(t, []string{"gpt-5", "gpt-safe"}, filterModelsForKeyScope(cat, "all", []string{"gpt-5", "gpt-safe"}))
	cat.scopeMappings = append(cat.scopeMappings, modelScopeMapping{platform: PlatformMixed, mapping: map[string]string{"gpt-5": "gpt-5"}})
	require.Equal(t, []string{"gpt-5", "gpt-safe"}, filterModelsForKeyScope(cat, "openai", []string{"gpt-5", "gpt-safe"}), "one bad mapping must not hide a safe alternative")
}
