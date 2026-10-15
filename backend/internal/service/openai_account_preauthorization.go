package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// OpenAIAccountPreauthorizationInput separates the public billing identity from
// the actual outbound model. Account selection, mappings and credentials must
// be known before applying subscription-only pricing to an existing hold.
type OpenAIAccountPreauthorizationInput struct {
	APIKey         *APIKey
	Subscription   *UserSubscription
	Account        *Account
	Body           []byte
	RequestedModel string
	ChannelMapping ChannelMappingResult
	BillingModel   string
	UpstreamModel  string
	PricingAt      time.Time
}

type openAIAccountPreauthorizationContextKey struct{}

// ContextWithOpenAIAccountPreauthorization freezes the billing identity for an
// account attempt. The service adds its actual final body/model only after
// conversion and policy rewriting; no speculative copy of those transforms is
// used to reserve money. Do not retain a request body in this metadata.
func ContextWithOpenAIAccountPreauthorization(ctx context.Context, in OpenAIAccountPreauthorizationInput) context.Context {
	in.Body = nil
	in.Account = nil
	in.BillingModel = ""
	in.UpstreamModel = ""
	return context.WithValue(nonNilContext(ctx), openAIAccountPreauthorizationContextKey{}, in)
}

type openAIAccountPreauthorizationError struct{ error }

func (e *openAIAccountPreauthorizationError) Unwrap() error { return e.error }

// IsOpenAIAccountPreauthorizationError distinguishes a local money rejection
// from an upstream failure, so handlers retain their billing HTTP/SSE response.
func IsOpenAIAccountPreauthorizationError(err error) bool {
	var target *openAIAccountPreauthorizationError
	return errors.As(err, &target)
}

func (s *OpenAIGatewayService) repriceOpenAIFinalAttempt(ctx context.Context, account *Account, body []byte, billingModel, upstreamModel string) error {
	if _, ok := BalancePreauthorizationGuardFromContext(ctx); !ok {
		return nil
	}
	if account == nil {
		return &openAIAccountPreauthorizationError{balancePreauthorizationUnavailable(ErrInvalidBillingPreauthorizationEstimate)}
	}
	in, ok := ctx.Value(openAIAccountPreauthorizationContextKey{}).(OpenAIAccountPreauthorizationInput)
	if !ok {
		return &openAIAccountPreauthorizationError{balancePreauthorizationUnavailable(ErrInvalidBillingPreauthorizationEstimate)}
	}
	in.Account, in.Body = account, body
	in.BillingModel, in.UpstreamModel = billingModel, upstreamModel
	if err := s.RepriceBalancePreauthorizationForAccount(ctx, in); err != nil {
		return &openAIAccountPreauthorizationError{err}
	}
	return nil
}

// Request builders can still rewrite quota fields, metadata and Responses Lite
// payloads. Price their replayable wire body, without consuming the request.
func (s *OpenAIGatewayService) repriceOpenAIFinalHTTPRequest(ctx context.Context, account *Account, req *http.Request, billingModel, upstreamModel string) error {
	if _, ok := BalancePreauthorizationGuardFromContext(ctx); !ok {
		return nil
	}
	if req == nil || req.GetBody == nil {
		return &openAIAccountPreauthorizationError{balancePreauthorizationUnavailable(ErrInvalidBillingPreauthorizationEstimate)}
	}
	reader, err := req.GetBody()
	if err != nil {
		return &openAIAccountPreauthorizationError{balancePreauthorizationUnavailable(err)}
	}
	defer reader.Close()
	body, err := io.ReadAll(reader)
	if err != nil {
		return &openAIAccountPreauthorizationError{balancePreauthorizationUnavailable(err)}
	}
	if wireModel := strings.TrimSpace(gjson.GetBytes(body, "model").String()); wireModel != "" {
		upstreamModel = wireModel
	}
	return s.repriceOpenAIFinalAttempt(ctx, account, body, billingModel, upstreamModel)
}

func finalOpenAIAttemptReasoningEffort(body []byte, models ...string) *string {
	// Final explicit wire values have already passed protocol policy. In
	// particular Claude max must not be normalized to OpenAI's xhigh scale.
	if raw := explicitRequestedReasoningEffortFromBody(body); raw != "" {
		value := strings.ToLower(strings.TrimSpace(raw))
		return &value
	}
	return ApplyThinkingEnabledFallback(extractOpenAIReasoningEffortFromBody(body, models...), body, firstNonEmpty(models...))
}

// RepriceBalancePreauthorizationForAccount prices an already-transformed
// outbound attempt. Initial admission is conservative while no account is known.
// Repricing retains any surplus hold; an OAuth -> API-key switch must top up
// before network I/O, while API-key -> OAuth does not release money mid-request.
func (s *OpenAIGatewayService) RepriceBalancePreauthorizationForAccount(ctx context.Context, in OpenAIAccountPreauthorizationInput) error {
	guard, ok := BalancePreauthorizationGuardFromContext(ctx)
	if !ok {
		return nil
	}
	if in.Account == nil || in.APIKey == nil || strings.TrimSpace(in.BillingModel) == "" || strings.TrimSpace(in.UpstreamModel) == "" {
		return balancePreauthorizationUnavailable(ErrInvalidBillingPreauthorizationEstimate)
	}
	credential, err := resolveCredentialAccount(ctx, s.accountRepo, in.Account)
	if err != nil {
		return balancePreauthorizationUnavailable(err)
	}
	billingModel := strings.TrimSpace(in.BillingModel)
	// Requested/channel price policies still win; subscription eligibility is
	// based separately on upstreamModel, never on this selected price label.
	if in.ChannelMapping.BillingModelSource == BillingModelSourceRequested ||
		in.ChannelMapping.BillingModelSource == BillingModelSourceChannelMapped {
		billingModel = BalancePreauthorizationBillingModel(in.RequestedModel, in.ChannelMapping)
	}
	tier := gjson.GetBytes(in.Body, "service_tier").String()
	if groupBillsOpenAIFastAtStandard(in.APIKey, in.Account, tier) {
		tier = ""
	}
	costInput := s.BalancePreauthorizationCostInput(ctx, in.APIKey, billingModel, in.PricingAt, tier)
	costInput.LongContextBillingEnabled = openAILongContextBillingGate(credential)
	costInput.DisableModelLongContextPricing = openAISubscriptionLongContextExempt(credential, &OpenAIForwardResult{UpstreamModel: in.UpstreamModel})
	costInput.ReasoningEffort = optionalStringValue(finalOpenAIAttemptReasoningEffort(in.Body, in.UpstreamModel, in.BillingModel))
	estimate := EstimateBalancePreauthorizationTokens(in.Body)
	return guard.RepriceForRoute(ctx, BalancePreauthorizationRequest{
		APIKeyID: in.APIKey.ID, UserID: balancePreauthorizationAPIKeyUserID(in.APIKey),
		BillingType:        BalancePreauthorizationBillingType(in.APIKey, in.Subscription),
		BillableInputBytes: len(in.Body), EstimatedInputTokens: estimate.InputTokens,
		InitialOutputWindowTokens: estimate.OutputTokens, CostInput: costInput,
	})
}
