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
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIAccountPreauthorizationUsesCredentialAndSentModel(t *testing.T) {
	for _, tc := range []struct {
		name, accountType, wireModel, pricingSource, wantPriceModel string
		shadow, wantExempt                                          bool
	}{
		{name: "oauth_alias", accountType: AccountTypeOAuth, wireModel: "gpt-6.1-sol", wantExempt: true, wantPriceModel: "gpt-6.1-sol"},
		{name: "setup_token", accountType: AccountTypeSetupToken, wireModel: "gpt-6.1-sol", wantExempt: true, wantPriceModel: "gpt-6.1-sol"},
		{name: "api_key", accountType: AccountTypeAPIKey, wireModel: "gpt-6.1-sol", wantPriceModel: "gpt-6.1-sol"},
		{name: "mapped_away", accountType: AccountTypeOAuth, wireModel: "gpt-6-sol", wantPriceModel: "gpt-6-sol"},
		{name: "requested_price_is_not_wire_model", accountType: AccountTypeOAuth, wireModel: "gpt-6.1-sol", pricingSource: BillingModelSourceRequested, wantExempt: true, wantPriceModel: "public-alias"},
		{name: "channel_price_is_not_wire_model", accountType: AccountTypeOAuth, wireModel: "gpt-6.1-sol", pricingSource: BillingModelSourceChannelMapped, wantExempt: true, wantPriceModel: "channel-alias"},
		{name: "shadow_uses_parent", accountType: AccountTypeOAuth, wireModel: "gpt-6.1-sol", shadow: true, wantExempt: true, wantPriceModel: "gpt-6.1-sol"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, protocol := range []string{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
				fixture := newPreauthorizationFixture()
				guard, err := fixture.service.Preauthorize(context.Background(), balancePreauthorizationTestRequest())
				require.NoError(t, err)
				ctx := ContextWithBalancePreauthorizationGuard(context.Background(), guard)
				svc := &OpenAIGatewayService{}
				key := &APIKey{ID: 7, UserID: 42, GroupID: i64p(9), Group: &Group{ID: 9, Platform: PlatformOpenAI,
					RateMultiplier: 0.15}}
				account := &Account{ID: 3, Platform: PlatformOpenAI, Type: tc.accountType,
					Credentials: map[string]any{"model_mapping": map[string]any{"channel-alias": tc.wireModel}},
					Extra:       map[string]any{openAILongContextBillingEnabledKey: true}}
				if tc.shadow {
					account.ParentAccountID = i64p(4)
					account.QuotaDimension = QuotaDimensionSpark
					account.Extra = map[string]any{openAILongContextBillingEnabledKey: false}
					svc.accountRepo = &openAIRecordUsageAccountRepoStub{account: &Account{ID: 4,
						Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{openAILongContextBillingEnabledKey: true}}}
				}
				at := time.Unix(123, 0)
				err = svc.RepriceBalancePreauthorizationForAccount(ctx, OpenAIAccountPreauthorizationInput{
					APIKey: key, Account: account, Body: []byte(`{"model":"channel-alias","messages":[{"role":"user","content":"hi"}]}`),
					RequestedModel: "public-alias", ChannelMapping: ChannelMappingResult{Mapped: true, MappedModel: "channel-alias", BillingModelSource: tc.pricingSource},
					BillingModel: tc.wireModel, UpstreamModel: tc.wireModel, PricingAt: at,
				})
				require.NoError(t, err)
				last := fixture.calculator.inputs[len(fixture.calculator.inputs)-1]
				require.Equal(t, tc.wantExempt, last.DisableModelLongContextPricing)
				require.NotNil(t, last.LongContextBillingEnabled)
				require.True(t, *last.LongContextBillingEnabled)
				require.Equal(t, tc.wantPriceModel, last.Model)
				require.Equal(t, at, last.PricingAt)
				if tc.wantPriceModel == "gpt-6.1-sol" || tc.wantPriceModel == "gpt-6-sol" {
					require.Equal(t, 0.15, last.RateMultiplier)
				}
				require.NoError(t, guard.Refund(context.Background()))
				_ = protocol // All ingress shapes share the same final-attempt contract.
			}
		})
	}
}

func TestOpenAIAccountPreauthorizationFinalForcedFastBeforeNetwork(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := newPreauthorizationFixture()
	guard, err := fixture.service.Preauthorize(context.Background(), balancePreauthorizationTestRequest())
	require.NoError(t, err)
	defer func() { require.NoError(t, guard.Refund(context.Background())) }()
	group := &Group{ID: 9, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, ForceOpenAIFast: true, RateMultiplier: 1}
	key := &APIKey{ID: 7, UserID: 42, GroupID: &group.ID, Group: group}
	ctx := context.WithValue(ContextWithBalancePreauthorizationGuard(context.Background(), guard), ctxkey.Group, group)
	body := []byte(`{"model":"gpt-6.1-sol","instructions":"test","input":"hi","stream":false}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(strings.NewReader(`{"id":"resp_test","status":"completed","model":"gpt-6.1-sol","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`))}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account"}}
	ctx = ContextWithOpenAIAccountPreauthorization(ctx, OpenAIAccountPreauthorizationInput{
		APIKey: key, RequestedModel: "gpt-6.1-sol", PricingAt: time.Unix(123, 0),
	})
	result, err := svc.Forward(ctx, c, account, body)
	require.NoError(t, err)
	require.Len(t, upstream.bodies, 1)
	require.Equal(t, "priority", gjson.GetBytes(upstream.bodies[0], "service_tier").String())
	require.Equal(t, "priority", *result.ServiceTier)
	last := fixture.calculator.inputs[len(fixture.calculator.inputs)-1]
	require.Equal(t, "priority", last.ServiceTier, "the hold must price the forced tier before the actual upstream request")
}

func TestOpenAIAccountPreauthorizationNoGuardDoesNotResolveCredentials(t *testing.T) {
	svc := &OpenAIGatewayService{}
	require.NoError(t, svc.RepriceBalancePreauthorizationForAccount(context.Background(), OpenAIAccountPreauthorizationInput{
		Account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: i64p(999), QuotaDimension: QuotaDimensionSpark},
	}))
}

func TestOpenAIAccountPreauthorizationFailsClosedAfterOutput(t *testing.T) {
	fixture := newPreauthorizationFixture()
	guard, err := fixture.service.Preauthorize(context.Background(), balancePreauthorizationTestRequest())
	require.NoError(t, err)
	ctx := ContextWithBalancePreauthorizationGuard(context.Background(), guard)
	require.NoError(t, guard.ObserveStreamingOutput(ctx, 1))
	err = (&OpenAIGatewayService{}).RepriceBalancePreauthorizationForAccount(ctx, OpenAIAccountPreauthorizationInput{
		APIKey: &APIKey{ID: 7, UserID: 42}, Account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		Body: []byte(`{"model":"gpt-6.1-sol"}`), BillingModel: "gpt-6.1-sol", UpstreamModel: "gpt-6.1-sol",
	})
	require.ErrorIs(t, err, ErrBalancePreauthorizationOutputObserved)
}

func TestCalculateTokenCostForRequestPreservesSubscriptionPolicy(t *testing.T) {
	bs := NewBillingService(nil, nil)
	for _, withResolver := range []bool{false, true} {
		req := TokenCostRequest{Model: "gpt-6.1-sol", Tokens: UsageTokens{InputTokens: 300000}, RateMultiplier: 0.15,
			LongContextBillingEnabled: boolPtr(true), DisableModelLongContextPricing: true}
		if withResolver {
			req.Group = &Group{ID: 1, LongContextPricingEnabled: true}
			req.Resolver = NewModelPricingResolver(nil, bs)
		}
		cost, err := bs.CalculateTokenCostForRequest(req)
		require.NoError(t, err)
		require.InDelta(t, 0.6*0.15, cost.ActualCost, 1e-12)
		require.False(t, cost.LongContextBillingApplied)
	}
}

func TestOpenAIAccountPreauthorizationRebuildsRealPriceHoldAcrossCredentials(t *testing.T) {
	for _, startExempt := range []bool{false, true} {
		for _, insufficient := range []bool{false, true} {
			if !startExempt && insufficient {
				continue // A cheaper route retains the already-funded hold.
			}
			t.Run(fmt.Sprintf("startExempt=%v/insufficient=%v", startExempt, insufficient), func(t *testing.T) {
				fixture := newPreauthorizationFixture()
				bs := NewBillingService(nil, nil)
				fixture.service.costCalculator = bs
				group := &Group{ID: 9, Platform: PlatformOpenAI, LongContextPricingEnabled: true,
					RateMultiplier: 0.15}
				key := &APIKey{ID: 7, UserID: 42, GroupID: &group.ID, Group: group}
				svc := &OpenAIGatewayService{billingService: bs, resolver: NewModelPricingResolver(nil, bs)}
				body := []byte(`{"model":"gpt-6.1-sol","service_tier":"priority","max_output_tokens":16,"input":"` + strings.Repeat(" hello", 300001) + `"}`)
				tokens := EstimateBalancePreauthorizationTokens(body)
				require.Greater(t, tokens.InputTokens, 272000)
				request := balancePreauthorizationTestRequest()
				request.BillableInputBytes, request.EstimatedInputTokens, request.InitialOutputWindowTokens = len(body), tokens.InputTokens, tokens.OutputTokens
				request.CostInput = svc.BalancePreauthorizationCostInput(context.Background(), key, "gpt-6.1-sol", time.Unix(123, 0), "priority")
				request.CostInput.LongContextBillingEnabled = boolPtr(true)
				request.CostInput.DisableModelLongContextPricing = startExempt
				guard, err := fixture.service.Preauthorize(context.Background(), request)
				require.NoError(t, err)
				ctx := ContextWithBalancePreauthorizationGuard(context.Background(), guard)
				initialHold := guard.HoldAmount()
				initialAttempt := fixture.wallet.lastAttemptID
				accountType := AccountTypeOAuth
				if startExempt {
					accountType = AccountTypeAPIKey
				}
				if insufficient {
					fixture.wallet.topUp = []LiveBalanceResult{{Outcome: LiveBalanceOutcomeInsufficient}}
				}
				err = svc.RepriceBalancePreauthorizationForAccount(ctx, OpenAIAccountPreauthorizationInput{
					APIKey: key, Body: body, PricingAt: request.CostInput.PricingAt, RequestedModel: "gpt-6.1-sol", BillingModel: "gpt-6.1-sol", UpstreamModel: "gpt-6.1-sol",
					Account: &Account{Platform: PlatformOpenAI, Type: accountType, Extra: map[string]any{openAILongContextBillingEnabledKey: true}},
				})
				if insufficient {
					require.ErrorIs(t, err, ErrBalanceWithholdingFailed)
					require.Equal(t, initialHold, guard.HoldAmount())
				} else {
					require.NoError(t, err)
					if startExempt {
						require.Greater(t, guard.HoldAmount(), initialHold)
						require.Equal(t, 1, fixture.wallet.topUpCalls)
					} else {
						require.Equal(t, initialHold, guard.HoldAmount())
						require.Zero(t, fixture.wallet.topUpCalls)
						tracker := guard.core.outputHoldTracker
						require.NotNil(t, tracker)
						require.Less(t, tracker.TargetHoldAmount(), initialHold)
						// Consuming the cheaper output price uses retained surplus
						// before extending the wallet's hold.
						require.NoError(t, guard.ObserveStreamingOutput(ctx, tokens.OutputTokens+1))
						require.Zero(t, fixture.wallet.topUpCalls)
					}
				}
				require.Equal(t, initialAttempt, fixture.wallet.lastAttemptID)
				require.NoError(t, guard.Refund(context.Background()))
			})
		}
	}
}
