package service

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// GeminiCacheCreateQuoteInput uses the actual resolved upstream model and a
// frozen interval. Only native group/channel pricing can override token rates.
// There are deliberately no output, effort, service-tier or arbitrary price inputs.
type GeminiCacheCreateQuoteInput struct {
	GeminiCacheStorageQuoteInput
	GroupID  *int64
	Group    *Group
	Resolver *ModelPricingResolver
}

// GeminiCacheCreateQuote retains unrounded component costs for reconciliation.
// InputOnlyChargeAmount is the input charge when orphan cleanup is confirmed;
// it is not a refund amount or permission to release a storage reservation.
type GeminiCacheCreateQuote struct {
	GeminiCacheStorageQuoteInput
	InputQuote            CostBreakdown
	StorageQuote          GeminiCacheStorageQuote
	ChargeAmount          float64
	ReservationAmount     float64
	InputOnlyChargeAmount float64
}

// QuoteGeminiCacheCreate only calculates a quote. It does not authorize a hold,
// create/delete a cache, or mutate wallets, quotas, subscriptions or usage.
func (s *BillingService) QuoteGeminiCacheCreate(ctx context.Context, input GeminiCacheCreateQuoteInput) (*GeminiCacheCreateQuote, error) {
	if s == nil {
		return nil, fmt.Errorf("cache create quote requires a billing service")
	}
	if input.TotalTokenCount < 0 || uint64(input.TotalTokenCount) > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("cache create token count is outside int range")
	}
	storage, err := s.QuoteGeminiCacheStorage(input.GeminiCacheStorageQuoteInput)
	if err != nil {
		return nil, err
	}
	resolver := input.Resolver
	if resolver == nil {
		resolver = NewModelPricingResolver(nil, s)
	}
	if resolver.billingService == nil {
		return nil, fmt.Errorf("cache create quote requires a billing pricing resolver")
	}
	resolved := resolver.Resolve(ctx, PricingInput{Model: input.Model, GroupID: input.GroupID, Group: input.Group})
	if resolved.Mode != BillingModeToken {
		return nil, fmt.Errorf("cache create requires token pricing: %w", ErrModelPricingUnavailable)
	}
	pricingContext := int(input.TotalTokenCount)
	if !resolved.longContextPricingEnabled {
		pricingContext = 1
	}
	// Presence travels with this resolved snapshot, including catalog, configured
	// and interval rates. Never infer a free input rate from an output-only card
	// or re-read a potentially newer catalog to validate an older price snapshot.
	pricing := resolver.GetIntervalPricing(resolved, pricingContext)
	if pricing == nil || pricing.InputPriceAbsent {
		return nil, fmt.Errorf("cache create input pricing: %w", ErrModelPricingUnavailable)
	}
	if !validCacheStorageAmount(pricing.InputPricePerToken) {
		return nil, fmt.Errorf("invalid cache create input price")
	}
	inputCost, err := s.CalculateCostUnified(CostInput{
		Ctx: ctx, Model: input.Model, GroupID: input.GroupID, Group: input.Group,
		Tokens:         UsageTokens{InputTokens: int(input.TotalTokenCount)},
		RateMultiplier: input.RateMultiplier, PricingAt: input.PricingAt,
		Resolver: resolver, Resolved: resolved,
	})
	if err != nil {
		return nil, err
	}
	if inputCost == nil || !validCacheStorageAmount(inputCost.InputCost) ||
		!validCacheStorageAmount(inputCost.TotalCost) || !validCacheStorageAmount(inputCost.ActualCost) {
		return nil, fmt.Errorf("invalid cache create input amount or overflow")
	}
	// Convert the raw native component results before addition. Rounding each
	// component independently would lose charges below one monetary unit.
	inputAmount := decimal.NewFromFloat(inputCost.ActualCost)
	combined := inputAmount.Add(decimal.NewFromFloat(storage.ActualCost))
	charge, err := quantizeGeminiCacheCreateAmount(combined, false)
	if err != nil {
		return nil, err
	}
	reservation, err := quantizeGeminiCacheCreateAmount(combined, true)
	if err != nil {
		return nil, err
	}
	inputOnly, err := quantizeGeminiCacheCreateAmount(inputAmount, false)
	if err != nil {
		return nil, err
	}
	return &GeminiCacheCreateQuote{
		GeminiCacheStorageQuoteInput: input.GeminiCacheStorageQuoteInput,
		InputQuote:                   *inputCost, StorageQuote: *storage,
		ChargeAmount: charge, ReservationAmount: reservation, InputOnlyChargeAmount: inputOnly,
	}, nil
}

func quantizeGeminiCacheCreateAmount(amount decimal.Decimal, roundUp bool) (float64, error) {
	quantized := amount.Round(UsageBillingMonetaryScale)
	if roundUp {
		quantized = amount.RoundCeil(UsageBillingMonetaryScale)
	}
	limit := decimal.New(1, 12) // NUMERIC(20,8) has twelve integer digits.
	if amount.IsNegative() || quantized.GreaterThanOrEqual(limit) {
		return 0, fmt.Errorf("cache create amount overflows NUMERIC(20,8)")
	}
	value, _ := quantized.Float64()
	// A valid decimal just below the limit can round to 1e12 as float64.
	if !validCacheStorageAmount(value) || value >= 1e12 {
		return 0, fmt.Errorf("cache create amount float conversion overflows NUMERIC(20,8)")
	}
	// Downstream monetary APIs use float64's shortest decimal representation.
	// Reject lossy conversions rather than reduce a ceil8 hold (or change a
	// finalized charge) while still appearing to fit the database column.
	if !decimal.NewFromFloat(value).Equal(quantized) {
		return 0, fmt.Errorf("cache create amount loses monetary precision in float conversion")
	}
	return value, nil
}
