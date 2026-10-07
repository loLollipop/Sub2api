//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	gocache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/require"
)

func TestListPlazaGroups_OpenAIPassthroughDisplayFallback(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		name := "passthrough only"
		if mixed {
			name = "mixed passthrough and mapped accounts"
		}
		t.Run(name, func(t *testing.T) {
			groupID := int64(10)
			accounts := []Account{{ID: 1, Platform: PlatformOpenAI,
				Extra:       map[string]any{"openai_passthrough": true},
				Credentials: map[string]any{"model_mapping": map[string]any{"stale-model": "upstream"}},
			}}
			if mixed {
				accounts = append(accounts, Account{ID: 2, Platform: PlatformOpenAI,
					Credentials: map[string]any{"model_mapping": map[string]any{"mapped-model": "upstream"}},
				})
			}
			repo := &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: accounts}}
			gateway := &GatewayService{accountRepo: repo, modelsListCache: gocache.New(time.Minute, time.Minute), modelsListCacheTTL: time.Minute}
			svc := ProvideModelPlazaService(&mockChannelRepository{listAllFn: func(context.Context) ([]Channel, error) { return nil, nil }},
				&stubGroupRepoForAvailable{activeGroups: []Group{{ID: groupID, Name: "GPT", Platform: PlatformOpenAI}}}, nil, nil, nil, gateway)
			out, err := svc.ListGroups(context.Background())
			require.NoError(t, err)
			require.Len(t, out, 1)
			names := make([]string, 0, len(out[0].Models))
			for _, model := range out[0].Models {
				names = append(names, model.Name)
			}
			require.ElementsMatch(t, openai.DefaultModelIDs(), names)
			require.NotContains(t, names, "stale-model")
			require.NotContains(t, names, "mapped-model")
			// The display adapter never changes routing's unknown/passthrough semantics.
			require.Nil(t, gateway.GetAvailableModels(context.Background(), &groupID, PlatformOpenAI))
			require.Equal(t, int64(1), repo.listByGroupCalls.Load())
		})
	}
}

func TestListPlazaGroups_DefaultFallbackRequiresSchedulableOpenAI(t *testing.T) {
	for _, accounts := range [][]Account{nil, {{ID: 1, Platform: PlatformAnthropic}}} {
		repo := &modelsListAccountRepoStub{byGroup: map[int64][]Account{10: accounts}}
		gateway := &GatewayService{accountRepo: repo, modelsListCache: gocache.New(time.Minute, time.Minute), modelsListCacheTTL: time.Minute}
		svc := newPlazaService(nil, []Group{{ID: 10, Name: "empty GPT", Platform: PlatformOpenAI}}, nil)
		svc.SetListedModelCatalog(plazaGatewayModelCatalog{gateway: gateway})
		out, err := svc.ListGroups(context.Background())
		require.NoError(t, err)
		require.Empty(t, out)
	}
}

func TestListPlazaGroups_DefaultFallbackAllowlistAndChannelPrice(t *testing.T) {
	repo := &modelsListAccountRepoStub{byGroup: map[int64][]Account{10: {{ID: 1, Platform: PlatformOpenAI, Extra: map[string]any{"openai_passthrough": true}}}}}
	gateway := &GatewayService{accountRepo: repo, modelsListCache: gocache.New(time.Minute, time.Minute), modelsListCacheTTL: time.Minute}
	groups := []Group{{ID: 10, Name: "GPT", Platform: PlatformOpenAI,
		ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"gpt-5", "gpt-6"}},
	}}
	svc := newPlazaService([]Channel{plazaPricedChannel(1, "shop", []int64{10}, PlatformOpenAI, "gpt-5")}, groups, nil)
	svc.SetListedModelCatalog(plazaGatewayModelCatalog{gateway: gateway})
	out, err := svc.ListGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 2)
	byName := make(map[string]PlazaModel)
	for _, model := range out[0].Models {
		byName[model.Name] = model
	}
	require.Contains(t, byName, "gpt-6")
	require.NotContains(t, byName, "gpt-5.6")
	require.NotNil(t, byName["gpt-5"].Pricing)
	require.InDelta(t, 3e-6, *byName["gpt-5"].Pricing.InputPrice, 1e-15)
}
