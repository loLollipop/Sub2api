package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"
)

const modelsListCatalogMarker = "*"

func (s *GatewayService) GetCompositeRouteModels(ctx context.Context, groupID *int64, endpoint string) ([]string, error) {
	if s == nil || s.compositeResolver == nil || groupID == nil {
		return nil, nil
	}
	return s.compositeResolver.ListExactPublicModels(ctx, *groupID, endpoint)
}

type groupModelsCatalog struct {
	byPlatform map[string][]string
	platforms  map[string]struct{}
	// Private mapping snapshots preserve exact/longest-wildcard resolution.
	// They contain no account credentials and never synthesize model identities.
	scopeMappings []modelScopeMapping
}

type modelScopeMapping struct {
	platform    string
	mapping     map[string]string
	passthrough bool
}

func modelsListCatalogCacheKey(groupID *int64) string {
	return modelsListCacheKey(groupID, modelsListCatalogMarker)
}

func (c groupModelsCatalog) modelsFor(platform string) []string {
	if c.byPlatform == nil {
		return nil
	}
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return c.unionModels()
	}
	return c.byPlatform[platform]
}

func (c groupModelsCatalog) unionModels() []string {
	seen := make(map[string]struct{})
	var models []string
	for _, platform := range groupCatalogPlatforms() {
		for _, model := range c.byPlatform[platform] {
			model = strings.TrimSpace(model)
			if model == "" {
				continue
			}
			if _, ok := seen[model]; ok {
				continue
			}
			seen[model] = struct{}{}
			models = append(models, model)
		}
	}
	if len(models) == 0 {
		return nil
	}
	sort.Strings(models)
	return models
}

func storedModelList(models []string) []string {
	if models == nil {
		return []string{}
	}
	return cloneStringSlice(models)
}

func (s *GatewayService) ensureGroupModelsCatalog(ctx context.Context, groupID *int64) groupModelsCatalog {
	if s == nil {
		return groupModelsCatalog{}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	catalogKey := modelsListCatalogCacheKey(groupID)
	if s.modelsListCache != nil {
		if cached, found := s.modelsListCache.Get(catalogKey); found {
			if cat, ok := cached.(groupModelsCatalog); ok {
				return cat
			}
		}
	}

	resultCh := s.modelsListSF.DoChan(catalogKey, func() (any, error) {
		leaderCtx := context.Background()
		if ctx != nil {
			leaderCtx = context.WithoutCancel(ctx)
		}
		leaderCtx, leaderCancel := context.WithTimeout(leaderCtx, 5*time.Second)
		defer leaderCancel()
		if s.modelsListCache != nil {
			if cached, found := s.modelsListCache.Get(catalogKey); found {
				if cat, ok := cached.(groupModelsCatalog); ok {
					return cat, nil
				}
			}
		}
		cat := s.buildGroupModelsCatalog(leaderCtx, groupID)
		s.storeGroupModelsCatalog(groupID, cat)
		return cat, nil
	})

	var result singleflight.Result
	select {
	case result = <-resultCh:
	case <-ctx.Done():
		return groupModelsCatalog{}
	}
	if result.Err != nil {
		return groupModelsCatalog{}
	}
	cat, _ := result.Val.(groupModelsCatalog)
	return cat
}

func (s *GatewayService) storeGroupModelsCatalog(groupID *int64, cat groupModelsCatalog) {
	if s == nil {
		return
	}
	if s.modelsListCache != nil {
		for _, platform := range groupCatalogPlatforms() {
			s.modelsListCache.Set(modelsListCacheKey(groupID, platform), storedModelList(cat.modelsFor(platform)), s.modelsListCacheTTL)
		}
		s.modelsListCache.Set(modelsListCatalogCacheKey(groupID), cat, s.modelsListCacheTTL)
	}
	if s.platformsListCache != nil {
		s.platformsListCache.Set(fmt.Sprintf("%d", derefGroupID(groupID)), clonePlatformSet(cat.platforms), s.platformsListCacheTTL)
	}
}

func (s *GatewayService) buildGroupModelsCatalog(ctx context.Context, groupID *int64) groupModelsCatalog {
	if s == nil {
		return groupModelsCatalog{}
	}
	return loadGroupModelsCatalogFromStore(ctx, s.accountRepo, s.schedulerSnapshot, groupID)
}

func loadGroupModelsCatalogFromStore(ctx context.Context, repo AccountRepository, snapshot *SchedulerSnapshotService, groupID *int64) groupModelsCatalog {
	cat := groupModelsCatalog{
		byPlatform: make(map[string][]string, len(groupCatalogPlatforms())),
		platforms:  make(map[string]struct{}),
	}
	if repo == nil && snapshot == nil {
		return cat
	}
	if ctx == nil {
		ctx = context.Background()
	}

	addPlatforms := func(accounts []Account) {
		for i := range accounts {
			cat.scopeMappings = append(cat.scopeMappings, modelScopeMapping{
				platform:    accounts[i].Platform,
				mapping:     accounts[i].GetModelMapping(),
				passthrough: accounts[i].IsOpenAIPassthroughEnabled(),
			})
			if platform := strings.TrimSpace(accounts[i].Platform); platform != "" {
				cat.platforms[platform] = struct{}{}
			}
		}
	}

	if snapshot != nil {
		requestCtx := withSchedulerRequestMode(ctx, repo, snapshot)
		for _, platform := range groupCatalogPlatforms() {
			accounts, _, err := snapshot.listSchedulableAccountsForRequest(requestCtx, groupID, platform, false)
			if err != nil {
				// A cold/failed bucket leaves the catalog unknown. Recovery
				// belongs to the snapshot service, never a direct repo scan.
				continue
			}
			addPlatforms(accounts)
			cat.byPlatform[platform] = modelsFromSchedulableAccounts(accounts, platform)
		}
		return cat
	}

	accounts, err := listSchedulableAccountsFromRepo(ctx, repo, groupID)
	if err != nil || len(accounts) == 0 {
		return cat
	}
	addPlatforms(accounts)
	for _, platform := range groupCatalogPlatforms() {
		cat.byPlatform[platform] = modelsFromSchedulableAccounts(accounts, platform)
	}
	return cat
}

// FilterModelsForKeyScope applies the same public and target boundaries as
// forwarding. Opaque aliases that cannot establish the selected family remain
// unavailable for scoped keys. Legacy all keys preserve their original list.
func (s *GatewayService) FilterModelsForKeyScope(ctx context.Context, groupID *int64, scope string, models []string, compositeEndpoint ...string) []string {
	if NormalizeModelScope(scope) == ModelScopeAll {
		return cloneStringSlice(models)
	}
	cat := s.ensureGroupModelsCatalog(ctx, groupID)
	channelModel := func(model string) string {
		mapped, _ := s.ResolveChannelMappingAndRestrict(ctx, groupID, model)
		return mapped.MappedModel
	}
	if len(compositeEndpoint) == 0 || groupID == nil || s.compositeResolver == nil {
		return filterModelsForKeyScopeMapped(cat, scope, models, channelModel)
	}
	targets, err := s.compositeResolver.exactPublicModelTargets(ctx, *groupID, compositeEndpoint[0])
	if err != nil {
		// Preserve the account-only catalog when the route store is unavailable.
		return filterModelsForKeyScopeMapped(cat, scope, models, channelModel)
	}
	filtered := make([]string, 0, len(models))
	for _, model := range models {
		if !ModelScopeAllows(scope, model) {
			continue
		}
		candidates, routed := targets[model]
		if !routed {
			if len(filterModelsForKeyScopeMapped(cat, scope, []string{model}, channelModel)) > 0 {
				filtered = append(filtered, model)
			}
			continue
		}
		// A route alias is authorized against its winning target before the
		// channel/account mapping boundary, just like actual forwarding.
		for _, target := range candidates {
			targetCatalog := cat
			if target.platform != "" {
				targetCatalog.scopeMappings = nil
				for _, account := range cat.scopeMappings {
					if account.platform == target.platform {
						targetCatalog.scopeMappings = append(targetCatalog.scopeMappings, account)
					}
				}
				// No account of the routed platform cannot establish scope. Do
				// not borrow another platform's passthrough/mapping capability.
				if len(targetCatalog.scopeMappings) == 0 {
					continue
				}
			}
			if len(filterModelsForKeyScopeMapped(targetCatalog, scope, []string{target.model}, channelModel)) > 0 {
				filtered = append(filtered, model)
				break
			}
		}
	}
	return filtered
}

func (s *OpenAIGatewayService) FilterModelsForKeyScope(ctx context.Context, groupID *int64, scope string, models []string) []string {
	if NormalizeModelScope(scope) == ModelScopeAll {
		return cloneStringSlice(models)
	}
	return filterModelsForKeyScopeMapped(s.routeModelsCatalog(ctx, groupID), scope, models, func(model string) string {
		mapped, _ := s.ResolveChannelMappingAndRestrict(ctx, groupID, model)
		return mapped.MappedModel
	})
}

func filterModelsForKeyScope(cat groupModelsCatalog, scope string, models []string) []string {
	return filterModelsForKeyScopeMapped(cat, scope, models, nil)
}

func filterModelsForKeyScopeMapped(cat groupModelsCatalog, scope string, models []string, channelModel func(string) string) []string {
	if NormalizeModelScope(scope) == ModelScopeAll {
		return cloneStringSlice(models)
	}
	result := make([]string, 0, len(models))
	for _, model := range models {
		if !ModelScopeAllows(scope, model) {
			continue
		}
		allowed := len(cat.scopeMappings) == 0
		routedModel := strings.TrimPrefix(model, "models/")
		if channelModel != nil {
			routedModel = channelModel(routedModel)
		}
		if !ModelScopeAllows(scope, routedModel) {
			continue
		}
		for _, account := range cat.scopeMappings {
			if account.passthrough {
				allowed = true
				break
			}
			target, matched := resolveRequestedModelInMapping(account.mapping, routedModel)
			if !matched {
				normalized := normalizeRequestedModelForLookup(account.platform, routedModel)
				target, matched = resolveRequestedModelInMapping(account.mapping, normalized)
			}
			if !matched {
				if len(account.mapping) > 0 {
					continue
				}
				target = routedModel
			}
			if ModelScopeAllows(scope, target) {
				allowed = true
				break
			}
		}
		if allowed {
			result = append(result, model)
		}
	}
	return result
}

// Pinned discovery may use an account outside the scheduler's group snapshot.
func FilterModelsForAccountScope(scope string, models []string, account *Account) []string {
	if account == nil || NormalizeModelScope(scope) == ModelScopeAll {
		return FilterModelScope(scope, models)
	}
	return filterModelsForKeyScope(groupModelsCatalog{scopeMappings: []modelScopeMapping{{
		platform: account.Platform, mapping: account.GetModelMapping(), passthrough: account.IsOpenAIPassthroughEnabled(),
	}}}, scope, models)
}

func (s *GatewayService) SupportedKeyProviders(ctx context.Context, group *Group) []string {
	if group == nil {
		return []string{}
	}
	switch group.Platform {
	case PlatformOpenAI:
		return []string{ModelScopeOpenAI}
	case PlatformAnthropic:
		return []string{ModelScopeAnthropic}
	case PlatformMixed:
		// Mixed deliberately has no synthetic fallback: only its configured
		// public mapping keys count. Other platform selectors stay unchanged.
	default:
		if IsCNProvider(group.Platform) {
			return []string{ModelScopeCN}
		}
		return []string{ModelScopeOther}
	}
	cat := s.ensureGroupModelsCatalog(ctx, &group.ID)
	models := cat.unionModels()
	if group.ModelAllowlistEnabled() {
		models = group.ModelAllowlist.FilterForListing(models)
	}
	providers := make([]string, 0, 4)
	vendors := make(map[string]bool)
	for _, scope := range []string{ModelScopeOpenAI, ModelScopeAnthropic, ModelScopeCN, ModelScopeOther} {
		scopedModels := filterModelsForKeyScopeMapped(cat, scope, models, func(model string) string {
			mapped, _ := s.ResolveChannelMappingAndRestrict(ctx, &group.ID, model)
			return mapped.MappedModel
		})
		if len(scopedModels) > 0 {
			providers = append(providers, scope)
		}
	}
	// Legacy all-scope keys can use cross-category aliases too. Their priced
	// vendors are not necessarily the union of the four restricted scopes.
	for _, model := range models {
		for _, target := range s.mixedCatalogueBillingModels(ctx, group, model, cat) {
			vendors[ModelVendor(target)] = true
		}
	}
	// This request-local authorized projection shares the catalogue load above.
	group.SupportedModelVendors = []string{}
	for _, vendor := range mixedModelVendors {
		if vendors[vendor] {
			group.SupportedModelVendors = append(group.SupportedModelVendors, vendor)
		}
	}
	return providers
}

// Return every possible billing identity for a public alias. Different
// accounts may route the same alias to different vendors; never invent one.
func (s *GatewayService) mixedCatalogueBillingModels(ctx context.Context, group *Group, model string, cat groupModelsCatalog) []string {
	groupID := &group.ID
	mapped, _ := s.ResolveChannelMappingAndRestrict(ctx, groupID, model)
	primary := ""
	switch mapped.BillingModelSource {
	case BillingModelSourceRequested:
		primary = model
	case BillingModelSourceChannelMapped:
		if mapped.MappedModel != "" && mapped.MappedModel != model {
			primary = mapped.MappedModel
		}
	}
	// Explicit sources only fix the billing identity when it has a price.
	// Unpriced aliases follow the existing concrete-model fallback instead.
	if primary != "" && (s.billingService == nil && s.resolver == nil || mixedBillingModelPriced(ctx, s.billingService, s.resolver, groupID, group, primary)) {
		return []string{primary}
	}
	upstream := mapped.BillingModelSource == "" || mapped.BillingModelSource == BillingModelSourceUpstream
	seen := make(map[string]bool)
	var targets []string
	for _, account := range cat.scopeMappings {
		target, matched := resolveRequestedModelInMapping(account.mapping, mapped.MappedModel)
		if !matched {
			target, matched = resolveRequestedModelInMapping(account.mapping, normalizeRequestedModelForLookup(account.platform, mapped.MappedModel))
		}
		if !matched && (len(account.mapping) == 0 || account.passthrough) {
			target, matched = mapped.MappedModel, true
		}
		if matched {
			target = mixedBillingPricingIdentity(ctx, s.billingService, s.resolver, groupID, group, target, upstream)
		}
		if matched && target != "" && !strings.Contains(target, "*") && !seen[target] {
			seen[target] = true
			targets = append(targets, target)
		}
	}
	if len(targets) == 0 && primary != "" {
		targets = append(targets, primary)
	}
	sort.Strings(targets)
	return targets
}

func modelsFromSchedulableAccounts(accounts []Account, platform string) []string {
	if len(accounts) == 0 {
		return nil
	}

	filterPlatform := strings.TrimSpace(platform)
	modelSet := make(map[string]struct{})
	for i := range accounts {
		acc := &accounts[i]
		if filterPlatform != "" && acc.Platform != filterPlatform {
			continue
		}
		if acc.IsTypeSafe() {
			if acc.Type == AccountTypeAPIKey && acc.IsModelSupported("jev-latest") && acc.GetMappedModel("jev-latest") == "jev-latest" {
				modelSet["jev-latest"] = struct{}{}
			}
			continue
		}
		if platform == PlatformOpenAI && acc.IsOpenAIPassthroughEnabled() {
			return nil
		}
		for model := range acc.GetModelMapping() {
			modelSet[model] = struct{}{}
		}
	}
	if len(modelSet) == 0 {
		return nil
	}
	models := make([]string, 0, len(modelSet))
	for model := range modelSet {
		models = append(models, model)
	}
	sort.Strings(models)
	if platform == PlatformOpenAI {
		models = supplementUnmappedOpenAIModels(accounts, models)
	}
	return models
}

func catalogHasRequestedModel(cat groupModelsCatalog, requestedModel string) groupCatalogModelPresence {
	sawCatalog := false
	for _, platform := range groupCatalogPlatforms() {
		models := cat.modelsFor(platform)
		if models == nil {
			continue
		}
		sawCatalog = true
		if modelsAdmitRequestedModel(models, requestedModel) {
			return groupCatalogModelPresent
		}
	}
	if !sawCatalog {
		return groupCatalogModelUnknown
	}
	return groupCatalogModelAbsent
}
