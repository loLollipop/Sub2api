package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// deepSeekToolsDefaultModel is the model used to pick an account for the
// standalone Search/Fetch endpoints when the client omits a routing model.
const deepSeekToolsDefaultModel = "deepseek-v4-flash"

// DeepSeekToolsSearch / DeepSeekToolsFetch proxy DeepSeek's standalone tool
// endpoints (/v1/tools/search, /v1/tools/fetch). They are raw JSON endpoints
// with Bearer auth, so the client body is forwarded as-is and the upstream
// successful response is passed through. Each 2xx is billed once using the
// group search price per 1,000 calls.
func (h *OpenAIGatewayHandler) DeepSeekToolsSearch(c *gin.Context) {
	h.deepSeekTools(c, service.DeepSeekToolsPathSearch, service.DeepSeekToolsLabelSearch)
}

func (h *OpenAIGatewayHandler) DeepSeekToolsFetch(c *gin.Context) {
	h.deepSeekTools(c, service.DeepSeekToolsPathFetch, service.DeepSeekToolsLabelFetch)
}

func (h *OpenAIGatewayHandler) deepSeekTools(c *gin.Context, toolPath string, modelLabel string) {
	streamStarted := false
	defer h.recoverResponsesPanic(c, &streamStarted)
	setOpenAIClientTransportHTTP(c)
	requestStart := time.Now()
	c.Request = c.Request.WithContext(h.gatewayService.PrepareSchedulerRequestContext(c.Request.Context()))

	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.Group == nil {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	if apiKey.Group.Platform != service.PlatformDeepseek {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "DeepSeek tools are only available for DeepSeek groups")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	reqLog := requestLogger(
		c,
		"handler.openai_gateway.deepseek_tools",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
		zap.String("tool_path", toolPath),
	)
	if !h.ensureResponsesDependencies(c, reqLog) {
		return
	}

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	if len(body) == 0 {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}
	routingModel, auditBody, err := parseDeepSeekToolsRequest(body, c.Query("model"), toolPath)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	setOpsRequestContext(c, modelLabel, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))
	setActualUpstreamEndpoint(c, toolPath)
	if decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIChat, routingModel, auditBody); decision != nil && !decision.AllowNextStage {
		h.openAISecurityAuditError(c, decision)
		return
	}

	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())
	asPricingCtx, asPricingAt := h.attachTextPricingContext(c, apiKey.GroupID)
	c.Request = c.Request.WithContext(asPricingCtx)

	userRelease, acquired := h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, false, &streamStarted, reqLog)
	if !acquired {
		return
	}
	if userRelease != nil {
		defer userRelease()
	}

	if err := h.billingCacheService.CheckBillingEligibility(asPricingCtx, apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(asPricingCtx, apiKey)); err != nil {
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.errorResponse(c, status, code, message)
		return
	}

	sessionHash := h.gatewayService.GenerateSessionHashWithFallback(c, nil, "")
	failedAccountIDs := make(map[int64]struct{})
	var lastFailoverErr *service.UpstreamFailoverError
	profitVetoCount := 0
	accountSlotBusySeen := false
	switchCount := 0
	routingStart := time.Now()
	channelMapping := service.ChannelMappingResult{}

	for {
		selection, _, routedKey, err := h.gatewayService.SelectAccountWithSchedulerForCapabilityAlongKeyRoutes(
			c.Request.Context(),
			apiKey,
			"",
			sessionHash,
			routingModel,
			failedAccountIDs,
			service.OpenAIUpstreamTransportHTTPSSE,
			service.OpenAIEndpointCapabilityDeepSeekTools,
			false,
			false,
			false,
			service.PlatformDeepseek,
		)
		if err == nil && routedKey != nil {
			if routedKey.Group == nil || routedKey.Group.Platform != service.PlatformDeepseek {
				releaseRejectedKeyRouteSelection(selection)
				h.errorResponse(c, http.StatusNotFound, "not_found_error", "DeepSeek tools require a DeepSeek group")
				return
			}
			if bindErr := h.bindSelectedKeyRoute(c, keyRouteBinding{
				Previous: apiKey, Selected: routedKey, Subscription: &subscription,
				Mapping: &channelMapping, Body: body, Model: routingModel, PricingAt: asPricingAt,
			}); bindErr != nil {
				releaseRejectedKeyRouteSelection(selection)
				h.handlePreauthorizationError(c, bindErr, false)
				return
			}
			apiKey = routedKey
		}
		if err != nil || selection == nil || selection.Account == nil {
			if failoverClientGone(c) {
				reqLog.Info("deepseek_tools.account_select_aborted_client_disconnected", zap.Error(err))
				return
			}
			reqLog.Warn("deepseek_tools.select_failed",
				zap.String("tool_path", toolPath),
				zap.String("routing_model", routingModel),
				zap.Int("failed_accounts", len(failedAccountIDs)),
				zap.Error(err),
			)
			if lastFailoverErr != nil {
				h.handleFailoverExhausted(c, lastFailoverErr, false)
			} else if accountSlotBusySeen {
				h.handleOpenAIAccountSlotBusyExhausted(c, streamStarted, reqLog)
			} else {
				cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, routingModel, routingModel, service.PlatformDeepseek)
				cls = classifySelectionFailureErrorFromGin(c, err, cls)
				if !cls.ModelNotFound {
					markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
				}
				h.errorResponse(c, cls.Status, cls.ErrType, cls.Message)
			}
			return
		}

		account := selection.Account
		setOpsSelectedAccount(c, account.ID, account.Platform, account.Name)
		accountRelease, slotResult := h.acquireResponsesAccountSlot(c, apiKey.GroupID, sessionHash, selection, false, &streamStarted, reqLog)
		switch h.openAISlotLoopAction(c, slotResult, account.ID, apiKey.GroupID, sessionHash, failedAccountIDs, &profitVetoCount, streamStarted, reqLog, &accountSlotBusySeen) {
		case openAISlotLoopStop:
			return
		case openAISlotLoopRotate:
			continue
		}
		service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())
		writerSizeBeforeForward := c.Writer.Size()
		admissionRejected := false
		forwardStart := time.Now()
		var result *service.OpenAIForwardResult
		result, err = func() (*service.OpenAIForwardResult, error) {
			if accountRelease != nil {
				defer accountRelease()
			}
			// Reserve against the selected key route, whose group sets the final price.
			// The usage task takes its own hold before this attempt releases its reference.
			previousRequest := c.Request
			done, reserveErr := reserveInflightBalance(c, h.billingCacheService, service.DeepSeekToolsReservationEstimator{Gateway: h.gatewayService}, apiKey, subscription, service.InflightEstimateRequest{Model: modelLabel, Kind: service.InflightEstimatePerRequest, SearchCalls: 1})
			if reserveErr != nil {
				admissionRejected = true
				status, code, message, retryAfter := billingErrorDetails(reserveErr)
				if retryAfter > 0 {
					c.Header("Retry-After", strconv.Itoa(retryAfter))
				}
				h.errorResponse(c, status, code, message)
				return nil, reserveErr
			}
			defer done()
			defer func() { c.Request = previousRequest }()
			result, forwardErr := h.gatewayService.ForwardDeepSeekTools(c.Request.Context(), c, account, toolPath, body)
			if forwardErr == nil && result != nil {
				h.recordDeepSeekToolsUsage(c, reqLog, apiKey, account, subscription, body, result, subject.UserID, asPricingAt)
			}
			return result, forwardErr
		}()
		service.SetOpsLatencyMs(c, service.OpsResponseLatencyMsKey, time.Since(forwardStart).Milliseconds())
		if admissionRejected {
			return
		}

		if err == nil {
			if result != nil {
				h.gatewayService.ReportOpenAIAccountScheduleResult(account, openAIAccountScheduleModel(c, account, routingModel, false, nil), true, nil)
			}
			return
		}

		if failoverClientGone(c) {
			return
		}
		h.gatewayService.InvalidateUpstreamAffinity(c.Request.Context(), apiKey.GroupID, sessionHash, account, err)
		var failoverErr *service.UpstreamFailoverError
		if !errors.As(err, &failoverErr) {
			h.gatewayService.ReportOpenAIAccountScheduleResult(account, openAIAccountScheduleModel(c, account, routingModel, false, nil), false, nil, err)
			if c.Writer.Size() == writerSizeBeforeForward {
				h.errorResponse(c, http.StatusBadGateway, "upstream_error", "DeepSeek tools upstream request failed")
			}
			reqLog.Warn("deepseek_tools.forward_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			return
		}
		h.gatewayService.ReportOpenAIAccountScheduleResult(account, openAIAccountScheduleModel(c, account, routingModel, false, nil), false, nil, err)
		h.gatewayService.RecordOpenAIAccountSwitch()
		failedAccountIDs[account.ID] = struct{}{}
		lastFailoverErr = failoverErr
		if !fillSchedulingSwitchAllowed(switchCount, h.maxAccountSwitches) {
			h.handleFailoverExhausted(c, failoverErr, false)
			return
		}
		switchCount++
		reqLog.Warn("deepseek_tools.upstream_failover_switching",
			zap.Int64("account_id", account.ID),
			zap.Int("upstream_status", failoverErr.StatusCode),
			zap.Int("switch_count", switchCount),
		)
	}
}

// recordDeepSeekToolsUsage 为一次成功的 Search/Fetch 落按次计费用量行。
// SearchOnly + SearchCount=1 uses the group search price per 1,000 calls.
func (h *OpenAIGatewayHandler) recordDeepSeekToolsUsage(
	c *gin.Context,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	account *service.Account,
	subscription *service.UserSubscription,
	body []byte,
	result *service.OpenAIForwardResult,
	userID int64,
	pricingAt time.Time,
) {
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	sessionID := service.ExtractClientSessionID(c)
	requestPayloadHash := service.HashUsageRequestPayload(body)
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := result.UpstreamEndpoint
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)

	h.submitMandatoryUsageRecordTask(c.Request.Context(), func(ctx context.Context) {
		if err := h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
			Result:             result,
			APIKey:             apiKey,
			User:               apiKey.User,
			Account:            account,
			Subscription:       subscription,
			InboundEndpoint:    inboundEndpoint,
			UpstreamEndpoint:   upstreamEndpoint,
			UserAgent:          userAgent,
			IPAddress:          clientIP,
			RequestPayloadHash: requestPayloadHash,
			APIKeyService:      h.apiKeyService,
			QuotaPlatform:      quotaPlatform,
			SessionID:          sessionID,
			PricingAt:          pricingAt,
		}); err != nil {
			reqLog.Error("deepseek_tools.record_usage_failed",
				zap.Int64("user_id", userID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Any("group_id", apiKey.GroupID),
				zap.Int64("account_id", account.ID),
				zap.Error(err),
			)
		}
	})
}

// parseDeepSeekToolsRequest validates only the stable fields, keeping all client
// bytes intact for forwarding. Moderation receives a chat-shaped user message
// containing the real query/URL rather than attempting to parse raw tool JSON.
func parseDeepSeekToolsRequest(body []byte, queryModel, toolPath string) (string, []byte, error) {
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return "", nil, fmt.Errorf("request body must be a JSON object")
	}
	// Preserve raw forwarding only when all routing and audited fields are
	// unambiguous across first/last-value and case-insensitive JSON binders.
	seen := make(map[string]bool, 3)
	var fieldErr error
	gjson.ParseBytes(body).ForEach(func(key, _ gjson.Result) bool {
		name := key.String() // Decode escaped keys before comparing them.
		canonical := strings.ToLower(name)
		switch canonical {
		case "query", "url", "model":
			if name != canonical || seen[canonical] {
				fieldErr = fmt.Errorf("%s must appear once with its lowercase name", canonical)
				return false
			}
			seen[canonical] = true
		}
		return true
	})
	if fieldErr != nil {
		return "", nil, fieldErr
	}
	field := "query"
	if toolPath == service.DeepSeekToolsPathFetch {
		field = "url"
	}
	value := gjson.GetBytes(body, field)
	if value.Type != gjson.String || strings.TrimSpace(value.String()) == "" {
		return "", nil, fmt.Errorf("%s must be a non-empty string", field)
	}
	if openAIConflictingModels("application/json", body) {
		return "", nil, service.ErrDuplicateModelField
	}
	model := strings.TrimSpace(queryModel)
	if v := gjson.GetBytes(body, "model"); v.Exists() {
		if v.Type != gjson.String || strings.TrimSpace(v.String()) == "" {
			return "", nil, fmt.Errorf("model must be a non-empty string")
		}
		model = strings.TrimSpace(v.String())
	}
	if model == "" {
		model = deepSeekToolsDefaultModel
	}
	auditBody, err := json.Marshal(map[string]any{
		"messages": []map[string]string{{"role": "user", "content": value.String()}},
	})
	return model, auditBody, err
}
