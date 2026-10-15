package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/gemini"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/googleapi"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
)

// Gemini 分组对外提供 AI Studio 兼容协议（GET /v1beta/models、/v1beta/models/{model}），
// 分组内的账号可以是 AI Studio 兼容上游（API Key / OAuth），也可以是 Agent Platform (Vertex)。
// 本文件把不同账号类型的模型接口统一成 AI Studio 格式的响应。

// errGeminiModelsUpstreamStatus 在 Vertex 分页读取遇到非 2xx 时中止分页，原始响应由调用方另行保存。
var errGeminiModelsUpstreamStatus = errors.New("upstream model list returned non-2xx status")

// SelectAccountForModelsEndpoints 为模型列表与模型详情接口选择账号。
//
// 优先级：
//  1. 配置了 api_key 的 API Key 账号
//  2. 不带 project_id 的 OAuth 账号（AI Studio OAuth）
//  3. 显式标记为 ai_studio 的 OAuth 账号
//  4. Code Assist OAuth 账号（令牌可能缺 AI Studio 权限，保留上游错误）
//  5. 其余类型的账号
//  6. Vertex 服务账号（服务账号 JSON 认证，读取 Google 发布方模型列表）
//  7. Vertex API Key 账号（列表接口不接受 API Key，仅展示明确配置的映射）
//
// Vertex 账号排在所有 AI Studio 兼容账号之后，混合分组始终由 AI Studio 兼容账号应答。
// 同级账号按 priority、最近使用时间挑选。
func (s *GeminiMessagesCompatService) SelectAccountForModelsEndpoints(ctx context.Context, groupID *int64, requestedModel ...string) (*Account, error) {
	ctx = withSchedulerRequestMode(ctx, s.accountRepo, s.schedulerSnapshot)
	accounts, err := s.listSchedulableAccountsOnce(ctx, groupID, PlatformGemini, true)
	if err != nil {
		return nil, fmt.Errorf("query accounts failed: %w", err)
	}
	accounts = applySchedulerFreshnessForRequest(ctx, s.accountRepo, s.schedulerSnapshot, accounts)
	if len(accounts) == 0 {
		return nil, errors.New("no available Gemini accounts")
	}

	var selected *Account
	for i := range accounts {
		acc := &accounts[i]
		if acc.IsGeminiOpenAIProtocol() || acc.Platform != PlatformGemini {
			continue
		}
		if platform, resolved := ResolvedTargetPlatformFromContext(ctx); resolved && platform != acc.Platform {
			continue
		}
		if len(requestedModel) > 0 && !acc.IsModelSupported(requestedModel[0]) {
			continue
		}
		if selected == nil {
			selected = acc
			continue
		}

		r1, r2 := modelsEndpointAccountRank(acc), modelsEndpointAccountRank(selected)
		if r1 < r2 {
			selected = acc
			continue
		}
		if r1 > r2 {
			continue
		}

		if acc.Priority < selected.Priority {
			selected = acc
		} else if acc.Priority == selected.Priority {
			switch {
			case acc.LastUsedAt == nil && selected.LastUsedAt != nil:
				selected = acc
			case acc.LastUsedAt != nil && selected.LastUsedAt == nil:
				// keep selected
			case acc.LastUsedAt == nil && selected.LastUsedAt == nil:
				if acc.Type == AccountTypeOAuth && selected.Type != AccountTypeOAuth {
					selected = acc
				}
			default:
				if acc.LastUsedAt.Before(*selected.LastUsedAt) {
					selected = acc
				}
			}
		}
	}

	if selected == nil {
		return nil, errors.New("no available Gemini accounts")
	}
	return s.hydrateSelectedAccount(ctx, selected)
}

// modelsEndpointAccountRank 返回账号在模型接口选号中的优先级，数值越小越优先，顺序见 SelectAccountForModelsEndpoints。
func modelsEndpointAccountRank(a *Account) int {
	if a == nil {
		return 999
	}
	switch a.Type {
	case AccountTypeAPIKey:
		if strings.TrimSpace(a.GetCredential("api_key")) != "" {
			return 0
		}
		return 9
	case AccountTypeOAuth:
		if strings.TrimSpace(a.GetCredential("project_id")) == "" {
			return 1
		}
		if strings.TrimSpace(a.GetCredential("oauth_type")) == "ai_studio" {
			return 2
		}
		return 3
	case AccountTypeServiceAccount:
		if a.IsVertexAPIKey() {
			return 12
		}
		return 11
	default:
		return 10
	}
}

// ForwardListModels 读取账号上游的模型列表，返回 AI Studio 格式的响应（GET /v1beta/models）。
// AI Studio 兼容账号原样返回上游响应；Vertex 服务账号读取 Google 发布方模型列表后转换格式，
// 上游非 2xx 时原样返回该响应；Vertex API Key 账号只返回明确配置的模型映射。
func (s *GeminiMessagesCompatService) ForwardListModels(ctx context.Context, account *Account) (*UpstreamHTTPResult, error) {
	if account == nil {
		return nil, errors.New("account is nil")
	}
	if !account.IsVertexServiceAccount() {
		res, err := s.forwardGeminiModelsGET(ctx, account, "/v1beta/models")
		if err != nil {
			return nil, err
		}
		return projectGeminiAccountCatalog(ctx, res, account)
	}
	if account.IsVertexAPIKey() {
		models := make([]gemini.Model, 0)
		for _, id := range configuredGeminiModelIDs(account) {
			models = append(models, gemini.FallbackModel(id))
		}
		return geminiModelsJSONResult(http.StatusOK, gemini.ModelsListResponse{Models: models})
	}

	modelIDs, failure, err := s.listVertexGeminiModels(ctx, account, account.VertexLocation(""))
	if err != nil {
		return nil, err
	}
	if failure != nil {
		return failure, nil
	}
	models := make([]gemini.Model, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		models = append(models, gemini.FallbackModel(modelID))
	}
	res, err := geminiModelsJSONResult(http.StatusOK, gemini.ModelsListResponse{Models: models})
	if err != nil {
		return nil, err
	}
	return projectGeminiAccountCatalog(ctx, res, account)
}

// ForwardGetModel 读取账号上游的单个模型信息，返回 AI Studio 格式的响应（GET /v1beta/models/{model}）。
// Vertex 服务账号在该模型生成请求所用区域的模型列表中查找，找不到时返回 404；Vertex API Key 账号只返回已配置模型的信息。
func (s *GeminiMessagesCompatService) ForwardGetModel(ctx context.Context, account *Account, model string) (*UpstreamHTTPResult, error) {
	if account == nil {
		return nil, errors.New("account is nil")
	}
	if !IsSafeGeminiModelPathSegment(model) {
		return nil, errors.New("invalid model")
	}
	if !account.IsModelSupported(model) {
		return geminiModelNotFound(model)
	}
	upstreamModel := account.GetMappedModel(model)
	if !IsSafeGeminiModelPathSegment(upstreamModel) {
		return nil, errors.New("invalid mapped model")
	}
	if !account.IsVertexServiceAccount() {
		res, err := s.forwardGeminiModelsGET(ctx, account, "/v1beta/models/"+upstreamModel)
		if err != nil {
			return nil, err
		}
		if res.StatusCode == http.StatusOK && model != upstreamModel {
			var metadata map[string]json.RawMessage
			if json.Unmarshal(res.Body, &metadata) != nil || metadata == nil {
				return nil, errors.New("invalid model metadata")
			}
			metadata["name"], _ = json.Marshal("models/" + model)
			res.Body, err = json.Marshal(metadata)
		}
		return res, err
	}
	if account.IsVertexAPIKey() {
		if slices.Contains(configuredGeminiModelIDs(account), model) {
			return geminiModelsJSONResult(http.StatusOK, gemini.FallbackModel(model))
		}
		return geminiModelNotFound(model)
	}

	modelIDs, failure, err := s.listVertexGeminiModels(ctx, account, account.VertexLocation(upstreamModel))
	if err != nil {
		return nil, err
	}
	if failure != nil {
		return failure, nil
	}
	if slices.Contains(modelIDs, upstreamModel) {
		return geminiModelsJSONResult(http.StatusOK, gemini.FallbackModel(model))
	}
	return geminiModelNotFound(model)
}

func geminiModelNotFound(model string) (*UpstreamHTTPResult, error) {
	return geminiModelsJSONResult(http.StatusNotFound, map[string]any{
		"error": map[string]any{
			"code":    http.StatusNotFound,
			"message": fmt.Sprintf("models/%s is not found", model),
			"status":  googleapi.HTTPStatusToGoogleStatus(http.StatusNotFound),
		},
	})
}

// forwardGeminiModelsGET 向 AI Studio 兼容上游转发模型接口的 GET 请求。
// path 会被直接拼到上游 base URL 后面，因此按路径护栏逐片段校验，见 upstream_path_guard.go。
func (s *GeminiMessagesCompatService) forwardGeminiModelsGET(ctx context.Context, account *Account, path string) (*UpstreamHTTPResult, error) {
	sanitizedPath, ok := sanitizedUpstreamPathSuffix(path)
	if !ok || sanitizedPath == "" {
		return nil, errors.New("invalid path")
	}

	baseURL := account.GetGeminiBaseURL(geminicli.AIStudioBaseURL)
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	fullURL := strings.TrimRight(normalizedBaseURL, "/") + sanitizedPath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}

	switch account.Type {
	case AccountTypeAPIKey:
		apiKey := strings.TrimSpace(account.GetCredential("api_key"))
		if apiKey == "" {
			return nil, errors.New("gemini api_key not configured")
		}
		req.Header.Set("x-goog-api-key", apiKey)
	case AccountTypeOAuth:
		if s.tokenProvider == nil {
			return nil, errors.New("gemini token provider not configured")
		}
		accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
	default:
		return nil, fmt.Errorf("unsupported account type: %s", account.Type)
	}

	return s.doGeminiModelsGET(req, account)
}

// listVertexGeminiModels 读取 Vertex 服务账号在指定区域可用的 Gemini 模型。
// 上游返回非 2xx 时 failure 为该页的原始响应，modelIDs 与 err 均为空。
func (s *GeminiMessagesCompatService) listVertexGeminiModels(ctx context.Context, account *Account, location string) (modelIDs []string, failure *UpstreamHTTPResult, err error) {
	if s.tokenProvider == nil {
		return nil, nil, errors.New("gemini token provider not configured")
	}
	accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, nil, err
	}

	modelIDs, err = listVertexGeminiPublisherModels(location, func(_ int, listURL string) ([]byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+accessToken)

		res, err := s.doGeminiModelsGET(req, account)
		if err != nil {
			return nil, err
		}
		if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
			failure = res
			return nil, errGeminiModelsUpstreamStatus
		}
		return res.Body, nil
	})
	if failure != nil {
		return nil, failure, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return modelIDs, nil, nil
}

// doGeminiModelsGET 发送模型接口请求并读取响应。响应体上限取 gateway.models_list_read_max_bytes，超出时报错；响应头按配置过滤。
func (s *GeminiMessagesCompatService) doGeminiModelsGET(req *http.Request, account *Account) (*UpstreamHTTPResult, error) {
	var proxyURL string
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Body == nil {
		return nil, errors.New("empty upstream model response")
	}
	defer func() { _ = resp.Body.Close() }()

	bodyLimit := resolveModelsListReadLimit(s.cfg)
	body, err := io.ReadAll(io.LimitReader(resp.Body, bodyLimit+1))
	if err != nil {
		return nil, fmt.Errorf("read upstream model response: %w", err)
	}
	if int64(len(body)) > bodyLimit {
		return nil, fmt.Errorf("upstream model list response exceeds %d bytes", bodyLimit)
	}
	wwwAuthenticate := resp.Header.Get("Www-Authenticate")
	filteredHeaders := responseheaders.FilterHeaders(resp.Header, s.responseHeaderFilter)
	if wwwAuthenticate != "" {
		filteredHeaders.Set("Www-Authenticate", wwwAuthenticate)
	}
	return &UpstreamHTTPResult{
		StatusCode: resp.StatusCode,
		Headers:    filteredHeaders,
		Body:       body,
	}, nil
}

// geminiModelsJSONResult 把网关生成的模型接口响应包装成 UpstreamHTTPResult，交给 handler 统一写出。
func geminiModelsJSONResult(status int, payload any) (*UpstreamHTTPResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &UpstreamHTTPResult{
		StatusCode: status,
		Headers:    http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
		Body:       body,
	}, nil
}

// configuredGeminiModelIDs never manufactures a catalog from the global defaults.
// Wildcard mappings cannot prove that any concrete model is available.
func configuredGeminiModelIDs(account *Account) []string {
	if account == nil {
		return nil
	}
	ids := make([]string, 0)
	for public, target := range account.GetModelMapping() {
		if IsSafeGeminiModelPathSegment(public) && IsSafeGeminiModelPathSegment(target) &&
			strings.HasPrefix(strings.ToLower(target), "gemini-") {
			ids = append(ids, public)
		}
	}
	return dedupeAndSortModelIDs(ids)
}

// Project the real upstream catalog through the same account mapping used by
// generation, preserving every unknown envelope/metadata field. A mapping is
// not proof of existence when an upstream catalog is available.
func projectGeminiAccountCatalog(ctx context.Context, res *UpstreamHTTPResult, account *Account) (*UpstreamHTTPResult, error) {
	if res == nil || res.StatusCode != http.StatusOK {
		return res, nil
	}
	if len(account.GetModelMapping()) == 0 {
		return res, nil
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(res.Body, &envelope) != nil || envelope == nil {
		return nil, errors.New("invalid model catalog")
	}
	var models []map[string]json.RawMessage
	if json.Unmarshal(envelope["models"], &models) != nil {
		return nil, errors.New("invalid model catalog")
	}
	projected := make([]map[string]json.RawMessage, 0)
	for _, model := range models {
		var name string
		if json.Unmarshal(model["name"], &name) != nil {
			return nil, errors.New("invalid model name")
		}
		id := strings.TrimPrefix(name, "models/")
		publicIDs := []string{}
		if account.IsModelSupported(id) && account.GetMappedModel(id) == id {
			publicIDs = append(publicIDs, id)
		}
		for public := range account.GetModelMapping() {
			if IsSafeGeminiModelPathSegment(public) && public != id && account.GetMappedModel(public) == id {
				publicIDs = append(publicIDs, public)
			}
		}
		for _, public := range dedupeAndSortModelIDs(publicIDs) {
			copy := make(map[string]json.RawMessage, len(model))
			for key, value := range model {
				copy[key] = value
			}
			copy["name"], _ = json.Marshal("models/" + public)
			projected = append(projected, copy)
		}
	}
	envelope["models"], _ = json.Marshal(projected)
	var err error
	res.Body, err = json.Marshal(envelope)
	return res, err
}

// GeminiOpenAIModelIDs lists only explicit mappings that the existing native
// Gemini -> OpenAI bridge can actually serve. Mixed accounts are not native
// Gemini accounts and are deliberately not advertised by this endpoint.
func (s *GeminiMessagesCompatService) GeminiOpenAIModelIDs(ctx context.Context, groupID *int64) ([]string, error) {
	ctx = withSchedulerRequestMode(ctx, s.accountRepo, s.schedulerSnapshot)
	accounts, err := s.listSchedulableAccountsOnce(ctx, groupID, PlatformGemini, true)
	if err != nil {
		return nil, err
	}
	var ids []string
	for i := range accounts {
		account := &accounts[i]
		if account.IsGeminiOpenAIProtocol() {
			for _, id := range configuredGeminiModelIDs(account) {
				ids = append(ids, id)
			}
		}
	}
	return dedupeAndSortModelIDs(ids), nil
}
