package service

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	GeminiCachedContentNamePrefix          = "cachedContents/"
	GeminiCachedContentListDefaultPageSize = 50
	GeminiCachedContentListMaxPageSize     = 1000
)

var (
	ErrGeminiCachedContentNotFound         = infraerrors.New(http.StatusNotFound, "GEMINI_CACHED_CONTENT_NOT_FOUND", "gemini cached content not found")
	ErrGeminiCachedContentExists           = infraerrors.New(http.StatusConflict, "GEMINI_CACHED_CONTENT_EXISTS", "gemini cached content already exists")
	ErrGeminiCachedContentMutationConflict = infraerrors.New(http.StatusConflict, "GEMINI_CACHED_CONTENT_MUTATION_CONFLICT", "another update to this cached content is in progress")
)

// GeminiCachedContent binds a public resource to one owner, the actual selected
// group and one holding account. It is not transferable through key failover.
// Persistence alone does not enable the resource's HTTP lifecycle.
type GeminiCachedContent struct {
	ID                       int64
	PublicID                 string
	UserID                   int64
	APIKeyID                 int64
	GroupID                  int64
	AccountID                int64
	UpstreamName             string
	UpstreamEndpointIdentity string // Frozen collection/principal digest; never credentials.
	Model                    string // Original public model; used by scope and public allowlist.
	ChannelMappedModel       string
	UpstreamModel            string
	DisplayName              string
	TotalTokenCount          int64
	ExpireTime               time.Time
	PricingAt                time.Time // Frozen creation quote time, not database insertion time.
	ChannelUsage             ChannelUsageFields
	MutationToken            string
	MutationLeaseUntil       *time.Time
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

func (c *GeminiCachedContent) PublicName() string {
	return GeminiCachedContentNamePrefix + c.PublicID
}

func (c *GeminiCachedContent) UpstreamModelID() string {
	model := c.UpstreamModel
	if i := strings.LastIndex(model, "/"); i >= 0 {
		model = model[i+1:]
	}
	return model
}

type GeminiCachedContentRepository interface {
	Create(context.Context, *GeminiCachedContent) error
	GetForOwner(ctx context.Context, apiKeyID int64, publicID string) (*GeminiCachedContent, error)
	// Empty authorized group IDs must return no records, never all owner rows.
	// Handlers must reauthorize each result against current group/model policy.
	ListForOwner(ctx context.Context, apiKeyID int64, groupIDs []int64, activeAt time.Time, beforeID int64, limit int) ([]*GeminiCachedContent, error)
	BeginMutation(ctx context.Context, id int64, baseline time.Time, token string, lease time.Duration) (bool, error)
	TakeOverStaleMutation(ctx context.Context, id int64, token string, lease time.Duration) (*GeminiCachedContent, error)
	CommitMutation(ctx context.Context, id int64, token string, expireTime time.Time) error
	ReleaseMutation(ctx context.Context, id int64, token string) error
	ListStaleMutations(ctx context.Context, limit int) ([]int64, error)
	// Delete holds the same mutation lease as a TTL change. A stale lease
	// holder cannot delete a binding another worker has taken over.
	SoftDelete(ctx context.Context, id int64, token string) error
}

// ResolveBoundAPIKeyRouteGroup restores a resource's exact current route, not a
// billing task's historical entitlement. It must never select a fallback group.
func (s *GatewayService) ResolveBoundAPIKeyRouteGroup(ctx context.Context, key *APIKey, groupID int64) (*APIKey, error) {
	if key == nil || !slices.Contains(key.CandidateGroupIDs(), groupID) {
		return nil, ErrGroupNotAllowed
	}
	routed, err := s.hydrateAPIKeyGroup(ctx, key, groupID)
	if err != nil {
		return nil, err
	}
	if !apiKeyRouteGroupAllowed(routed) {
		return nil, ErrGroupNotAllowed
	}
	return routed, nil
}

// AuthorizeGeminiCachedContent rechecks the binding against today's key/group,
// mapping and holding-account policy. Subscription/quota admission and feature
// capability checks must additionally run before any upstream mutation. This
// helper neither selects an account nor performs billing/network I/O.
func (s *GatewayService) AuthorizeGeminiCachedContent(ctx context.Context, key *APIKey, record *GeminiCachedContent, account *Account, now time.Time) (*APIKey, error) {
	if key == nil || key.User == nil || !key.User.IsActive() || record == nil || account == nil ||
		key.ID != record.APIKeyID || key.User.ID != record.UserID || account.ID != record.AccountID ||
		account.Platform != PlatformGemini || !record.MatchesEndpoint(account) || !record.ExpireTime.After(now) || !slices.Contains(account.GroupIDs, record.GroupID) {
		return nil, ErrGeminiCachedContentNotFound
	}
	routed, err := s.ResolveBoundAPIKeyRouteGroup(ctx, key, record.GroupID)
	if err != nil || !groupAllowsRequestedModel(routed.Group, record.Model) {
		return nil, ErrGeminiCachedContentNotFound
	}
	var policy *channelCache
	if s.channelService != nil {
		policy, err = s.channelService.loadCache(ctx)
		if err != nil || policy == nil || !policy.policyLoaded {
			return nil, ErrGeminiCachedContentNotFound
		}
	}
	routeCtx, mapped, ok := s.GeminiCatalogRoute(ctx, routed.Group, record.Model)
	if !ok || mapped != record.ChannelMappedModel || !account.IsModelSupported(mapped) ||
		account.GetMappedModel(mapped) != record.UpstreamModelID() {
		return nil, ErrGeminiCachedContentNotFound
	}
	if platform, resolved := ResolvedTargetPlatformFromContext(routeCtx); resolved && platform != account.Platform {
		return nil, ErrGeminiCachedContentNotFound
	}
	// Mapping resolution alone deliberately does not enforce channel pricing
	// restrictions. This resource path does not pass through the scheduler,
	// so apply the same current billing-model restriction here, fail closed
	// if its policy cannot be loaded, and never infer permission from old usage.
	if policy != nil {
		if channel := policy.channelByGroupID[record.GroupID]; channel != nil && channel.IsActive() {
			lookup := &channelLookup{cache: policy, channel: channel, platform: channelLookupPlatform(routeCtx, policy.groupPlatform[record.GroupID])}
			requested := record.Model
			if routed.Group.Platform == PlatformComposite {
				requested, _ = ResolvedUpstreamModelFromContext(routeCtx)
			}
			model := billingModelForRestriction(lookup.channel.BillingModelSource, requested, mapped)
			if lookup.channel.BillingModelSource == BillingModelSourceUpstream {
				model = record.UpstreamModelID()
			}
			if model == "" || checkRestricted(lookup, record.GroupID, model) {
				return nil, ErrGeminiCachedContentNotFound
			}
		}
	}
	return routed, nil
}
