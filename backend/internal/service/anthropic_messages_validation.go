package service

import (
	"fmt"
	"math"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const messagesValidationFactsKey = "gateway.messages_original_validation_facts"

type messagesValidationFacts struct{ opusError string }

// CaptureMessagesValidationFacts records semantic validation before defaults,
// empty-block cleanup or account-specific compatibility can change the body.
// Enforcement waits until the actual mapped upstream model has been selected.
func CaptureMessagesValidationFacts(c *gin.Context, body []byte) {
	if c != nil {
		c.Set(messagesValidationFactsKey, messagesValidationFacts{opusError: strictOpusMessagesError(body)})
	}
}

func validateSelectedAnthropicMessagesRequest(c *gin.Context, body []byte, model string) error {
	if claude.IsOpus55(model) {
		message := strictOpusMessagesError(body)
		if c != nil {
			if value, exists := c.Get(messagesValidationFactsKey); exists {
				if facts, ok := value.(messagesValidationFacts); ok {
					message = facts.opusError
				}
			}
		}
		if message != "" {
			return fmt.Errorf("%s", message)
		}
	}
	// Preserve the existing enabled->adaptive compatibility gate for Opus and
	// Sonnet. No other original parameters are repaired to bypass validation.
	return validateClaude55Request(sanitizeAnthropicThinkingForModel(body, model), model)
}

func strictOpusMessagesError(body []byte) string {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() || len(messages.Array()) == 0 {
		return "field messages is required"
	}
	maxTokens := gjson.GetBytes(body, "max_tokens")
	if maxTokens.Type != gjson.Number || maxTokens.Float() < 1 || math.Trunc(maxTokens.Float()) != maxTokens.Float() {
		return "max_tokens: positive integer required"
	}
	if temperature := gjson.GetBytes(body, "temperature"); temperature.Exists() &&
		(temperature.Type != gjson.Number || temperature.Float() < 0 || temperature.Float() > 1) {
		return "temperature: range: 0..1"
	}
	for i, message := range messages.Array() {
		content := message.Get("content")
		if !content.IsArray() {
			continue
		}
		for j, block := range content.Array() {
			if block.Get("type").String() == "text" {
				text := block.Get("text")
				if text.Type != gjson.String || strings.TrimSpace(text.String()) == "" {
					return fmt.Sprintf("messages.%d.content.%d.text: text content blocks must contain non-whitespace text", i, j)
				}
			}
		}
	}
	// 5.5 has implicit adaptive thinking when omitted, too.
	for _, field := range []struct {
		name         string
		defaultValue float64
	}{{"temperature", 1}, {"top_p", 1}, {"top_k", 0}} {
		if value := gjson.GetBytes(body, field.name); value.Exists() && (value.Type != gjson.Number || value.Float() != field.defaultValue) {
			return fmt.Sprintf("%s may only be set to %v when thinking is enabled", field.name, field.defaultValue)
		}
	}
	return ""
}
