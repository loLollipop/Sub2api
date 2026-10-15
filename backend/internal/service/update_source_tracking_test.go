//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type updateTrackingClientStub struct {
	updateServiceGitHubClientStub
	heads    map[string]string
	errs     map[string]error
	branches []string
	mu       sync.Mutex
}

type contextAwareUpdateCache struct{ updateServiceCacheStub }

func (c *contextAwareUpdateCache) GetUpdateInfo(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return c.updateServiceCacheStub.GetUpdateInfo(ctx)
}

func (c *contextAwareUpdateCache) SetUpdateInfo(ctx context.Context, data string, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.updateServiceCacheStub.SetUpdateInfo(ctx, data, ttl)
}

type slowUpdateTrackingClient struct {
	*updateTrackingClientStub
	waitRelease bool
	waitRepo    string
}

func (c *slowUpdateTrackingClient) FetchLatestRelease(ctx context.Context, repo string) (*GitHubRelease, error) {
	if c.waitRelease {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return c.updateTrackingClientStub.FetchLatestRelease(ctx, repo)
}

func (c *slowUpdateTrackingClient) FetchBranchHead(ctx context.Context, repo, branch string) (string, error) {
	if repo == c.waitRepo {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return c.updateTrackingClientStub.FetchBranchHead(ctx, repo, branch)
}

func TestUpdateTrackingTimeoutPreservesReleaseFallbackAndPartialCache(t *testing.T) {
	t.Setenv("UPDATE_CHECK_MODE", "upstream")
	client := &slowUpdateTrackingClient{updateTrackingClientStub: newUpdateTrackingClient()}
	client.release.TagName = "v2.1.17"
	cache := &contextAwareUpdateCache{}
	svc := NewUpdateService(cache, client, "2.1.16", "release")
	svc.checkTimeout = 5 * time.Millisecond
	_, err := svc.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	var seeded updateCachePayload
	require.NoError(t, json.Unmarshal([]byte(cache.data), &seeded))
	seeded.ReleaseTimestamp -= 300
	encoded, err := json.Marshal(seeded)
	require.NoError(t, err)
	cache.data = string(encoded)
	client.waitRelease = true
	info, err := svc.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	require.True(t, info.HasUpdate, "a remote timeout must not cancel the cache fallback")
	require.Contains(t, info.Warning, "cached release")
	var cached updateCachePayload
	require.NoError(t, json.Unmarshal([]byte(cache.data), &cached))
	require.Equal(t, seeded.ReleaseTimestamp, cached.ReleaseTimestamp, "failed checks must not renew release age")

	client.waitRelease = false
	client.waitRepo = "Wei-Shaw/sub2api"
	info, err = svc.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	require.True(t, info.HasUpdate)
	require.NotEmpty(t, info.UpstreamUpdates[1].Warning)
	cachedInfo, err := svc.getFromCache(context.Background())
	require.NoError(t, err)
	require.Equal(t, info.UpstreamUpdates, cachedInfo.UpstreamUpdates, "partial results must survive the remote timeout")
}

func TestUpdateTrackingOldReleaseRetainsAgeWarning(t *testing.T) {
	t.Setenv("UPDATE_CHECK_MODE", "upstream")
	cache := &updateServiceCacheStub{}
	svc := NewUpdateService(cache, newUpdateTrackingClient(), "2.1.16", "release")
	_, err := svc.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	var payload updateCachePayload
	require.NoError(t, json.Unmarshal([]byte(cache.data), &payload))
	payload.ReleaseTimestamp = time.Now().Unix() - updateCacheTTL - 1
	encoded, err := json.Marshal(payload)
	require.NoError(t, err)
	cache.data = string(encoded)
	info, err := svc.getFromCache(context.Background())
	require.NoError(t, err)
	require.Contains(t, info.Warning, "older than 20 minutes")
}

func (s *updateTrackingClientStub) FetchBranchHead(_ context.Context, repo, branch string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.branches = append(s.branches, repo+"/"+branch)
	return s.heads[repo], s.errs[repo]
}

func newUpdateTrackingClient() *updateTrackingClientStub {
	return &updateTrackingClientStub{
		updateServiceGitHubClientStub: updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v2.1.12"}},
		heads:                         map[string]string{"kiss-kedaya/sub2api": strings.Repeat("a", 40), "Wei-Shaw/sub2api": originalReviewedCommit},
		errs:                          map[string]error{},
	}
}

func TestUpdateTrackingComparesCheckpointsIndependentlyOfLocalVersion(t *testing.T) {
	t.Setenv("UPDATE_CHECK_MODE", "upstream")
	client := newUpdateTrackingClient()
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "2.1.16", "release")
	info, err := svc.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	require.False(t, info.HasUpdate, "local release is newer than this project's latest release")
	require.True(t, info.ReviewRequired)
	require.Equal(t, []string{personalReleaseRepo}, client.latestRepos)
	require.ElementsMatch(t, []string{"kiss-kedaya/sub2api/main", "Wei-Shaw/sub2api/main"}, client.branches)
	require.Len(t, info.UpstreamUpdates, 2)
	require.True(t, info.UpstreamUpdates[0].HasUpdate)
	require.Equal(t, "https://github.com/kiss-kedaya/sub2api/compare/"+forkImportedCommit+"..."+strings.Repeat("a", 40), info.UpstreamUpdates[0].CompareURL)
	require.False(t, info.UpstreamUpdates[1].HasUpdate)
	require.Empty(t, info.UpstreamUpdates[1].Warning)
}

func TestUpdateTrackingFailuresDoNotSuppressOtherSources(t *testing.T) {
	t.Setenv("UPDATE_CHECK_MODE", "upstream")
	for _, failedRepo := range []string{"kiss-kedaya/sub2api", "Wei-Shaw/sub2api"} {
		t.Run(failedRepo, func(t *testing.T) {
			client := newUpdateTrackingClient()
			client.heads["Wei-Shaw/sub2api"] = strings.Repeat("b", 40)
			client.errs[failedRepo] = errors.New("rate limited")
			client.release.TagName = "v2.1.17"
			svc := NewUpdateService(&updateServiceCacheStub{}, client, "2.1.16", "release")
			info, err := svc.CheckUpdate(context.Background(), true)
			require.NoError(t, err)
			require.True(t, info.HasUpdate)
			for _, source := range info.UpstreamUpdates {
				if source.Repository == failedRepo {
					require.Contains(t, source.Warning, "rate limited")
					require.False(t, source.HasUpdate)
					require.Empty(t, source.LatestCommit)
				} else {
					require.True(t, source.HasUpdate)
				}
			}
		})
	}
}

func TestUpdateTrackingReleaseFailureStillChecksSources(t *testing.T) {
	t.Setenv("UPDATE_CHECK_MODE", "upstream")
	client := newUpdateTrackingClient()
	client.latestErr = errors.New("release unavailable")
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "2.1.16", "release")
	info, err := svc.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	require.Contains(t, info.Warning, "release unavailable")
	require.True(t, info.UpstreamUpdates[0].HasUpdate)
	require.Len(t, client.branches, 2)
	svc.currentVersion = "2.1.17"
	cached, err := svc.getFromCache(context.Background())
	require.NoError(t, err)
	require.Equal(t, "2.1.17", cached.CurrentVersion)
	require.Equal(t, "2.1.17", cached.LatestVersion, "no release data must not retain an old local version")
	require.Contains(t, cached.Warning, "release unavailable")
}

func TestUpdateTrackingUnknownCheckpointsAndInvalidHeads(t *testing.T) {
	t.Setenv("UPDATE_CHECK_MODE", "upstream")
	for _, tt := range []struct{ name, baseline, head string }{
		{"missing checkpoint", "", strings.Repeat("a", 40)},
		{"malformed checkpoint", "not-a-sha", strings.Repeat("a", 40)},
		{"missing head", forkImportedCommit, ""},
		{"short head", forkImportedCommit, "abcdef"},
		{"nonhex head", forkImportedCommit, strings.Repeat("z", 40)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newUpdateTrackingClient()
			client.heads["kiss-kedaya/sub2api"] = tt.head
			svc := NewUpdateService(&updateServiceCacheStub{}, client, "2.1.16", "release")
			svc.upstreamSources[0].commit = tt.baseline
			info, err := svc.CheckUpdate(context.Background(), true)
			require.NoError(t, err)
			require.NotEmpty(t, info.UpstreamUpdates[0].Warning)
			require.False(t, info.UpstreamUpdates[0].HasUpdate)
			require.Empty(t, info.UpstreamUpdates[0].CompareURL)
		})
	}
}

func TestUpdateTrackingOptionalClientAndInvalidMode(t *testing.T) {
	t.Setenv("UPDATE_CHECK_MODE", "upstream")
	client := &updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v2.1.16"}}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "2.1.16", "release")
	info, err := svc.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	require.Len(t, info.UpstreamUpdates, 2)
	for _, source := range info.UpstreamUpdates {
		require.Contains(t, source.Warning, "does not support")
	}
	t.Setenv("UPDATE_CHECK_MODE", "https://example.com/arbitrary")
	svc = NewUpdateService(&updateServiceCacheStub{}, client, "2.1.16", "release")
	info, err = svc.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	require.Equal(t, "release", info.CheckMode)
	require.NotEmpty(t, info.Warning)
	require.Empty(t, info.UpstreamUpdates)
}

func TestUpdateTrackingDefaultReleaseDoesNotFetchBranches(t *testing.T) {
	t.Setenv("UPDATE_CHECK_MODE", "")
	client := newUpdateTrackingClient()
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "2.1.16", "release")
	info, err := svc.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	require.Equal(t, personalReleaseRepo, info.ReleaseRepository)
	require.Empty(t, client.branches)
	require.Empty(t, info.UpstreamUpdates)
}

func TestUpdateTrackingFailureReplacesPreviouslyMatchedCheckpoint(t *testing.T) {
	t.Setenv("UPDATE_CHECK_MODE", "upstream")
	client := newUpdateTrackingClient()
	client.heads["kiss-kedaya/sub2api"] = forkImportedCommit
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "2.1.16", "release")
	first, err := svc.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	require.Empty(t, first.UpstreamUpdates[0].Warning)
	require.False(t, first.UpstreamUpdates[0].HasUpdate)
	client.errs["kiss-kedaya/sub2api"] = errors.New("offline")
	second, err := svc.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	require.NotEmpty(t, second.UpstreamUpdates[0].Warning)
	require.Empty(t, second.UpstreamUpdates[0].LatestCommit)
	require.Empty(t, second.UpstreamUpdates[0].CompareURL)
	cached, err := svc.getFromCache(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, cached.UpstreamUpdates[0].Warning)
	require.Empty(t, cached.UpstreamUpdates[0].LatestCommit)
}

func TestUpdateTrackingCacheIdentityAndLocalRuntimeRecomputed(t *testing.T) {
	t.Setenv("UPDATE_CHECK_MODE", "release")
	client := newUpdateTrackingClient()
	client.release.TagName = "v2.1.17-personal.2"
	cache := &updateServiceCacheStub{}
	releaseService := NewUpdateService(cache, client, "2.1.16", "release")
	_, err := releaseService.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	t.Setenv("UPDATE_CHECK_MODE", "upstream")
	trackingService := NewUpdateService(cache, client, "2.1.16", "release")
	_, err = trackingService.getFromCache(context.Background())
	require.Error(t, err, "release cache must not hide source tracking")
	_, err = trackingService.CheckUpdate(context.Background(), false)
	require.NoError(t, err)
	require.Len(t, client.branches, 2)
	t.Setenv("UPDATE_STRATEGY", "disabled")
	newRuntime := NewUpdateService(cache, client, "2.1.17-personal.1", "source")
	info, err := newRuntime.CheckUpdate(context.Background(), false)
	require.NoError(t, err)
	require.True(t, info.Cached)
	require.True(t, info.HasUpdate, "personal release revisions are ordered")
	require.Equal(t, "2.1.17-personal.1", info.CurrentVersion)
	require.Equal(t, "disabled", info.UpdateStrategy)
	require.Equal(t, "source", info.BuildType)
	newRuntime.currentVersion = "2.1.18"
	info, err = newRuntime.getFromCache(context.Background())
	require.NoError(t, err)
	require.False(t, info.HasUpdate)
	newRuntime.upstreamSources[0].commit = strings.Repeat("c", 40)
	_, err = newRuntime.getFromCache(context.Background())
	require.Error(t, err, "checkpoint changes invalidate source cache")
	newRuntime.upstreamSources = defaultUpdateSourceBaselines()
	newRuntime.upstreamSources[0].repository = "attacker/repo"
	_, err = newRuntime.getFromCache(context.Background())
	require.Error(t, err, "repository identity changes invalidate source cache")
}

func TestUpdateTrackingCacheRejectsTamperedSourceAndLegacyPayload(t *testing.T) {
	t.Setenv("UPDATE_CHECK_MODE", "upstream")
	cache := &updateServiceCacheStub{}
	svc := NewUpdateService(cache, newUpdateTrackingClient(), "2.1.16", "release")
	_, err := svc.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	var payload updateCachePayload
	require.NoError(t, json.Unmarshal([]byte(cache.data), &payload))
	payload.UpstreamUpdates[0].BaselineCommit = strings.Repeat("c", 40)
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	cache.data = string(data)
	_, err = svc.getFromCache(context.Background())
	require.Error(t, err)
	cache.data = `{"latest":"99.0.0","timestamp":9999999999}`
	_, err = svc.getFromCache(context.Background())
	require.Error(t, err)
}

func TestUpdateTrackingNeverDownloadsOrInstallsSourceArtifacts(t *testing.T) {
	t.Setenv("UPDATE_CHECK_MODE", "upstream")
	t.Setenv("UPDATE_STRATEGY", "binary")
	client := newUpdateTrackingClient()
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "2.1.16", "release")
	require.ErrorIs(t, svc.PerformUpdate(context.Background()), ErrNoUpdateAvailable)
	require.Zero(t, client.downloadCalls)
	client.release.TagName = "v2.1.17"
	require.ErrorIs(t, svc.PerformUpdate(context.Background()), ErrUpstreamReviewRequired)
	require.Zero(t, client.downloadCalls)
	for _, repo := range client.latestRepos {
		require.Equal(t, personalReleaseRepo, repo)
	}
}
