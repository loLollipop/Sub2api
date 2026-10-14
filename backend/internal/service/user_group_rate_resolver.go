package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	gocache "github.com/patrickmn/go-cache"
	"golang.org/x/sync/singleflight"
)

type userGroupRateResolver struct {
	repo         UserGroupRateRepository
	cache        *gocache.Cache
	cacheTTL     time.Duration
	sf           *singleflight.Group
	logComponent string
	cacheWriteMu sync.Mutex
}

// Cache only the override, including its absence. A resolved fallback belongs
// to one model and must never be reused for another model in the same group.
type cachedUserGroupRate struct {
	Present  bool
	Value    float64
	Revision uint64
}

func (v cachedUserGroupRate) resolve(fallback float64) float64 {
	if v.Present {
		return v.Value
	}
	return fallback
}

func (r *userGroupRateResolver) revision() uint64 {
	if repo, ok := r.repo.(UserGroupRateRevisionRepository); ok {
		return repo.UserGroupRateRevision()
	}
	return 0
}

func (r *userGroupRateResolver) cachedRate(key string, revision uint64) (cachedUserGroupRate, bool) {
	if r.cache != nil {
		if cached, ok := r.cache.Get(key); ok {
			if rate, castOK := cached.(cachedUserGroupRate); castOK && rate.Revision == revision {
				return rate, true
			}
		}
	}
	return cachedUserGroupRate{}, false
}

func (r *userGroupRateResolver) cacheRate(key string, rate cachedUserGroupRate) {
	if r.cache == nil {
		return
	}
	r.cacheWriteMu.Lock()
	defer r.cacheWriteMu.Unlock()
	if r.revision() != rate.Revision {
		return
	}
	r.cache.Set(key, rate, r.cacheTTL)
	// A commit can occur between the check and Set. Remove that old load
	// before a newer load can populate this key under the same local lock.
	if r.revision() != rate.Revision {
		r.cache.Delete(key)
	}
}

func newUserGroupRateResolver(repo UserGroupRateRepository, cache *gocache.Cache, cacheTTL time.Duration, sf *singleflight.Group, logComponent string) *userGroupRateResolver {
	if cacheTTL <= 0 {
		cacheTTL = defaultUserGroupRateCacheTTL
	}
	if cache == nil {
		cache = gocache.New(cacheTTL, time.Minute)
	}
	if logComponent == "" {
		logComponent = "service.gateway"
	}
	if sf == nil {
		sf = &singleflight.Group{}
	}

	return &userGroupRateResolver{
		repo:         repo,
		cache:        cache,
		cacheTTL:     cacheTTL,
		sf:           sf,
		logComponent: logComponent,
	}
}

func (r *userGroupRateResolver) Resolve(ctx context.Context, userID, groupID int64, groupDefaultMultiplier float64) float64 {
	if r == nil || userID <= 0 || groupID <= 0 {
		return groupDefaultMultiplier
	}

	key := fmt.Sprintf("%d:%d", userID, groupID)
	for {
		revision := r.revision()
		if multiplier, ok := r.cachedRate(key, revision); ok {
			if r.revision() != revision {
				continue
			}
			userGroupRateCacheHitTotal.Add(1)
			return multiplier.resolve(groupDefaultMultiplier)
		}
		if r.repo == nil || ctx.Err() != nil {
			return groupDefaultMultiplier
		}
		userGroupRateCacheMissTotal.Add(1)

		// A caller after an admin write must not join an earlier database read.
		flightKey := fmt.Sprintf("%s:%d", key, revision)
		value, err, shared := r.sf.Do(flightKey, func() (any, error) {
			if multiplier, ok := r.cachedRate(key, revision); ok {
				userGroupRateCacheHitTotal.Add(1)
				return multiplier, nil
			}

			userGroupRateCacheLoadTotal.Add(1)
			userRate, repoErr := r.repo.GetByUserAndGroup(ctx, userID, groupID)
			if repoErr != nil {
				return nil, repoErr
			}

			multiplier := cachedUserGroupRate{Revision: revision}
			if userRate != nil {
				multiplier.Present, multiplier.Value = true, *userRate
			}
			r.cacheRate(key, multiplier)
			return multiplier, nil
		})
		if shared {
			userGroupRateCacheSFSharedTotal.Add(1)
		}
		if r.revision() != revision {
			continue
		}
		if err != nil {
			userGroupRateCacheFallbackTotal.Add(1)
			logger.LegacyPrintf(r.logComponent, "get user group rate failed, fallback to group default: user=%d group=%d err=%v", userID, groupID, err)
			return groupDefaultMultiplier
		}

		multiplier, ok := value.(cachedUserGroupRate)
		if !ok {
			userGroupRateCacheFallbackTotal.Add(1)
			return groupDefaultMultiplier
		}
		return multiplier.resolve(groupDefaultMultiplier)
	}
}

// ResolveSnapshotOnly returns a user-specific multiplier only when it is
// already present in the process-local cache. It deliberately never invokes
// the repository. Scheduler/profit admission uses this variant on the
// snapshot-authoritative request path so a cold user×group override cannot
// turn an otherwise database-free model request into a PostgreSQL query.
// Billing paths must continue to use Resolve, which may perform the explicit
// durable lookup needed to calculate the final charge.
func (r *userGroupRateResolver) ResolveSnapshotOnly(userID, groupID int64, groupDefaultMultiplier float64) float64 {
	if r == nil || userID <= 0 || groupID <= 0 {
		return groupDefaultMultiplier
	}
	key := fmt.Sprintf("%d:%d", userID, groupID)
	revision := r.revision()
	if multiplier, ok := r.cachedRate(key, revision); ok && r.revision() == revision {
		userGroupRateCacheHitTotal.Add(1)
		return multiplier.resolve(groupDefaultMultiplier)
	}
	return groupDefaultMultiplier
}
