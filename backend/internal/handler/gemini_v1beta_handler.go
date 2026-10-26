package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/gemini"
	"github.com/Wei-Shaw/sub2api/internal/pkg/googleapi"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// geminiCLITmpDirRegex 用于从 Gemini CLI 请求体中提取 tmp 目录的哈希值
// 匹配格式: /Users/xxx/.gemini/tmp/[64位十六进制哈希]
var geminiCLITmpDirRegex = regexp.MustCompile(`/\.gemini/tmp/([A-Fa-f0-9]{64})`)

func geminiNativePlatformAllowed(c *gin.Context, apiKey *service.APIKey) bool {
	switch effectiveAPIKeyPlatform(c, apiKey) {
	case service.PlatformGemini, service.PlatformAntigravity:
		return true
	}
	return apiKey != nil && apiKey.UsesRequestTargetPlatform()
}

func geminiStudioGroupIDs(apiKey *service.APIKey) []int64 {
	if apiKey == nil {
		return nil
	}
	if ids := apiKey.CandidateGroupIDs(); len(ids) > 0 {
		return ids
	}
	if apiKey.GroupID != nil && *apiKey.GroupID > 0 {
		return []int64{*apiKey.GroupID}
	}
	if apiKey.Group != nil && apiKey.Group.ID > 0 {
		return []int64{apiKey.Group.ID}
	}
	return nil
}

// GeminiV1BetaListModels proxies:
// GET /v1beta/models
func (h *GatewayHandler) GeminiV1BetaListModels(c *gin.Context) {
	apiKey, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		googleError(c, http.StatusUnauthorized, "Invalid API key")
		return
	}
	forcePlatform, forced := middleware.GetForcePlatformFromContext(c)
	if !forced && !geminiNativePlatformAllowed(c, apiKey) {
		googleError(c, http.StatusBadRequest, "API key group platform is not gemini")
		return
	}
	ctx := c.Request.Context()
	localModels := make([]gemini.Model, 0)
	var response, failure *service.UpstreamHTTPResult
	for _, groupID := range geminiStudioGroupIDs(apiKey) {
		if h.geminiCompatService == nil {
			continue
		}
		group := h.groupForModelsList(ctx, apiKey, groupID)
		requireMixed := forcePlatform != service.PlatformAntigravity && (group == nil || group.Platform != service.PlatformComposite)
		ids, err := h.geminiCompatService.AntigravityGeminiModelIDs(ctx, &groupID, requireMixed)
		if err != nil {
			requestLogger(c, "handler.gemini_v1beta.models").Warn("unable to list Antigravity mappings", zap.Int64("group_id", groupID), zap.Error(err))
		}
		localByPlatform := map[string][]string{service.PlatformAntigravity: ids}
		if forcePlatform != service.PlatformAntigravity {
			compatible, compatErr := h.geminiCompatService.GeminiOpenAIModelIDs(ctx, &groupID)
			if compatErr == nil {
				localByPlatform[service.PlatformGemini] = compatible
			}
		}
		for _, platform := range []string{service.PlatformAntigravity, service.PlatformGemini} {
			local := make([]gemini.Model, 0, len(localByPlatform[platform]))
			for _, id := range localByPlatform[platform] {
				local = append(local, gemini.FallbackModel(id))
			}
			localBody, _ := json.Marshal(gemini.ModelsListResponse{Models: local})
			if projected, projectErr := h.filterGeminiCatalogForGroup(ctx, apiKey, groupID, localBody, platform); projectErr == nil {
				var catalog gemini.ModelsListResponse
				if json.Unmarshal(projected, &catalog) == nil {
					localModels = append(localModels, catalog.Models...)
				}
			}
		}
		if forcePlatform == service.PlatformAntigravity {
			continue
		}
		account, err := h.geminiCompatService.SelectAccountForModelsEndpoints(ctx, &groupID)
		if err != nil || account == nil {
			continue
		}
		res, err := h.geminiCompatService.ForwardListModels(ctx, account)
		if err != nil || res == nil {
			continue
		}
		if res.StatusCode != http.StatusOK {
			if failure == nil {
				failure = res
			}
			continue
		}
		body, err := h.filterGeminiCatalogForGroup(ctx, apiKey, groupID, res.Body, account.Platform)
		if err != nil {
			googleError(c, http.StatusBadGateway, "Invalid model catalogue")
			return
		}
		res.Body = body
		if response == nil {
			response = res
		} else if merged, ok := mergeUpstreamGeminiCatalogs(response.Body, res.Body); ok {
			response.Body = merged
		} else {
			googleError(c, http.StatusBadGateway, "Invalid model catalogue")
			return
		}
	}
	if response == nil && failure != nil {
		writeUpstreamResponse(c, failure)
		return
	}
	if response == nil {
		// Only explicitly supported local mappings, never the builtin full list.
		body, _ := json.Marshal(gemini.ModelsListResponse{Models: []gemini.Model{}})
		response = &service.UpstreamHTTPResult{StatusCode: http.StatusOK, Headers: http.Header{"Content-Type": {"application/json"}}, Body: body}
	}
	if merged, ok := appendUpstreamGeminiModels(response.Body, localModels); ok {
		response.Body = merged
	}
	writeUpstreamResponse(c, response)
}

func (h *GatewayHandler) filterGeminiIDsForGroup(ctx context.Context, apiKey *service.APIKey, groupID int64, ids []string) []string {
	group := h.groupForModelsList(ctx, apiKey, groupID)
	if group == nil {
		return nil // A cold group-policy projection cannot establish its allowlist.
	}
	allowed := ids
	filtered := make([]string, 0, len(allowed))
	for _, id := range allowed {
		if !group.ModelAllowlistEnabled() || group.ModelAllowlist.Allows(id) {
			filtered = append(filtered, id)
		}
	}
	return filtered
}

func (h *GatewayHandler) filterGeminiCatalogForGroup(ctx context.Context, apiKey *service.APIKey, groupID int64, body []byte, catalogPlatform string) ([]byte, error) {
	var envelope map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) != nil || envelope == nil {
		return nil, errors.New("invalid model catalog")
	}
	var models []json.RawMessage
	if json.Unmarshal(envelope["models"], &models) != nil {
		return nil, errors.New("invalid model catalog")
	}
	ids := make([]string, len(models))
	metadata := make(map[string]json.RawMessage, len(models))
	for i, raw := range models {
		var model struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(raw, &model) != nil || model.Name == "" {
			return nil, errors.New("invalid model metadata")
		}
		ids[i] = strings.TrimPrefix(model.Name, "models/")
		metadata[ids[i]] = raw
	}
	group := h.groupForModelsList(ctx, apiKey, groupID)
	if h.gatewayService != nil {
		ids = append(ids, h.gatewayService.GeminiCatalogAliases(ctx, group)...)
	}
	allowed := make(map[string]bool)
	for _, id := range h.filterGeminiIDsForGroup(ctx, apiKey, groupID, ids) {
		allowed[id] = true
	}
	kept := make([]json.RawMessage, 0, len(models))
	seen := make(map[string]bool)
	changed := false
	for _, public := range ids {
		if !allowed[public] || seen[public] {
			continue
		}
		routeCtx, target, ok := h.gatewayService.GeminiCatalogRoute(ctx, group, public)
		if !ok {
			continue
		}
		if platform, resolved := service.ResolvedTargetPlatformFromContext(routeCtx); resolved && platform != catalogPlatform {
			continue
		}
		raw, exists := metadata[target]
		if !exists {
			continue
		}
		if public != target {
			var projected map[string]json.RawMessage
			if json.Unmarshal(raw, &projected) != nil || projected == nil {
				return nil, errors.New("invalid model metadata")
			}
			projected["name"], _ = json.Marshal("models/" + public)
			raw, _ = json.Marshal(projected)
			changed = true
		}
		kept = append(kept, raw)
		seen[public] = true
	}
	if !changed && len(kept) == len(models) {
		return body, nil
	}
	envelope["models"], _ = json.Marshal(kept)
	return json.Marshal(envelope)
}

// Merge only already-authorized entries; retain unknown metadata and the first
// native response envelope (including its pagination token).
func mergeUpstreamGeminiCatalogs(body, extra []byte) ([]byte, bool) {
	var envelope, more map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) != nil || envelope == nil || json.Unmarshal(extra, &more) != nil {
		return nil, false
	}
	var models, additions []json.RawMessage
	if json.Unmarshal(envelope["models"], &models) != nil || json.Unmarshal(more["models"], &additions) != nil {
		return nil, false
	}
	seen := make(map[string]bool)
	for _, raw := range models {
		var model struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(raw, &model) != nil {
			return nil, false
		}
		seen[model.Name] = true
	}
	for _, raw := range additions {
		var model struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(raw, &model) != nil {
			return nil, false
		}
		if !seen[model.Name] {
			models = append(models, raw)
			seen[model.Name] = true
		}
	}
	envelope["models"], _ = json.Marshal(models)
	merged, err := json.Marshal(envelope)
	return merged, err == nil
}

// appendUpstreamGeminiModels preserves unknown model metadata and envelope fields.
func appendUpstreamGeminiModels(body []byte, extra []gemini.Model) ([]byte, bool) {
	var envelope map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) != nil || envelope == nil {
		return body, false
	}
	var models []json.RawMessage
	raw, exists := envelope["models"]
	if !exists || json.Unmarshal(raw, &models) != nil {
		return body, false
	}
	seen := make(map[string]bool, len(models))
	for _, raw := range models {
		var model struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(raw, &model) != nil {
			return body, false
		}
		seen[model.Name] = true
	}
	changed := false
	for _, model := range extra {
		if seen[model.Name] {
			continue
		}
		raw, err := json.Marshal(model)
		if err != nil {
			return body, false
		}
		models = append(models, raw)
		seen[model.Name] = true
		changed = true
	}
	if !changed {
		return body, true
	}
	envelope["models"], _ = json.Marshal(models)
	merged, err := json.Marshal(envelope)
	return merged, err == nil
}

// filterUpstreamGeminiModelsBody 按白名单过滤上游 /v1beta/models 响应中的
// models[].name，其余信封字段（如 nextPageToken）原样保留。
// 返回值：filtered 为过滤后的响应体；dropped 表示是否有条目被移除（全命中时
// 为 false，调用方应保持原始响应以完整透传上游头）；ok=false 表示解析失败，
// 调用方同样应透传原始响应。
func filterUpstreamGeminiModelsBody(body []byte, allowlist service.GroupModelAllowlist) (filtered []byte, dropped bool, ok bool) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, false, false
	}
	rawModels, hasModels := envelope["models"]
	if !hasModels {
		return body, false, true
	}
	type geminiModelName struct {
		Name string `json:"name"`
	}
	var models []json.RawMessage
	if err := json.Unmarshal(rawModels, &models); err != nil {
		return nil, false, false
	}
	kept := make([]json.RawMessage, 0, len(models))
	for _, raw := range models {
		var model geminiModelName
		if err := json.Unmarshal(raw, &model); err != nil {
			return nil, false, false
		}
		if allowlist.Allows(model.Name) {
			kept = append(kept, raw)
		}
	}
	if len(kept) == len(models) {
		// 全部过滤时直接透传原始响应。
		return body, false, true
	}
	mergedModels, err := json.Marshal(kept)
	if err != nil {
		return nil, false, false
	}
	envelope["models"] = mergedModels
	merged, err := json.Marshal(envelope)
	if err != nil {
		return nil, false, false
	}
	return merged, true, true
}

// GeminiV1BetaGetModel proxies:
// GET /v1beta/models/{model}
func (h *GatewayHandler) GeminiV1BetaGetModel(c *gin.Context) {
	apiKey, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		googleError(c, http.StatusUnauthorized, "Invalid API key")
		return
	}
	forcePlatform, forced := middleware.GetForcePlatformFromContext(c)
	if !forced && !geminiNativePlatformAllowed(c, apiKey) {
		googleError(c, http.StatusBadRequest, "API key group platform is not gemini")
		return
	}
	publicModel := strings.TrimSpace(c.Param("model"))
	model := publicModel
	if resolved, ok := service.ResolvedUpstreamModelFromContext(c.Request.Context()); ok && strings.TrimSpace(resolved) != "" {
		model = strings.TrimSpace(resolved)
	}
	if !forced && servesCompositeAntigravityGemini(apiKey, effectiveAPIKeyPlatform(c, apiKey)) && !isAntigravityGeminiModel(model) {
		googleError(c, http.StatusBadRequest, "API key group platform is not gemini")
		return
	}
	// Validate both the public and the mapped segment before constructing a URL.
	if !service.IsSafeGeminiModelPathSegment(publicModel) || !service.IsSafeGeminiModelPathSegment(model) {
		googleError(c, http.StatusBadRequest, "Invalid model in URL")
		return
	}
	ctx := c.Request.Context()
	var failure *service.UpstreamHTTPResult
	sawTransportError := false
	for _, groupID := range geminiStudioGroupIDs(apiKey) {
		if len(h.filterGeminiIDsForGroup(ctx, apiKey, groupID, []string{publicModel})) == 0 || h.geminiCompatService == nil {
			continue
		}
		routeCtx, target, targetOK := h.gatewayService.GeminiCatalogRoute(ctx, h.groupForModelsList(ctx, apiKey, groupID), publicModel)
		model = target
		if !targetOK {
			continue
		}
		platform, resolved := service.ResolvedTargetPlatformFromContext(routeCtx)
		var ids []string
		if !resolved || platform == service.PlatformAntigravity {
			requireMixed := forcePlatform != service.PlatformAntigravity && !resolved
			ids, _ = h.geminiCompatService.AntigravityGeminiModelIDs(routeCtx, &groupID, requireMixed)
		}
		if forcePlatform != service.PlatformAntigravity && (!resolved || platform == service.PlatformGemini) {
			compatible, err := h.geminiCompatService.GeminiOpenAIModelIDs(routeCtx, &groupID)
			if err == nil {
				ids = append(ids, compatible...)
			}
		}
		for _, id := range ids {
			if id == model {
				c.JSON(http.StatusOK, gemini.FallbackModel(publicModel))
				return
			}
		}
		if forcePlatform == service.PlatformAntigravity {
			continue
		}
		account, err := h.geminiCompatService.SelectAccountForModelsEndpoints(routeCtx, &groupID, model)
		if err != nil || account == nil {
			continue
		}
		res, err := h.geminiCompatService.ForwardGetModel(routeCtx, account, model)
		if err != nil || res == nil {
			sawTransportError = true
			continue
		}
		if res.StatusCode == http.StatusOK {
			if model != publicModel {
				var metadata map[string]json.RawMessage
				if json.Unmarshal(res.Body, &metadata) != nil || metadata == nil {
					googleError(c, http.StatusBadGateway, "Invalid model metadata")
					return
				}
				metadata["name"], _ = json.Marshal("models/" + publicModel)
				res.Body, _ = json.Marshal(metadata)
			}
			writeUpstreamResponse(c, res)
			return
		}
		if failure == nil || (failure.StatusCode == http.StatusNotFound && res.StatusCode != http.StatusNotFound) {
			failure = res
		}
	}
	if failure != nil {
		writeUpstreamResponse(c, failure)
	} else if sawTransportError {
		googleError(c, http.StatusBadGateway, "Unable to fetch upstream model")
	} else {
		googleError(c, http.StatusNotFound, "Model is not available for this API key")
	}
}

// GeminiV1BetaModels proxies Gemini native REST endpoints like:
// POST /v1beta/models/{model}:generateContent
// POST /v1beta/models/{model}:streamGenerateContent?alt=sse
func (h *GatewayHandler) GeminiV1BetaModels(c *gin.Context) {
	c.Request = c.Request.WithContext(h.gatewayService.PrepareSchedulerRequestContext(c.Request.Context()))
	apiKey, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		googleError(c, http.StatusUnauthorized, "Invalid API key")
		return
	}
	authSubject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		googleError(c, http.StatusInternalServerError, "User context not found")
		return
	}
	reqLog := requestLogger(
		c,
		"handler.gemini_v1beta.models",
		zap.Int64("user_id", authSubject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)

	// 检查平台：优先使用强制平台（/antigravity 路由，中间件已设置 request.Context），否则要求 gemini 分组
	if !middleware.HasForcePlatform(c) {
		if !geminiNativePlatformAllowed(c, apiKey) {
			googleError(c, http.StatusBadRequest, "API key group platform is not gemini")
			return
		}
	}

	modelName, action, err := parseGeminiModelAction(strings.TrimPrefix(c.Param("modelAction"), "/"))
	if err != nil {
		googleError(c, http.StatusNotFound, err.Error())
		return
	}
	// URL 里的模型名最终会被拼进上游 /v1beta/models/{model}:{action}，
	// 先在入口校验片段合规性，见 service/upstream_path_guard.go。
	if !service.IsSafeGeminiModelPathSegment(modelName) {
		googleError(c, http.StatusBadRequest, "Invalid model in URL")
		return
	}
	if apiKey.Group != nil && apiKey.Group.ModelAllowlistEnabled() && !apiKey.Group.ModelAllowlist.Allows(modelName) {
		googleError(c, http.StatusNotFound, "Model is not available for this group")
		return
	}
	urlModel := modelName
	if resolvedModel, ok := service.ResolvedUpstreamModelFromContext(c.Request.Context()); ok && strings.TrimSpace(resolvedModel) != "" {
		modelName = strings.TrimSpace(resolvedModel)
	}
	if !middleware.HasForcePlatform(c) && servesCompositeAntigravityGemini(apiKey, effectiveAPIKeyPlatform(c, apiKey)) && !isAntigravityGeminiModel(modelName) {
		googleError(c, http.StatusBadRequest, "API key group platform is not gemini")
		return
	}

	stream := action == "streamGenerateContent"
	reqLog = reqLog.With(zap.String("model", modelName), zap.String("action", action), zap.Bool("stream", stream))

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			googleError(c, http.StatusRequestEntityTooLarge, buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		googleError(c, http.StatusBadRequest, "Failed to read request body")
		return
	}
	if len(body) == 0 {
		googleError(c, http.StatusBadRequest, "Request body is empty")
		return
	}

	setOpsRequestContext(c, modelName, stream)
	setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(stream, false)))
	pricingCtx, pricingAt := service.WithGatewayTokenRequestPricing(c.Request.Context())
	c.Request = c.Request.WithContext(pricingCtx)

	// 解析渠道级模型映射
	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, modelName)
	reqModel := modelName // 保存映射前的原始模型名
	if action != "countTokens" && geminiImageGenerationRejected(c, apiKey, body, urlModel, reqModel, channelMapping.MappedModel) {
		googleError(c, http.StatusBadRequest, service.ImageGenerationPermissionMessage())
		return
	}

	if decision := h.checkSecurityAudit(c, reqLog, apiKey, authSubject, service.ContentModerationProtocolGemini, modelName, body); decision != nil && !decision.AllowNextStage {
		googleSecurityAuditError(c, decision)
		return
	}

	if channelMapping.Mapped {
		modelName = channelMapping.MappedModel
	}

	// Get subscription (may be nil)
	subscription, _ := middleware.GetSubscriptionFromContext(c)

	// For Gemini native API, do not send Claude-style ping frames.
	geminiConcurrency := NewConcurrencyHelper(h.concurrencyHelper.concurrencyService, SSEPingFormatNone, 0)

	// 1) user concurrency slot
	streamStarted := false
	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}
	userReleaseFunc, err := geminiConcurrency.AcquireUserSlotWithWait(c, authSubject.UserID, authSubject.Concurrency, stream, &streamStarted)
	if err != nil {
		reqLog.Warn("gemini.user_slot_acquire_failed", zap.Error(err))
		googleError(c, http.StatusTooManyRequests, err.Error())
		return
	}
	// 确保请求取消时也会释放槽位，避免长连接被动中断造成泄漏
	userReleaseFunc = wrapReleaseOnDone(c.Request.Context(), userReleaseFunc)
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	// 2) billing eligibility check (after wait)
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		reqLog.Info("gemini.billing_eligibility_check_failed", zap.Error(err))
		status, _, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		googleError(c, status, message)
		return
	}

	// Reserve the user's live balance before selecting an account or forwarding
	// to Gemini. This native entrypoint shares the same request-local hold guard
	// as OpenAI/Anthropic compatibility routes; the usage worker later transfers
	// that guard and finalizes it against provider-reported usage. Keeping the
	// reservation here (after eligibility and before account selection) closes
	// the race where concurrent low-balance Gemini requests all pass the legacy
	// snapshot check and incur upstream cost before asynchronous settlement.
	balanceGuard, err := preauthorizeTextGatewayRequest(
		c.Request.Context(), h.balancePreauthorizer, h.gatewayService,
		apiKey, subscription, body,
		service.BalancePreauthorizationBillingModel(reqModel, channelMapping),
		pricingAt, "",
	)
	if err != nil {
		status, _, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		googleError(c, status, message)
		return
	}
	defer func() { deferBalancePreauthorizationRefund(reqLog, balanceGuard) }()
	if balanceGuard != nil {
		c.Request = c.Request.WithContext(service.ContextWithBalancePreauthorizationGuard(c.Request.Context(), balanceGuard))
	}

	// 余额模式在途预留：防止并发请求在预检时看到同一份余额而集体透支。
	inflightRelease, err := reserveInflightBalance(c, h.billingCacheService, h.gatewayService, apiKey, subscription, tokenInflightEstimate(modelName, body))
	if err != nil {
		reqLog.Info("gemini.inflight_reservation_rejected", zap.Error(err))
		status, _, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		googleError(c, status, message)
		return
	}
	defer inflightRelease()

	// 3) select account (sticky session based on request body)
	// 优先使用 Gemini CLI 的会话标识（privileged-user-id + tmp 目录哈希）
	sessionHash := extractGeminiCLISessionHash(c, body)
	if sessionHash == "" {
		// Fallback: 使用通用的会话哈希生成逻辑（适用于其他客户端）
		parsedReq, _ := service.ParseGatewayRequest(service.NewRequestBodyRef(body), domain.PlatformGemini)
		if parsedReq != nil {
			parsedReq.SessionContext = &service.SessionContext{
				ClientIP:  ip.GetClientIP(c),
				UserAgent: c.GetHeader("User-Agent"),
				APIKeyID:  apiKey.ID,
			}
		}
		sessionHash = h.gatewayService.GenerateSessionHash(parsedReq)
	}
	sessionKey := sessionHash
	if sessionHash != "" {
		sessionKey = "gemini:" + sessionHash
	}

	// 查询粘性会话绑定的账号 ID（用于检测账号切换）
	var sessionBoundAccountID int64
	if sessionKey != "" {
		sessionBoundAccountID, _ = h.gatewayService.GetCachedSessionAccountID(c.Request.Context(), apiKey.GroupID, sessionKey)
		if sessionBoundAccountID > 0 {
			prefetchedGroupID := int64(0)
			if apiKey.GroupID != nil {
				prefetchedGroupID = *apiKey.GroupID
			}
			ctx := service.WithPrefetchedStickySession(c.Request.Context(), sessionBoundAccountID, prefetchedGroupID, h.metadataBridgeEnabled())
			c.Request = c.Request.WithContext(ctx)
		}
	}

	// === Gemini 内容摘要会话 Fallback 逻辑 ===
	// 当原有会话标识无效时（sessionBoundAccountID == 0），尝试基于内容摘要链匹配
	var geminiDigestChain string
	var geminiPrefixHash string
	var geminiSessionUUID string
	var matchedDigestChain string
	useDigestFallback := sessionBoundAccountID == 0

	if useDigestFallback {
		// 解析 Gemini 请求体
		var geminiReq antigravity.GeminiRequest
		if err := json.Unmarshal(body, &geminiReq); err == nil && len(geminiReq.Contents) > 0 {
			// 生成摘要链
			geminiDigestChain = service.BuildGeminiDigestChain(&geminiReq)
			if geminiDigestChain != "" {
				// 生成前缀 hash
				userAgent := c.GetHeader("User-Agent")
				clientIP := ip.GetClientIP(c)
				platform := ""
				if apiKey.Group != nil {
					platform = apiKey.Group.Platform
				}
				geminiPrefixHash = service.GenerateGeminiPrefixHash(
					authSubject.UserID,
					apiKey.ID,
					clientIP,
					userAgent,
					platform,
					modelName,
				)

				// 查找会话
				foundUUID, foundAccountID, foundMatchedChain, found := h.gatewayService.FindGeminiSession(
					c.Request.Context(),
					derefGroupID(apiKey.GroupID),
					geminiPrefixHash,
					geminiDigestChain,
				)
				if found {
					matchedDigestChain = foundMatchedChain
					sessionBoundAccountID = foundAccountID
					geminiSessionUUID = foundUUID
					reqLog.Info("gemini.digest_fallback_matched",
						zap.String("session_uuid_prefix", safeShortPrefix(foundUUID, 8)),
						zap.Int64("account_id", foundAccountID),
						zap.String("digest_chain", truncateDigestChain(geminiDigestChain)),
					)

					// 关键：如果原 sessionKey 为空，使用 prefixHash + uuid 作为 sessionKey
					// 这样 SelectAccountWithLoadAwareness 的粘性会话逻辑会优先使用匹配到的账号
					if sessionKey == "" {
						sessionKey = service.GenerateGeminiDigestSessionKey(geminiPrefixHash, foundUUID)
					}
					_ = h.gatewayService.BindStickySession(c.Request.Context(), apiKey.GroupID, sessionKey, foundAccountID)
				} else {
					// 生成新的会话 UUID
					geminiSessionUUID = uuid.New().String()
					// 为新会话也生成 sessionKey（用于后续请求的粘性会话）
					if sessionKey == "" {
						sessionKey = service.GenerateGeminiDigestSessionKey(geminiPrefixHash, geminiSessionUUID)
					}
				}
			}
		}
	}

	// 判断是否真的绑定了粘性会话：有 sessionKey 且已经绑定到某个账号
	hasBoundSession := sessionKey != "" && sessionBoundAccountID > 0
	cleanedForUnknownBinding := false

	fs := NewFillFailoverState(h.maxAccountSwitchesGemini, hasBoundSession)

	// 单账号分组提前设置 SingleAccountRetry 标记，让 Service 层首次 503 就不设模型限流标记。
	// 避免单账号分组收到 503 (MODEL_CAPACITY_EXHAUSTED) 时设 29s 限流，导致后续请求连续快速失败。
	c.Request = c.Request.WithContext(h.gatewayService.WithSingleAntigravityAccountGroupProbe(c.Request.Context(), apiKey.GroupID))

	for {
		selection, routedKey, err := h.gatewayService.SelectAccountAlongKeyRoutes(c.Request.Context(), apiKey, sessionKey, reqModel, fs.FailedAccountIDs, "", int64(0), service.PlatformGemini) // Gemini 不使用会话限制
		if err == nil && routedKey != nil {
			if bindErr := h.bindSelectedKeyRoute(c, keyRouteBinding{
				Previous: apiKey, Selected: routedKey, Subscription: &subscription,
				Mapping: &channelMapping, Guard: &balanceGuard, Body: body, Model: reqModel, PricingAt: pricingAt,
			}); bindErr != nil {
				releaseRejectedKeyRouteSelection(selection)
				status, _, message, _ := billingErrorDetails(bindErr)
				googleError(c, status, message)
				return
			}
			apiKey = routedKey
			modelName = openAIChannelForwardModel(channelMapping, reqModel)
		}
		if err != nil {
			if len(fs.FailedAccountIDs) == 0 {
				diagPlatform := service.PlatformGemini
				if p := effectiveAPIKeyPlatform(c, apiKey); p == service.PlatformAntigravity {
					diagPlatform = service.PlatformAntigravity
				}
				cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, modelName, modelName, diagPlatform)
				cls = classifySelectionFailureErrorFromGin(c, err, cls)
				if !cls.ModelNotFound {
					markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
				}
				message := cls.Message
				if !cls.ModelNotFound {
					message = "No available Gemini accounts: " + err.Error()
				}
				googleError(c, cls.Status, message)
				return
			}
			action := fs.HandleSelectionExhausted(c.Request.Context())
			switch action {
			case FailoverContinue:
				ctx := service.WithSingleAccountRetry(c.Request.Context(), true, h.metadataBridgeEnabled())
				c.Request = c.Request.WithContext(ctx)
				continue
			case FailoverCanceled:
				failoverClientGone(c)
				return
			default: // FailoverExhausted
				h.handleGeminiFailoverExhausted(c, fs.LastFailoverErr)
				return
			}
		}
		account := selection.Account
		setOpsSelectedAccount(c, account.ID, account.Platform, account.Name)

		// 检测账号切换：如果粘性会话绑定的账号与当前选择的账号不同，清除 thoughtSignature
		// 注意：Gemini 原生 API 的 thoughtSignature 与具体上游账号强相关；跨账号透传会导致 400。
		if sessionBoundAccountID > 0 && sessionBoundAccountID != account.ID {
			reqLog.Info("gemini.sticky_session_account_switched",
				zap.Int64("from_account_id", sessionBoundAccountID),
				zap.Int64("to_account_id", account.ID),
				zap.Bool("clean_thought_signature", true),
			)
			body = service.CleanGeminiNativeThoughtSignatures(body)
			sessionBoundAccountID = account.ID
		} else if sessionKey != "" && sessionBoundAccountID == 0 && !cleanedForUnknownBinding && bytes.Contains(body, []byte(`"thoughtSignature"`)) {
			// 无缓存绑定但请求里已有 thoughtSignature：常见于缓存丢失/TTL 过期后，客户端继续携带旧签名。
			// 为避免第一次转发就 400，这里做一次确定性清理，让新账号重新生成签名链路。
			reqLog.Info("gemini.sticky_session_binding_missing",
				zap.Bool("clean_thought_signature", true),
			)
			body = service.CleanGeminiNativeThoughtSignatures(body)
			cleanedForUnknownBinding = true
			sessionBoundAccountID = account.ID
		} else if sessionBoundAccountID == 0 {
			// 记录本次请求中首次选择到的账号，便于同一请求内 failover 时检测切换。
			sessionBoundAccountID = account.ID
		}

		// 4) account concurrency slot
		accountReleaseFunc := selection.ReleaseFunc
		if !selection.Acquired {
			if selection.WaitPlan == nil {
				markOpsRoutingCapacityLimited(c)
				googleError(c, http.StatusServiceUnavailable, "No available Gemini accounts")
				return
			}
			accountWaitCounted := false
			canWait, err := geminiConcurrency.IncrementAccountWaitCount(c.Request.Context(), account.ID, selection.WaitPlan.MaxWaiting)
			if err != nil {
				reqLog.Warn("gemini.account_wait_counter_increment_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			} else if !canWait {
				reqLog.Info("gemini.account_wait_queue_full",
					zap.Int64("account_id", account.ID),
					zap.Int("max_waiting", selection.WaitPlan.MaxWaiting),
				)
				googleError(c, http.StatusTooManyRequests, "Too many pending requests, please retry later")
				return
			}
			if err == nil && canWait {
				accountWaitCounted = true
			}
			defer func() {
				if accountWaitCounted {
					geminiConcurrency.DecrementAccountWaitCount(c.Request.Context(), account.ID)
				}
			}()

			accountReleaseFunc, err = geminiConcurrency.AcquireAccountSlotWithWaitTimeout(
				c,
				account.ID,
				selection.WaitPlan.MaxConcurrency,
				selection.WaitPlan.Timeout,
				stream,
				&streamStarted,
			)
			if err != nil {
				reqLog.Warn("gemini.account_slot_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
				googleError(c, http.StatusTooManyRequests, err.Error())
				return
			}
			if accountWaitCounted {
				geminiConcurrency.DecrementAccountWaitCount(c.Request.Context(), account.ID)
				accountWaitCounted = false
			}
		}
		// 终检与准入后绑定使用选号结果携带的门（见 responses 同名注释）。
		admissionCtx := service.ContextWithSelectionProfitGate(c.Request.Context(), selection)
		latest, vetoed, reason := h.gatewayService.GatewayProfitControlVetoLatest(admissionCtx, account)
		if vetoed {
			if accountReleaseFunc != nil {
				accountReleaseFunc()
			}
			reqLog.Debug("gemini.account_slot_profit_vetoed", zap.Int64("account_id", account.ID), zap.String("reason", reason))
			if fs.RecordProfitVeto(account.ID) == FailoverExhausted {
				reqLog.Warn("gemini.profit_veto_attempts_exhausted", zap.Int("profit_veto_count", fs.ProfitVetoCount()))
				markOpsRoutingCapacityLimited(c)
				googleError(c, http.StatusServiceUnavailable, profitVetoExhaustedMessage)
				return
			}
			continue
		}
		account = latest
		selection.Account = latest
		// 等待路径保持既有 eager 绑定（无门时 helper 直接绑定）；调度器已抢槽
		// 的直达路径无门时由选号内部绑定，这里只在门下补准入后绑定。
		if selection.ProfitGateActive() || !selection.Acquired {
			if err := h.gatewayService.BindStickySessionAfterProfitAdmission(admissionCtx, apiKey.GroupID, sessionKey, account.ID); err != nil {
				reqLog.Warn("gemini.bind_sticky_session_after_profit_admission_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			}
		}
		// 账号槽位/等待计数需要在超时或断开时安全回收
		accountReleaseFunc = wrapReleaseOnDone(c.Request.Context(), accountReleaseFunc)

		// 5) forward (根据平台分流)
		var result *service.ForwardResult
		requestCtx := c.Request.Context()
		if fs.SwitchCount > 0 {
			requestCtx = service.WithAccountSwitchCount(requestCtx, fs.SwitchCount, h.metadataBridgeEnabled())
		}
		sessionGroupID := derefGroupID(apiKey.GroupID)
		if account.Platform == service.PlatformAntigravity && account.Type != service.AccountTypeAPIKey {
			result, err = h.antigravityGatewayService.ForwardGemini(
				requestCtx,
				c,
				account,
				modelName,
				action,
				stream,
				body,
				hasBoundSession,
				service.WithForwardGeminiSession(sessionGroupID, sessionKey),
			)
		} else {
			result, err = h.geminiCompatService.ForwardNative(requestCtx, c, account, modelName, action, stream, body)
		}
		if accountReleaseFunc != nil {
			accountReleaseFunc()
		}
		if result != nil {
			usageInput := &service.RecordUsageInput{
				Result: result, APIKey: apiKey, User: apiKey.User, Account: account,
				Subscription: subscription, PricingAt: pricingAt,
				QuotaPlatform:   service.QuotaPlatform(c.Request.Context(), apiKey),
				InboundEndpoint: GetInboundEndpoint(c), UpstreamEndpoint: GetUpstreamEndpoint(c, account.Platform),
				UserAgent: c.GetHeader("User-Agent"), IPAddress: ip.GetClientIP(c),
				RequestPayloadHash: service.HashUsageRequestPayload(body), ForceCacheBilling: fs.ForceCacheBilling,
				APIKeyService: h.apiKeyService, SessionID: service.ExtractClientSessionID(c),
				ChannelUsageFields: clientRequestedUsageFields(c, channelMapping, reqModel, result.UpstreamModel),
			}
			h.submitUsageRecordTask(c.Request.Context(), func(ctx context.Context) {
				if recordErr := h.gatewayService.RecordUsage(ctx, usageInput); recordErr != nil {
					logger.L().With(
						zap.String("component", "handler.gemini_v1beta.models"),
						zap.Int64("user_id", usageInput.User.ID), zap.Int64("api_key_id", usageInput.APIKey.ID),
						zap.Any("group_id", usageInput.APIKey.GroupID), zap.String("model", usageInput.Result.Model),
						zap.Int64("account_id", usageInput.Account.ID),
					).Error("gemini.record_usage_failed", zap.Error(recordErr))
				}
			})
		}
		if err != nil {
			h.gatewayService.InvalidateUpstreamAffinity(c.Request.Context(), apiKey.GroupID, sessionKey, account, err)
			var failoverErr *service.UpstreamFailoverError
			if result == nil && errors.As(err, &failoverErr) {
				failoverAction := fs.HandleFailoverError(c.Request.Context(), h.gatewayService, account.ID, account.Platform, account.GetPoolModeRetryCount(), failoverErr)
				switch failoverAction {
				case FailoverContinue:
					continue
				case FailoverExhausted:
					h.handleGeminiFailoverExhausted(c, fs.LastFailoverErr)
					return
				case FailoverCanceled:
					failoverClientGone(c)
					return
				}
			}
			// ForwardNative already wrote the response
			logGatewayForwardFailure(reqLog, c, "gemini.forward_failed", err, zap.Int64("account_id", account.ID))
			return
		}

		// 保存 Gemini 内容摘要会话（用于 Fallback 匹配）
		if useDigestFallback && geminiDigestChain != "" && geminiPrefixHash != "" {
			if err := h.gatewayService.SaveGeminiSession(
				c.Request.Context(),
				derefGroupID(apiKey.GroupID),
				geminiPrefixHash,
				geminiDigestChain,
				geminiSessionUUID,
				account.ID,
				matchedDigestChain,
			); err != nil {
				reqLog.Warn("gemini.digest_session_save_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			}
		}

		reqLog.Debug("gemini.request_completed",
			zap.Int64("account_id", account.ID),
			zap.Int("switch_count", fs.SwitchCount),
		)
		return
	}
}

func parseGeminiModelAction(rest string) (model string, action string, err error) {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", "", &pathParseError{"missing path"}
	}

	// Standard: {model}:{action}
	if i := strings.Index(rest, ":"); i > 0 && i < len(rest)-1 {
		return rest[:i], rest[i+1:], nil
	}

	// Fallback: {model}/{action}
	if i := strings.Index(rest, "/"); i > 0 && i < len(rest)-1 {
		return rest[:i], rest[i+1:], nil
	}

	return "", "", &pathParseError{"invalid model action path"}
}

func (h *GatewayHandler) handleGeminiFailoverExhausted(c *gin.Context, failoverErr *service.UpstreamFailoverError) {

	if failoverErr != nil {
		defer service.GuardUpstreamFinancialError(c, failoverErr.StatusCode, failoverErr.ResponseBody)()
	}
	if failoverErr == nil {
		googleError(c, http.StatusBadGateway, "Upstream request failed")
		return
	}

	statusCode := failoverErr.StatusCode
	responseBody := failoverErr.ResponseBody
	if service.IsUpstreamCapacityCoolingBody(responseBody) {
		c.Header("Retry-After", "30")
		status, _, message := wrapUpstreamClientError(statusCode, responseBody)
		if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
			status = http.StatusServiceUnavailable
		}
		googleError(c, status, message)
		return
	}

	// 先检查透传规则
	if h.errorPassthroughService != nil && len(responseBody) > 0 {
		if rule := h.errorPassthroughService.MatchRule(service.PlatformGemini, statusCode, responseBody); rule != nil {
			// 确定响应状态码
			respCode := statusCode
			if !rule.PassthroughCode && rule.ResponseCode != nil {
				respCode = *rule.ResponseCode
			}

			// 确定响应消息
			msg := service.ExtractUpstreamErrorMessage(responseBody)
			if !rule.PassthroughBody && rule.CustomMessage != nil {
				msg = *rule.CustomMessage
			}

			if rule.SkipMonitoring {
				c.Set(service.OpsSkipPassthroughKey, true)
			}

			defer service.GuardUpstreamFinancialError(c, respCode, []byte(msg))()
			googleError(c, respCode, msg)
			return
		}
	}

	// 记录原始上游状态码，以便 ops 错误日志捕获真实的上游错误
	upstreamMsg := service.ExtractUpstreamErrorMessage(responseBody)
	service.SetOpsUpstreamError(c, statusCode, upstreamMsg, "")

	// 使用默认的错误映射
	status, _, message := wrapUpstreamClientError(statusCode, responseBody)
	status, _, message = exhaustedFailoverResponse(status, "", message)
	googleError(c, status, message)
}

type pathParseError struct{ msg string }

func (e *pathParseError) Error() string { return e.msg }

func googleError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    status,
			"message": message,
			"status":  googleapi.HTTPStatusToGoogleStatus(status),
		},
	})
}

func writeUpstreamResponse(c *gin.Context, res *service.UpstreamHTTPResult) {
	if res == nil {
		googleError(c, http.StatusBadGateway, "Empty upstream response")
		return
	}
	for k, vv := range res.Headers {
		// Avoid overriding content-length and hop-by-hop headers.
		if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Transfer-Encoding") || strings.EqualFold(k, "Connection") {
			continue
		}
		for _, v := range vv {
			c.Writer.Header().Add(k, v)
		}
	}
	contentType := res.Headers.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(res.StatusCode, contentType, res.Body)
}

// extractGeminiCLISessionHash 从 Gemini CLI 请求中提取会话标识。
// 组合 x-gemini-api-privileged-user-id header 和请求体中的 tmp 目录哈希。
//
// 会话标识生成策略：
//  1. 从请求体中提取 tmp 目录哈希（64位十六进制）
//  2. 从 header 中提取 privileged-user-id（UUID）
//  3. 组合两者生成 SHA256 哈希作为最终的会话标识
//
// 如果找不到 tmp 目录哈希，返回空字符串（不使用粘性会话）。
//
// extractGeminiCLISessionHash extracts session identifier from Gemini CLI requests.
// Combines x-gemini-api-privileged-user-id header with tmp directory hash from request body.
func extractGeminiCLISessionHash(c *gin.Context, body []byte) string {
	// 1. 从请求体中提取 tmp 目录哈希
	match := geminiCLITmpDirRegex.FindSubmatch(body)
	if len(match) < 2 {
		return "" // 没有找到 tmp 目录，不使用粘性会话
	}
	tmpDirHash := string(match[1])

	// 2. 提取 privileged-user-id
	privilegedUserID := strings.TrimSpace(c.GetHeader("x-gemini-api-privileged-user-id"))

	// 3. 组合生成最终的 session hash
	if privilegedUserID != "" {
		// 组合两个标识符：privileged-user-id + tmp 目录哈希
		combined := privilegedUserID + ":" + tmpDirHash
		hash := sha256.Sum256([]byte(combined))
		return hex.EncodeToString(hash[:])
	}

	// 如果没有 privileged-user-id，直接使用 tmp 目录哈希
	return tmpDirHash
}

// truncateDigestChain 截断摘要链用于日志显示
func truncateDigestChain(chain string) string {
	if len(chain) <= 50 {
		return chain
	}
	return chain[:50] + "..."
}

// safeShortPrefix 返回字符串前 n 个字符；长度不足时返回原字符串。
// 用于日志展示，避免切片越界。
func safeShortPrefix(value string, n int) string {
	if n <= 0 || len(value) <= n {
		return value
	}
	return value[:n]
}

// derefGroupID 安全解引用 *int64，nil 返回 0
func derefGroupID(groupID *int64) int64 {
	if groupID == nil {
		return 0
	}
	return *groupID
}

func servesCompositeAntigravityGemini(apiKey *service.APIKey, effectivePlatform string) bool {
	return apiKey.Group != nil && apiKey.Group.Platform == service.PlatformComposite && effectivePlatform == service.PlatformAntigravity
}

func isAntigravityGeminiModel(model string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(model, "models/")))
	if trimmed == "" || strings.HasPrefix(trimmed, "claude-") {
		return false
	}
	detected, ok := service.DetectModelPlatform(trimmed)
	return ok && detected == service.PlatformGemini
}
