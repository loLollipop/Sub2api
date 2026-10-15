package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type geminiBindingGroupRepo struct {
	GroupRepository
	groups map[int64]*Group
}

type geminiBindingUnavailableChannelRepo struct{ ChannelRepository }

func (geminiBindingUnavailableChannelRepo) ListAll(context.Context) ([]Channel, error) {
	return nil, errors.New("policy unavailable")
}

func TestGeminiCachedContentBindingRechecksChannelPricingRestriction(t *testing.T) {
	for _, source := range []string{BillingModelSourceRequested, BillingModelSourceChannelMapped, BillingModelSourceUpstream, BillingModelSourceResponse} {
		for _, allowed := range []bool{false, true} {
			t.Run(source+"/allowed="+map[bool]string{false: "no", true: "yes"}[allowed], func(t *testing.T) {
				now := time.Now()
				group := &Group{ID: 17, Platform: PlatformGemini, Status: StatusActive, Hydrated: true}
				key := &APIKey{ID: 3, UserID: 5, User: &User{ID: 5, Status: StatusActive}, GroupID: &group.ID, Group: group, ModelScope: ModelScopeOther}
				record := &GeminiCachedContent{APIKeyID: 3, UserID: 5, GroupID: 17, AccountID: 21,
					Model: "gemini-public", ChannelMappedModel: "gemini-mapped", UpstreamModel: "models/gemini-upstream", ExpireTime: now.Add(time.Hour)}
				account := &Account{ID: 21, Platform: PlatformGemini, Type: AccountTypeAPIKey, GroupIDs: []int64{17}, Credentials: map[string]any{
					"api_key":       "binding-key",
					"model_mapping": map[string]any{"gemini-mapped": "gemini-upstream"},
				}}
				var identityErr error
				record.UpstreamEndpointIdentity, identityErr = GeminiCachedContentEndpointIdentity(account, record.UpstreamModelID())
				require.NoError(t, identityErr)
				model := "gemini-other"
				if allowed {
					model = billingModelForRestriction(source, record.Model, record.ChannelMappedModel)
					if source == BillingModelSourceUpstream {
						model = record.UpstreamModelID()
					}
				}
				channel := Channel{ID: 1, Status: StatusActive, GroupIDs: []int64{17}, RestrictModels: true, BillingModelSource: source,
					ModelMapping: map[string]map[string]string{PlatformGemini: {record.Model: record.ChannelMappedModel}},
					ModelPricing: []ChannelModelPricing{{Platform: PlatformGemini, Models: []string{model}}},
				}
				channels := &ChannelService{}
				channels.cache.Store(populateChannelCache([]Channel{channel}, map[int64]string{17: PlatformGemini}))
				svc := &GatewayService{channelService: channels}
				routed, err := svc.AuthorizeGeminiCachedContent(context.Background(), key, record, account, now)
				if allowed {
					require.NoError(t, err)
					require.NotNil(t, routed)
				} else {
					require.ErrorIs(t, err, ErrGeminiCachedContentNotFound)
					require.Nil(t, routed)
				}
			})
		}
	}
}

func TestGeminiCachedContentBindingChannelPolicyUnavailableFailsClosed(t *testing.T) {
	now := time.Now()
	group := &Group{ID: 17, Platform: PlatformGemini, Status: StatusActive, Hydrated: true}
	key := &APIKey{ID: 3, UserID: 5, User: &User{ID: 5, Status: StatusActive}, GroupID: &group.ID, Group: group}
	record := &GeminiCachedContent{APIKeyID: 3, UserID: 5, GroupID: 17, AccountID: 21, Model: "gemini-live",
		ChannelMappedModel: "gemini-live", UpstreamModel: "models/gemini-live", ExpireTime: now.Add(time.Hour)}
	account := &Account{ID: 21, Platform: PlatformGemini, Type: AccountTypeAPIKey, GroupIDs: []int64{17}, Credentials: map[string]any{"api_key": "binding-key"}}
	var identityErr error
	record.UpstreamEndpointIdentity, identityErr = GeminiCachedContentEndpointIdentity(account, record.UpstreamModelID())
	require.NoError(t, identityErr)
	svc := &GatewayService{channelService: NewChannelService(geminiBindingUnavailableChannelRepo{}, nil, nil, nil, nil)}
	for i := 0; i < 2; i++ {
		routed, err := svc.AuthorizeGeminiCachedContent(context.Background(), key, record, account, now)
		require.ErrorIs(t, err, ErrGeminiCachedContentNotFound)
		require.Nil(t, routed)
	}
}

func (r geminiBindingGroupRepo) GetByIDLite(_ context.Context, id int64) (*Group, error) {
	return r.groups[id], nil
}

func TestGeminiCachedContentBindingUsesCurrentOwnerAndExactGroup(t *testing.T) {
	for _, changed := range []string{"none", "owner", "user", "unbound", "group disabled", "account unbound", "account changed", "expired", "allowlist", "scope", "mapping changed", "endpoint changed", "key changed", "missing identity"} {
		t.Run(changed, func(t *testing.T) {
			now := time.Now()
			group := &Group{ID: 17, Platform: PlatformGemini, Status: StatusActive, Hydrated: true, RateMultiplier: 0.2}
			primary := &Group{ID: 9, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, RateMultiplier: 0.15}
			key := &APIKey{ID: 3, UserID: 5, User: &User{ID: 5, Status: StatusActive}, GroupID: &primary.ID, Group: primary, RouteGroupIDs: []int64{9, 17}, ModelScope: ModelScopeOther}
			record := &GeminiCachedContent{APIKeyID: 3, UserID: 5, GroupID: 17, AccountID: 21, Model: "gemini-live", ChannelMappedModel: "gemini-live", UpstreamModel: "models/gemini-live", ExpireTime: now.Add(time.Hour)}
			account := &Account{ID: 21, Platform: PlatformGemini, Type: AccountTypeAPIKey, GroupIDs: []int64{17}, Credentials: map[string]any{"api_key": "binding-key"}}
			var identityErr error
			record.UpstreamEndpointIdentity, identityErr = GeminiCachedContentEndpointIdentity(account, record.UpstreamModelID())
			require.NoError(t, identityErr)
			svc := &GatewayService{groupRepo: geminiBindingGroupRepo{groups: map[int64]*Group{9: primary, 17: group}}}
			switch changed {
			case "owner":
				key.ID++
			case "user":
				key.User.ID++
			case "unbound":
				key.RouteGroupIDs = []int64{9}
			case "group disabled":
				group.Status = StatusDisabled
			case "account unbound":
				account.GroupIDs = []int64{9}
			case "account changed":
				account.ID++
			case "expired":
				record.ExpireTime = now
			case "allowlist":
				group.ModelAllowlist = GroupModelAllowlist{Enabled: true, Models: []string{"gemini-other"}}
			case "scope":
				key.ModelScope = ModelScopeCN
			case "mapping changed":
				account.Credentials["model_mapping"] = map[string]any{"gemini-live": "gemini-new"}
			case "endpoint changed":
				account.Credentials["base_url"] = "https://new.invalid"
				account.Credentials[GeminiExplicitCacheUpstreamCredentialKey] = true
			case "key changed":
				account.Credentials["api_key"] = "new-key"
			case "missing identity":
				record.UpstreamEndpointIdentity = ""
			}
			routed, err := svc.AuthorizeGeminiCachedContent(context.Background(), key, record, account, now)
			if changed != "none" {
				require.ErrorIs(t, err, ErrGeminiCachedContentNotFound)
				require.Nil(t, routed)
				return
			}
			require.NoError(t, err)
			require.Equal(t, int64(17), *routed.GroupID)
			require.Equal(t, 0.2, routed.Group.RateMultiplier)
			require.Equal(t, int64(9), *key.GroupID, "resource routing must not mutate the original key")
		})
	}
}
