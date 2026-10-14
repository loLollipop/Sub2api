package service

import "context"

// Selection must admit the model the executor will send, including compact
// and passthrough semantics. A capability placeholder is checked at execution.
func openAIAccountAllowsModelScope(ctx context.Context, account *Account, requestedModel string, requireCompact bool) bool {
	if ModelScopeFromContext(ctx) == ModelScopeAll {
		return true
	}
	if !ValidModelScope(ModelScopeFromContext(ctx)) {
		return false
	}
	if requestedModel == "" {
		return true
	}
	return CheckModelScope(ctx, requestedModel, resolveOpenAIAccountUpstreamModelForRequest(account, requestedModel, requireCompact)) == nil
}

// The native gateway uses ordinary protocol mappings. In particular, an
// Anthropic API-key account applies model_mapping, never compact_model_mapping.
func gatewayAccountAllowsModelScope(ctx context.Context, account *Account, requestedModel string) bool {
	if ModelScopeFromContext(ctx) == ModelScopeAll {
		return true
	}
	if account == nil || !ValidModelScope(ModelScopeFromContext(ctx)) {
		return false
	}
	if requestedModel == "" {
		return true
	}
	upstreamModel := requestedModel
	switch account.Platform {
	case PlatformAnthropic:
		if account.IsAPIKeyOrBedrock() || account.Type == AccountTypeServiceAccount {
			upstreamModel = account.GetMappedModel(requestedModel)
		}
	case PlatformAntigravity:
		upstreamModel = mapAntigravityModel(account, requestedModel)
	case PlatformOpenAI:
		upstreamModel = resolveOpenAIAccountUpstreamModelForRequest(account, requestedModel, false)
	default:
		upstreamModel = account.GetMappedModel(requestedModel)
	}
	return CheckModelScope(ctx, requestedModel, upstreamModel) == nil
}
