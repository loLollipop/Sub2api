//go:build unit

package handler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Run the actual Messages handler, upstream conversion and usage settlement.
// Both accounts remain eligible: a replay would make a second upstream call.
func TestGatewayHandlerGeminiPartialResultIsTerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, protocol := range []string{"gemini", "responses"} {
		t.Run(protocol, func(t *testing.T) {
			const model = "gemini-3.7-flash"
			f := newSingleAccountProbeFixture(t)
			f.group.RateMultiplier = 1
			f.group.CrossProtocolConversionEnabled = true
			cfg := &config.Config{RunMode: config.RunModeStandard}
			cfg.Default.RateMultiplier = 1
			cfg.Gateway.MaxSameAccountRetriesGemini = 1
			cfg.Gateway.MaxLineSize = 1024
			accounts := f.schedulerCache.accounts
			second := *accounts[0]
			second.ID++
			second.Priority++
			second.AccountGroups = []service.AccountGroup{{AccountID: second.ID, GroupID: f.group.ID}}
			accounts = append(accounts, &second)
			f.schedulerCache.accounts = accounts
			upstream := &passthroughFailureHandlerUpstream{}
			stream := true
			wantTokens := 2
			if protocol == "responses" {
				for _, account := range accounts {
					account.Credentials = map[string]any{"api_key": "synthetic-key", "base_url": "https://api.openai.com", "api_protocol": service.APIProtocolResponses}
				}
				// Buffered response.failed carries diagnostic usage and a typed
				// failover error before any client output has been written.
				stream = false
				wantTokens = 3
				upstream.wire = `data: {"type":"response.failed","response":{"id":"resp_partial","object":"response","model":"gemini-3.7-flash","status":"failed","output":[],"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13},"error":{"code":"server_error","message":"An error occurred while processing your request."}}}` + "\n\n"
			} else {
				upstream.wire = `data: {"candidates":[{"content":{"parts":[{"text":"partial answer"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":2}}` + "\n\n" +
					"data: " + strings.Repeat("x", 4096) + "\n\n" +
					`data: {"usageMetadata":{"promptTokenCount":999,"candidatesTokenCount":999}}` + "\n\n"
			}
			usage := &passthroughFailureHandlerUsage{}
			ledger := &passthroughFailureHandlerBilling{}
			billing := service.NewBillingService(cfg, nil)
			snapshot := service.NewSchedulerSnapshotService(f.schedulerCache, nil, nil, nil, nil)
			f.handler.gatewayService = service.NewGatewayService(
				nil, &fakeGroupRepo{group: f.group}, usage, ledger, nil, nil, nil, nil, cfg,
				snapshot, nil, billing, nil, f.handler.billingCacheService, nil,
				upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			)
			f.handler.geminiCompatService = service.NewGeminiMessagesCompatService(nil, nil, nil, nil, nil, nil, upstream, nil, cfg)
			f.handler.openAIGatewayService = service.NewOpenAIGatewayService(
				nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, billing, nil,
				f.handler.billingCacheService, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
			)
			f.handler.cfg = cfg
			f.handler.maxAccountSwitchesGemini = 2
			streamValue := "false"
			if stream {
				streamValue = "true"
			}
			body := `{"model":"` + model + `","stream":` + streamValue + `,"max_tokens":32,"messages":[{"role":"user","content":"hello"}]}`
			rec, _ := f.serve(t, EndpointMessages, EndpointMessages, body, f.handler.Messages)
			require.EqualValues(t, 1, upstream.calls.Load(), rec.Body.String())
			require.Len(t, usage.logs, 1)
			require.Len(t, ledger.commands, 1)
			require.Equal(t, 10, usage.logs[0].InputTokens)
			require.Equal(t, wantTokens, usage.logs[0].OutputTokens)
			require.NotContains(t, rec.Body.String(), "999")
			if protocol == "responses" {
				require.Equal(t, http.StatusBadGateway, rec.Code)
				require.Zero(t, usage.logs[0].ActualCost)
				require.Zero(t, ledger.commands[0].BalanceCost)
			} else {
				require.Equal(t, http.StatusOK, rec.Code)
				require.Contains(t, rec.Body.String(), "partial answer")
				require.Equal(t, 1, strings.Count(rec.Body.String(), "event: message_start\n"))
				require.Greater(t, ledger.commands[0].BalanceCost, 0.0)
			}
		})
	}
}
