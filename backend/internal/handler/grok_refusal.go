package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
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
	if !grokRefusalCheckRelevant(c, key, body, "") {
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

func grokRefusalCheckRelevant(c *gin.Context, key *service.APIKey, body []byte, model string) bool {
	if key == nil {
		return false
	}
	platform := openAICompatibleRequestPlatform(c.Request.Context(), key)
	if model == "" {
		model = gjson.GetBytes(body, "model").String()
	}
	// Mixed-route keys may eventually select Grok even when their primary group
	// is OpenAI. Strong identities are protocol/model independent.
	return platform == service.PlatformGrok || platform == service.PlatformComposite || strings.HasPrefix(strings.ToLower(model), "grok") || len(key.CandidateGroupIDs()) > 1
}

func checkGrokRefusedWebSocketRequest(c *gin.Context, gateway *service.OpenAIGatewayService, key *service.APIKey, body []byte, model string, updater *service.APIKeyService) error {
	// The first frame precedes composite resolution, whose normalized default
	// is OpenAI. A non-Grok model alias can still select a Grok account.
	composite := key != nil && key.Group != nil && key.Group.Platform == service.PlatformComposite
	if !composite && !grokRefusalCheckRelevant(c, key, body, model) {
		return nil
	}
	blocked, err := gateway.CheckGrokRefusalWebSocketRequest(c, key, body, model, updater)
	if err != nil {
		return service.NewOpenAIWSClientCloseError(coderws.StatusInternalError, "Unable to check refused conversation; retry later", err)
	}
	if blocked {
		return service.NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, service.GrokModerationRefusalMessage(), nil)
	}
	return nil
}
