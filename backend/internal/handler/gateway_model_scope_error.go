package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// A scope refusal is local authorization. Handle it before affinity, health,
// scheduler scoring or failover can attribute it to an upstream account.
func (h *OpenAIGatewayHandler) handleModelScopeError(c *gin.Context, err error, streamStarted bool) bool {
	if !errors.Is(err, service.ErrModelScopeDenied) {
		return false
	}
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalPolicyDenied)
	if service.OpenAIImagesJSONKeepalivePresent(c) {
		// Images heartbeats commit JSON headers but write only whitespace,
		// not SSE. Stop them before inspecting or writing the final document.
		committed := service.StopOpenAIImagesJSONKeepaliveCommitted(c)
		if service.IsResponseCommitted(c) || service.OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c) >= 0 {
			return true
		}
		status := http.StatusForbidden
		if committed {
			status = c.Writer.Status()
			service.MarkOpsStreamError(c, "permission_error", service.ErrModelScopeDenied.Message, http.StatusForbidden)
		}
		c.JSON(status, gin.H{"error": gin.H{
			"type": "permission_error", "code": "MODEL_SCOPE_DENIED", "message": service.ErrModelScopeDenied.Message,
		}})
		return true
	}
	if c.Request != nil && strings.HasSuffix(strings.TrimRight(c.Request.URL.Path, "/"), "/messages") {
		h.anthropicStreamingAwareError(c, http.StatusForbidden, "permission_error", service.ErrModelScopeDenied.Message, streamStarted || c.Writer.Written())
		return true
	}
	h.handleStreamingAwareErrorWithCode(c, http.StatusForbidden, "permission_error", "MODEL_SCOPE_DENIED", service.ErrModelScopeDenied.Message, streamStarted || c.Writer.Written(), false)
	return true
}

func (h *GatewayHandler) handleModelScopeError(c *gin.Context, err error, streamStarted bool) bool {
	if !errors.Is(err, service.ErrModelScopeDenied) {
		return false
	}
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalPolicyDenied)
	if c.Request != nil && strings.HasSuffix(strings.TrimRight(c.Request.URL.Path, "/"), "/chat/completions") {
		if service.StopOpenAICompactSSEKeepaliveCommitted(c) {
			streamStarted = true
		}
		if !streamStarted && !c.Writer.Written() {
			h.chatCompletionsErrorResponse(c, http.StatusForbidden, "permission_error", service.ErrModelScopeDenied.Message)
			return true
		}
		service.MarkOpsStreamError(c, "permission_error", service.ErrModelScopeDenied.Message, http.StatusForbidden)
		payload, _ := json.Marshal(gin.H{"error": gin.H{"type": "permission_error", "code": "MODEL_SCOPE_DENIED", "message": service.ErrModelScopeDenied.Message}})
		if _, writeErr := fmt.Fprintf(c.Writer, "data: %s\n\n", payload); writeErr != nil {
			_ = c.Error(writeErr)
		}
		c.Writer.Flush()
		return true
	}
	h.handleStreamingAwareErrorWithCode(c, http.StatusForbidden, "permission_error", "MODEL_SCOPE_DENIED", service.ErrModelScopeDenied.Message, streamStarted || c.Writer.Written())
	return true
}

func handleGeminiModelScopeError(c *gin.Context, err error, streamStarted bool) bool {
	if !errors.Is(err, service.ErrModelScopeDenied) {
		return false
	}
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalPolicyDenied)
	if !streamStarted && !c.Writer.Written() {
		googleError(c, http.StatusForbidden, service.ErrModelScopeDenied.Message)
		return true
	}
	service.MarkOpsStreamError(c, "permission_error", service.ErrModelScopeDenied.Message, http.StatusForbidden)
	payload, _ := json.Marshal(gin.H{"error": gin.H{"code": http.StatusForbidden, "status": "PERMISSION_DENIED", "message": service.ErrModelScopeDenied.Message}})
	if _, writeErr := fmt.Fprintf(c.Writer, "data: %s\n\n", payload); writeErr != nil {
		_ = c.Error(writeErr)
	}
	c.Writer.Flush()
	return true
}
