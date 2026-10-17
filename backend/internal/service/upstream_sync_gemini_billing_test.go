//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGeminiCompatForwardedEffortPreservesLocalRate(t *testing.T) {
	for _, tc := range []struct{ model, requested, forwarded string }{
		{"gemini-3.8-flash", "none", "minimal"},
		{"gemini-3-pro-preview", "medium", "high"},
		{"gemini-2.5-flash", "high", ""},
	} {
		t.Run(tc.model, func(t *testing.T) {
			upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":30,"candidatesTokenCount":10}}`))}}}
			svc := &GeminiMessagesCompatService{httpUpstream: upstream, cfg: &config.Config{}}
			body := []byte(`{"model":"` + tc.model + `","reasoning_effort":"` + tc.requested + `","messages":[{"role":"user","content":"hi"}]}`)
			c, _ := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)
			result, err := svc.ForwardAsChatCompletions(context.Background(), c, geminiCompatThinkingTestAccount(), body)
			require.NoError(t, err)
			require.Equal(t, tc.forwarded, optionalStringValue(result.ReasoningEffort))
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			billing := newGatewayRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})
			billing.billingService = NewBillingService(billing.cfg, &PricingService{pricingData: map[string]*LiteLLMModelPricing{tc.model: {InputCostPerToken: 1e-6, OutputCostPerToken: 4e-6}}})
			billing.resolver = NewModelPricingResolver(nil, billing.billingService)
			group := &Group{ID: 27, Platform: PlatformGemini, RateMultiplier: 0.15}
			err = billing.RecordUsage(context.Background(), &RecordUsageInput{Result: result, APIKey: &APIKey{ID: 501, GroupID: &group.ID, Group: group}, User: &User{ID: 601}, Account: geminiCompatThinkingTestAccount()})
			require.NoError(t, err)
			require.NotNil(t, usageRepo.lastLog)
			require.Greater(t, usageRepo.lastLog.TotalCost, 0.0)
			require.InDelta(t, usageRepo.lastLog.TotalCost*0.15, usageRepo.lastLog.ActualCost, 1e-12)
			require.Equal(t, tc.forwarded, optionalStringValue(usageRepo.lastLog.ReasoningEffort))
		})
	}
}
