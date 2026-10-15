package service

import (
	"context"
	"strings"
)

// GeminiCatalogAliases returns finite public aliases, not inferred wildcard
// model names. A caller must still prove that the resolved target exists in an
// account's actual native catalog before exposing the alias.
func (s *GatewayService) GeminiCatalogAliases(ctx context.Context, group *Group) []string {
	if s == nil || group == nil {
		return nil
	}
	var aliases []string
	if s.channelService != nil {
		channel, err := s.channelService.GetChannelForGroup(ctx, group.ID)
		if err == nil && channel != nil {
			for _, platform := range matchingPlatforms(group.Platform) {
				for public := range channel.ModelMapping[platform] {
					if IsSafeGeminiModelPathSegment(public) {
						aliases = append(aliases, public)
					}
				}
			}
		}
	}
	if group.Platform == PlatformComposite && s.compositeResolver != nil {
		targets, err := s.compositeResolver.exactPublicModelTargets(ctx, group.ID, CompositeRouteEndpointGemini)
		if err == nil {
			for public := range targets {
				if IsSafeGeminiModelPathSegment(public) {
					aliases = append(aliases, public)
				}
			}
		}
	}
	// A concrete public allowlist entry may instantiate a wildcard mapping.
	if group.ModelAllowlistEnabled() {
		for _, public := range group.ModelAllowlist.Models {
			if IsSafeGeminiModelPathSegment(public) {
				aliases = append(aliases, public)
			}
		}
	}
	return dedupeAndSortModelIDs(aliases)
}

// GeminiCatalogRoute follows the existing generateContent routing order:
// public allowlist (at the caller), composite target, channel, then account
// mapping (in the native service). It never applies a public allowlist to an
// internal target and never redirects to a non-native provider.
// The context carries the resolved platform into channel lookup and account
// selection; an Antigravity alias must not borrow a Gemini account's catalog.
func (s *GatewayService) GeminiCatalogRoute(ctx context.Context, group *Group, public string) (context.Context, string, bool) {
	model := strings.TrimPrefix(strings.TrimSpace(public), "models/")
	if !IsSafeGeminiModelPathSegment(model) || group == nil {
		return ctx, "", false
	}
	if group.Platform == PlatformComposite {
		if s == nil {
			return ctx, "", false
		}
		// A model-list request has no model for middleware to resolve and may
		// carry its default Gemini platform. Resolve each catalog entry afresh
		// for its own group, rather than treating that default as a decision.
		decision, err := s.compositeResolver.Resolve(ctx, group.ID, model, CompositeRouteEndpointGemini)
		if err != nil || !decision.Matched || (decision.TargetPlatform != PlatformGemini && decision.TargetPlatform != PlatformAntigravity) {
			return ctx, "", false
		}
		model = decision.UpstreamModel
		ctx = WithCompositeRouteDecision(ctx, decision)
	}
	if s != nil {
		mapped, _ := s.ResolveChannelMappingAndRestrict(ctx, &group.ID, model)
		model = mapped.MappedModel
	}
	return ctx, model, IsSafeGeminiModelPathSegment(model)
}
