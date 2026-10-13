package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// Connectivity tests use the live dispatcher and its bounded refusal policy.
// They test exactly the selected model and never persist capability guesses.
func (s *AccountTestService) testMixedAccountConnection(c *gin.Context, account *Account, model, prompt string) error {
	if err := validateMixedAccount(account); err != nil {
		return s.sendErrorAndEnd(c, err.Error())
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return s.sendErrorAndEnd(c, "Select a model to test")
	}
	if s.openaiGatewayService == nil {
		return s.sendErrorAndEnd(c, "Mixed gateway is unavailable")
	}
	if strings.TrimSpace(prompt) == "" {
		prompt = "Hi"
	}
	body, err := json.Marshal(map[string]any{
		"model": model, "stream": false, "max_tokens": 128,
		"messages": []map[string]string{{"role": "user", "content": prompt}},
	})
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to build test request")
	}
	recorder := httptest.NewRecorder()
	forwardContext, _ := gin.CreateTestContext(recorder)
	forwardContext.Request, err = http.NewRequestWithContext(c.Request.Context(), http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to build test request")
	}
	s.sendEvent(c, TestEvent{Type: "test_start", Model: model})
	result, err := s.openaiGatewayService.ForwardAsChatCompletions(c.Request.Context(), forwardContext, account, body, "", "")
	if err != nil || result == nil || recorder.Code != http.StatusOK {
		return s.sendErrorAndEnd(c, "Mixed model test failed at "+GetActualOpenAIUpstreamEndpoint(forwardContext))
	}
	s.sendEvent(c, TestEvent{Type: "content", Text: "Upstream: " + GetActualOpenAIUpstreamEndpoint(forwardContext) + "\n"})
	text := gjson.GetBytes(recorder.Body.Bytes(), "choices.0.message.content").String()
	if text != "" {
		s.sendEvent(c, TestEvent{Type: "content", Text: text})
	}
	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}
