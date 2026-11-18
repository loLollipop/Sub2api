//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func cacheSnapshotBoundaryResponse(stream, zero bool) string {
	usage := `{"input_tokens":10,"output_tokens":2,"cache_read_input_tokens":7,"cache_creation_input_tokens":11000,"cache_creation":{"ephemeral_1h_input_tokens":3000}}`
	if zero {
		usage = `{"input_tokens":0,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}`
	}
	message := `{"id":"msg_cache","type":"message","role":"assistant","model":"k3","content":[],"stop_reason":"end_turn","usage":` + usage + `}`
	if !stream {
		return message
	}
	start := `{"id":"msg_cache","type":"message","role":"assistant","model":"k3","content":[],"usage":{"input_tokens":10,"output_tokens":2,"cache_read_input_tokens":7,"cache_creation_input_tokens":3000,"cache_creation":{"ephemeral_1h_input_tokens":3000}}}`
	return "event: message_start\ndata: " + `{"type":"message_start","message":` + start + "}\n\n" +
		"event: message_delta\ndata: " + `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":` + usage + "}\n\n" +
		"event: message_delta\ndata: " + `{"type":"message_delta","usage":{"cache_creation":{"ephemeral_1h_input_tokens":3000}}}` + "\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
}

func assertCacheSnapshotRecordUsage(t *testing.T, claudeUsage *ClaudeUsage, nativeResult *OpenAIForwardResult, zero bool) {
	t.Helper()
	for _, customStats := range []bool{false, true} {
		usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
		billingRepo := &openAIRecordUsageBillingRepoStub{}
		pricing := NewBillingService(&config.Config{}, &PricingService{pricingData: map[string]*LiteLLMModelPricing{"k3": {
			InputCostPerToken: .001, OutputCostPerToken: .002, CacheReadInputTokenCost: .0001,
			CacheCreationInputTokenCost: .002, CacheCreationInputTokenCostAbove1hr: .004,
		}}})
		account := nativeAnthropicTestAccount()
		// Native Anthropic routes keep provider TTLs even when this account has an override.
		account.Extra = map[string]any{"cache_ttl_override_enabled": true, "cache_ttl_override_target": "5m"}
		channel := &Channel{ID: 1, Status: StatusActive}
		statsMultiplier := 1.0
		if customStats {
			statsMultiplier = 10
			channel.AccountStatsPricingRules = []AccountStatsPricingRule{{AccountIDs: []int64{account.ID}, Pricing: []ChannelModelPricing{{
				Platform: PlatformKimi, Models: []string{"k3"}, BillingMode: BillingModeToken,
				InputPrice: float64Ptr(.01), OutputPrice: float64Ptr(.02), CacheReadPrice: float64Ptr(.001), CacheWritePrice: float64Ptr(.02), CacheWrite1hPrice: float64Ptr(.04),
			}}}}
		}
		channels := newTestChannelServiceForStats(t, channel, 10, PlatformKimi)
		groupID := int64(10)
		key := &APIKey{ID: 501, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformKimi, RateMultiplier: .15}}
		user := &User{ID: 601}
		if nativeResult != nil {
			svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			svc.billingService, svc.channelService = pricing, channels
			require.NoError(t, svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{Result: nativeResult, APIKey: key, User: user, Account: account}))
		} else {
			svc := newGatewayRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})
			svc.billingService, svc.channelService = pricing, channels
			account.Extra = nil
			result := &ForwardResult{RequestID: "cache_billing", Model: "k3", UpstreamModel: "k3", Usage: *claudeUsage, Duration: time.Second}
			require.NoError(t, svc.RecordUsage(context.Background(), &RecordUsageInput{Result: result, APIKey: key, User: user, Account: account}))
		}
		wantCache, wantTotal := 28.0, 28.0147
		if zero {
			wantCache, wantTotal = 0, 0
		}
		require.NotNil(t, usageRepo.lastLog)
		require.InDelta(t, wantCache, usageRepo.lastLog.CacheCreationCost, 1e-12)
		require.InDelta(t, wantTotal, usageRepo.lastLog.TotalCost, 1e-12)
		require.InDelta(t, wantTotal*.15, usageRepo.lastLog.ActualCost, 1e-12)
		require.False(t, usageRepo.lastLog.CacheTTLOverridden)
		if !zero {
			require.Equal(t, 1, billingRepo.calls)
			require.InDelta(t, wantTotal*.15, billingRepo.lastCmd.BalanceCost, 1e-12)
			require.NotNil(t, usageRepo.lastLog.AccountStatsCost)
			require.InDelta(t, wantTotal*statsMultiplier, *usageRepo.lastLog.AccountStatsCost, 1e-12)
			require.Equal(t, 3000, usageRepo.lastLog.CacheCreation1hTokens)
			require.Equal(t, 11000, usageRepo.lastLog.CacheCreationTokens)
		}
	}
}

func TestNativeAnthropicCacheSnapshotForwardBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, route := range []string{"messages", "chat", "responses"} {
		for _, stream := range []bool{false, true} {
			for _, zero := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stream=%t/zero=%t", route, stream, zero), func(t *testing.T) {
					payload := cacheSnapshotBoundaryResponse(stream || route != "messages", zero)
					contentType := "text/event-stream"
					if !stream && route == "messages" {
						contentType = "application/json"
					}
					upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(payload))}}
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
					body := []byte(fmt.Sprintf(`{"model":"k3","max_tokens":32,"messages":[{"role":"user","content":"hi"}],"stream":%t,"stream_options":{"include_usage":true}}`, stream))
					path := "/v1/messages"
					if route == "chat" {
						path = "/v1/chat/completions"
					}
					if route == "responses" {
						path = "/v1/responses"
						body = []byte(fmt.Sprintf(`{"model":"k3","input":"hi","stream":%t}`, stream))
					}
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
					account := nativeAnthropicTestAccount()
					account.Extra = map[string]any{"cache_ttl_override_enabled": true, "cache_ttl_override_target": "5m"}
					var result *OpenAIForwardResult
					var err error
					switch route {
					case "messages":
						result, err = svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
					case "chat":
						result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
					case "responses":
						result, err = svc.Forward(context.Background(), c, account, body)
					}
					require.NoError(t, err)
					require.NotNil(t, result)
					wantWrite, wantInput := 11000, 11017
					if zero {
						wantWrite, wantInput = 0, 0
					}
					require.Equal(t, wantWrite, result.Usage.CacheCreationInputTokens)
					require.Equal(t, wantInput, result.Usage.InputTokens)
					if route == "messages" {
						require.Equal(t, payload, rec.Body.String())
					} else if !stream {
						inputPath := "usage.input_tokens"
						if route == "chat" {
							inputPath = "usage.prompt_tokens"
						}
						require.EqualValues(t, wantInput, gjson.Get(rec.Body.String(), inputPath).Int())
					}
					assertCacheSnapshotRecordUsage(t, nil, result, zero)
				})
			}
		}
	}
}

func TestAnthropicCacheSnapshotConvertedForwardBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, route := range []string{"standard", "passthrough", "responses", "buffered responses", "chat", "buffered chat"} {
		for _, zero := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/zero=%t", route, zero), func(t *testing.T) {
				payload := cacheSnapshotBoundaryResponse(true, zero)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(payload))}
				svc := &GatewayService{cfg: &config.Config{}, rateLimitService: &RateLimitService{}}
				var result *ForwardResult
				var err error
				switch route {
				case "standard", "passthrough":
					var streamed *streamingResult
					if route == "standard" {
						streamed, err = svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 701}, time.Now(), "k3", "k3", false)
					} else {
						streamed, err = svc.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 701}, time.Now(), "k3")
					}
					require.NotNil(t, streamed)
					result = &ForwardResult{Usage: *streamed.usage}
				case "responses":
					result, err = svc.handleResponsesStreamingResponse(resp, c, "k3", "k3", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
				case "buffered responses":
					result, err = svc.handleResponsesBufferedStreamingResponse(resp, c, "k3", "k3", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
				case "chat":
					result, err = svc.handleCCStreamingFromAnthropic(resp, c, "k3", "k3", nil, time.Now(), true)
				case "buffered chat":
					result, err = svc.handleCCBufferedFromAnthropic(resp, c, "k3", "k3", nil, time.Now())
				}
				require.NoError(t, err)
				require.NotNil(t, result)
				assertCacheSnapshotRecordUsage(t, &result.Usage, nil, zero)
			})
		}
	}
}
