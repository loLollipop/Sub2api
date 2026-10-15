//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIFinalResponsesRejectMissingRelayEndpoint(t *testing.T) {
	for _, platform := range []string{PlatformCommandCode, PlatformCline} {
		for _, passthrough := range []bool{false, true} {
			t.Run(platform+"/passthrough="+map[bool]string{false: "false", true: "true"}[passthrough], func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				account := &Account{Platform: platform, Type: AccountTypeAPIKey, Credentials: map[string]any{
					"api_protocol": APIProtocolAdaptive, "api_base_urls": map[string]any{APIProtocolAnthropic: "https://relay.example/messages"},
				}}
				svc := &OpenAIGatewayService{cfg: &config.Config{}}
				var req *http.Request
				var err error
				if passthrough {
					req, err = svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, []byte(`{"model":"test"}`), "relay-secret")
				} else {
					req, err = svc.buildUpstreamRequest(context.Background(), c, account, []byte(`{"model":"test"}`), "relay-secret", false, "", false)
				}
				require.Error(t, err)
				require.Nil(t, req, "a partial relay must never send its key to api.openai.com")
			})
		}
	}
}

func TestOpenAIUsageNoResolverRetainsPricingAt(t *testing.T) {
	svc := &OpenAIGatewayService{billingService: NewBillingService(nil, nil)}
	at := time.Date(2026, 10, 12, 2, 0, 0, 0, time.UTC)
	calc := func(at time.Time) float64 {
		cost, err := svc.calculateOpenAIRecordUsageTokenCost(context.Background(), &APIKey{}, "deepseek-v4-pro", 1, at,
			UsageTokens{InputTokens: 1000}, "", "", nil, false)
		require.NoError(t, err)
		return cost.ActualCost
	}
	require.InDelta(t, 2*calc(at.Add(3*time.Hour)), calc(at), 1e-12)
}

func TestOpenAIFinalAttemptInsufficientHoldStopsUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := newPreauthorizationFixture()
	fixture.service.costCalculator = NewBillingService(nil, nil)
	request := balancePreauthorizationTestRequest()
	request.CostInput = CostInput{Model: "gpt-6.1-sol", RateMultiplier: 1}
	guard, err := fixture.service.Preauthorize(context.Background(), request)
	require.NoError(t, err)
	defer func() { require.NoError(t, guard.Refund(context.Background())) }()
	fixture.wallet.topUp = []LiveBalanceResult{{Outcome: LiveBalanceOutcomeInsufficient}}
	ctx := ContextWithBalancePreauthorizationGuard(context.Background(), guard)
	ctx = ContextWithOpenAIAccountPreauthorization(ctx, OpenAIAccountPreauthorizationInput{
		APIKey: &APIKey{ID: 7, UserID: 42}, RequestedModel: "gpt-6.1-sol", PricingAt: time.Unix(123, 0),
	})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK,
		Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{}`))}}
	svc := &OpenAIGatewayService{cfg: &config.Config{Default: config.DefaultConfig{RateMultiplier: 1}}, httpUpstream: upstream}
	account := &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}
	_, err = svc.Forward(ctx, c, account, []byte(`{"model":"gpt-6.1-sol","input":"`+strings.Repeat("hello ", 10000)+`"}`))
	require.ErrorIs(t, err, ErrBalanceWithholdingFailed)
	require.True(t, IsOpenAIAccountPreauthorizationError(err))
	require.Empty(t, upstream.requests)
	require.Equal(t, 1, fixture.wallet.topUpCalls)
}

func TestOpenAIFinalAttemptRetryRepricesBeforeSecondNetworkCall(t *testing.T) {
	fixture := newPreauthorizationFixture()
	fixture.calculator.outputUnitPrice = 0.001
	request := balancePreauthorizationTestRequest()
	request.InitialOutputWindowTokens = 1
	guard, err := fixture.service.Preauthorize(context.Background(), request)
	require.NoError(t, err)
	defer func() { require.NoError(t, guard.Refund(context.Background())) }()
	fixture.wallet.topUp = []LiveBalanceResult{{Outcome: LiveBalanceOutcomeInsufficient}}
	ctx := ContextWithBalancePreauthorizationGuard(context.Background(), guard)
	ctx = ContextWithOpenAIAccountPreauthorization(ctx, OpenAIAccountPreauthorizationInput{
		APIKey: &APIKey{ID: 7, UserID: 42}, RequestedModel: "gpt-5.4", PricingAt: time.Unix(123, 0),
	})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusBadRequest,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(strings.NewReader(`{"error":{"code":"unsupported_parameter","message":"Unsupported parameter: max_output_tokens","param":"max_output_tokens"}}`))}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}
	_, err = svc.Forward(ctx, c, account, []byte(`{"model":"gpt-5.4","input":"hi","max_output_tokens":1}`))
	require.ErrorIs(t, err, ErrBalanceWithholdingFailed)
	require.True(t, IsOpenAIAccountPreauthorizationError(err))
	require.Len(t, upstream.requests, 1, "removing the rejected cap requires funding the larger output window before retry")
	require.Equal(t, 1, fixture.wallet.topUpCalls)
}

func TestOpenAIFinalAttemptPricesExplicitEffortWithoutChangingScale(t *testing.T) {
	for _, body := range []string{
		`{"reasoning_effort":"max"}`, `{"reasoning":{"effort":"max"}}`, `{"output_config":{"effort":"max"}}`,
	} {
		require.Equal(t, "max", optionalStringValue(finalOpenAIAttemptReasoningEffort([]byte(body), "claude-fable-5-1")))
	}
}

func TestOpenAIFinalAttemptProviderNativeMaxMustFundBeforeNetwork(t *testing.T) {
	for _, platform := range []string{PlatformCommandCode} {
		t.Run(platform, func(t *testing.T) {
			fixture := newPreauthorizationFixture()
			bs := NewBillingService(nil, nil)
			fixture.service.costCalculator = bs
			group := &Group{ID: 9, Platform: platform, RateMultiplier: 1, ModelPricing: []ChannelModelPricing{{
				Models: []string{"claude-fable-5-1"}, MaxReasoningEffortMultiplier: float64Ptr(3),
			}}}
			key := &APIKey{ID: 7, UserID: 42, GroupID: &group.ID, Group: group}
			body := []byte(`{"model":"claude-fable-5-1","max_tokens":32,"messages":[{"role":"user","content":"hi"}],"output_config":{"effort":"max"}}`)
			svc := &OpenAIGatewayService{cfg: &config.Config{}, billingService: bs, resolver: NewModelPricingResolver(nil, bs)}
			request := balancePreauthorizationTestRequest()
			estimate := EstimateBalancePreauthorizationTokens(body)
			request.EstimatedInputTokens, request.InitialOutputWindowTokens = estimate.InputTokens, estimate.OutputTokens
			request.CostInput = svc.BalancePreauthorizationCostInput(context.Background(), key, "claude-fable-5-1", time.Unix(123, 0), "")
			guard, err := fixture.service.Preauthorize(context.Background(), request)
			require.NoError(t, err)
			defer func() { require.NoError(t, guard.Refund(context.Background())) }()
			fixture.wallet.topUp = []LiveBalanceResult{{Outcome: LiveBalanceOutcomeInsufficient}}
			ctx := ContextWithBalancePreauthorizationGuard(context.Background(), guard)
			ctx = ContextWithOpenAIAccountPreauthorization(ctx, OpenAIAccountPreauthorizationInput{
				APIKey: key, RequestedModel: "claude-fable-5-1", PricingAt: time.Unix(123, 0),
			})
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
			upstream := &httpUpstreamRecorder{}
			svc.httpUpstream = upstream
			account := &Account{ID: 3, Platform: platform, Type: AccountTypeAPIKey, Credentials: map[string]any{
				"api_key": "relay-secret", "base_url": "https://relay.example", "api_protocol": APIProtocolAnthropic,
				"api_base_urls": map[string]any{APIProtocolAnthropic: "https://relay.example"},
			}}
			_, err = svc.forwardAnthropicViaNativeAnthropicEndpoint(ctx, c, account, body, "")
			require.ErrorIs(t, err, ErrBalanceWithholdingFailed)
			require.True(t, IsOpenAIAccountPreauthorizationError(err))
			require.Empty(t, upstream.requests)
			require.Equal(t, 1, fixture.wallet.topUpCalls)
		})
	}
}

func TestOpenAIFinalAttemptProviderChatMustFundBeforeNetwork(t *testing.T) {
	for _, platform := range []string{PlatformCommandCode, PlatformCline} {
		t.Run(platform, func(t *testing.T) {
			fixture := newPreauthorizationFixture()
			fixture.service.costCalculator = NewBillingService(nil, nil)
			request := balancePreauthorizationTestRequest()
			request.CostInput = CostInput{Model: "gpt-5.4", RateMultiplier: 1}
			guard, err := fixture.service.Preauthorize(context.Background(), request)
			require.NoError(t, err)
			defer func() { require.NoError(t, guard.Refund(context.Background())) }()
			fixture.wallet.topUp = []LiveBalanceResult{{Outcome: LiveBalanceOutcomeInsufficient}}
			ctx := ContextWithBalancePreauthorizationGuard(context.Background(), guard)
			ctx = ContextWithOpenAIAccountPreauthorization(ctx, OpenAIAccountPreauthorizationInput{
				APIKey: &APIKey{ID: 7, UserID: 42}, RequestedModel: "gpt-5.4", PricingAt: time.Unix(123, 0),
			})
			body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"` + strings.Repeat("hello ", 10000) + `"}]}`)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
			upstream := &httpUpstreamRecorder{}
			svc := &OpenAIGatewayService{cfg: &config.Config{Default: config.DefaultConfig{RateMultiplier: 1}}, httpUpstream: upstream}
			account := &Account{ID: 3, Platform: platform, Type: AccountTypeAPIKey, Credentials: map[string]any{
				"api_key": "relay-secret", "base_url": "https://relay.example", "api_protocol": APIProtocolChatCompletions,
				"api_base_urls": map[string]any{APIProtocolChatCompletions: "https://relay.example"},
			}}
			_, err = svc.ForwardAsChatCompletions(ctx, c, account, body, "", "")
			require.ErrorIs(t, err, ErrBalanceWithholdingFailed)
			require.True(t, IsOpenAIAccountPreauthorizationError(err))
			require.Empty(t, upstream.requests)
			require.Equal(t, 1, fixture.wallet.topUpCalls)
		})
	}
}

func TestOpenAIFinalAttemptMissingMetadataFailsClosed(t *testing.T) {
	fixture := newPreauthorizationFixture()
	guard, err := fixture.service.Preauthorize(context.Background(), balancePreauthorizationTestRequest())
	require.NoError(t, err)
	defer func() { require.NoError(t, guard.Refund(context.Background())) }()
	ctx := ContextWithBalancePreauthorizationGuard(context.Background(), guard)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	upstream := &httpUpstreamRecorder{}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}
	_, err = svc.Forward(ctx, c, account, []byte(`{"model":"gpt-5.4","input":"hi"}`))
	require.True(t, IsOpenAIAccountPreauthorizationError(err))
	require.ErrorIs(t, err, ErrInvalidBillingPreauthorizationEstimate)
	require.Empty(t, upstream.requests)
}

func TestOpenAIFinalAttemptFreeFastPricesStandardHold(t *testing.T) {
	fixture := newPreauthorizationFixture()
	guard, err := fixture.service.Preauthorize(context.Background(), balancePreauthorizationTestRequest())
	require.NoError(t, err)
	defer func() { require.NoError(t, guard.Refund(context.Background())) }()
	initialPrices := len(fixture.calculator.inputs)
	ctx := ContextWithBalancePreauthorizationGuard(context.Background(), guard)
	key := &APIKey{ID: 7, UserID: 42, GroupID: i64p(9), Group: &Group{ID: 9, Platform: PlatformOpenAI, FreeOpenAIFast: true, RateMultiplier: 1}}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	err = (&OpenAIGatewayService{}).RepriceBalancePreauthorizationForAccount(ctx, OpenAIAccountPreauthorizationInput{
		APIKey: key, Account: account, Body: []byte(`{"model":"gpt-6.1-sol","service_tier":"priority"}`),
		BillingModel: "gpt-6.1-sol", UpstreamModel: "gpt-6.1-sol", PricingAt: time.Unix(123, 0),
	})
	require.NoError(t, err)
	require.NotEmpty(t, fixture.calculator.inputs)
	for _, input := range fixture.calculator.inputs[initialPrices:] {
		require.Empty(t, input.ServiceTier)
	}
}

func TestOpenAIFinalAttemptNativeCompactFallbackRepricesSentModel(t *testing.T) {
	fixture := newPreauthorizationFixture()
	guard, err := fixture.service.Preauthorize(context.Background(), balancePreauthorizationTestRequest())
	require.NoError(t, err)
	defer func() { require.NoError(t, guard.Refund(context.Background())) }()
	initialPrices := len(fixture.calculator.inputs)
	ctx := ContextWithBalancePreauthorizationGuard(context.Background(), guard)
	ctx = ContextWithOpenAIAccountPreauthorization(ctx, OpenAIAccountPreauthorizationInput{
		APIKey: &APIKey{ID: 7, UserID: 42}, RequestedModel: "gpt-6.1-sol", PricingAt: time.Unix(123, 0),
	})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	MarkOpenAINativeCompactionV2(c)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{"error":{"code":"context_length_exceeded","message":"context window exceeded"}}`))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{"id":"resp_compact","status":"completed","model":"gpt-5.4","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`))},
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.4"}}, httpUpstream: upstream}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account"}}
	result, err := svc.Forward(ctx, c, account, []byte(`{"model":"gpt-6.1-sol","input":[{"type":"message","role":"user","content":"hi"},{"type":"compaction_trigger"}]}`))
	require.NoError(t, err)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "gpt-6.1-sol", gjson.GetBytes(upstream.bodies[0], "model").String())
	require.Equal(t, "gpt-5.4", gjson.GetBytes(upstream.bodies[1], "model").String())
	require.Equal(t, "gpt-5.4", result.UpstreamModel)
	var sawExempt, sawFull bool
	for _, input := range fixture.calculator.inputs[initialPrices:] {
		if input.DisableModelLongContextPricing {
			require.False(t, sawFull)
			sawExempt = true
		} else {
			sawFull = true
		}
	}
	require.True(t, sawExempt)
	require.True(t, sawFull)
}
