package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
)

// Connection probes use the same mapped model and protocol selection as forwarding.
func (s *AccountTestService) testModelRoutedAccountConnection(c *gin.Context, account *Account, modelID, prompt string) error {
	requested := strings.TrimSpace(modelID)
	if requested == "" {
		requested = account.providerDefaultTestModel()
	}
	if requested == "" {
		requested = openai.DefaultTestModel
	}
	mapped := normalizeOpenAIModelForUpstream(account, account.GetMappedModel(requested))
	protocol := account.resolveModelRoutedProtocol(mapped)
	if s.openaiGatewayService != nil {
		protocol = s.openaiGatewayService.resolveUpstreamProtocolFor(c.Request.Context(), account, "", mapped)
	}
	switch protocol {
	case APIProtocolAnthropic:
		return s.testCNProviderAnthropicConnection(c, account, requested, prompt)
	case APIProtocolResponses:
		token := strings.TrimSpace(account.GetOpenAIProtocolAPIKey())
		if token == "" {
			return s.sendErrorAndEnd(c, "No API key available")
		}
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("X-Accel-Buffering", "no")
		c.Writer.Flush()
		s.sendEvent(c, TestEvent{Type: "test_start", Model: mapped})
		return s.testCNProviderAdaptiveResponsesConnection(c, account, mapped, token, prompt)
	default:
		return s.testCNProviderChatCompletionsConnection(c, account, requested, prompt)
	}
}
