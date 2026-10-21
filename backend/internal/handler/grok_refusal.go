package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"net/http"
	"strings"
)

// All text protocols use the same durable user-scoped check before selecting an
// account or reserving balance. No rate/usage pool can drop a refusal event.
func rejectGrokRefusedRequest(c *gin.Context, gateway *service.OpenAIGatewayService, key *service.APIKey, body []byte, updater *service.APIKeyService, write func(*gin.Context, int, string, string)) bool {
	if key == nil {
		return false
	}
	platform := openAICompatibleRequestPlatform(c.Request.Context(), key)
	// Mixed-route keys may eventually select Grok even when their primary group
	// is OpenAI. Strong identities are protocol/model independent.
	relevant := platform == service.PlatformGrok || platform == service.PlatformComposite || strings.HasPrefix(strings.ToLower(gjson.GetBytes(body, "model").String()), "grok") || len(key.CandidateGroupIDs()) > 1
	if !relevant {
		return false
	}
	blocked, err := gateway.CheckGrokRefusalRequest(c, key, body, updater)
	if err != nil {
		write(c, http.StatusServiceUnavailable, "api_error", "Unable to check refused conversation; retry later")
		return true
	}
	if blocked {
		write(c, http.StatusForbidden, "invalid_request_error", service.GrokModerationRefusalMessage())
		return true
	}
	return false
}
