//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type updateServiceCacheStub struct {
	data string
}

func (s *updateServiceCacheStub) GetUpdateInfo(context.Context) (string, error) {
	if s.data == "" {
		return "", errors.New("cache miss")
	}
	return s.data, nil
}

func (s *updateServiceCacheStub) SetUpdateInfo(_ context.Context, data string, _ time.Duration) error {
	s.data = data
	return nil
}

type updateServiceGitHubClientStub struct {
	release        *GitHubRelease
	recentReleases []*GitHubRelease
	recentErr      error
	latestRepos    []string
	recentRepos    []string
	downloadCalls  int
}

func (s *updateServiceGitHubClientStub) FetchLatestRelease(_ context.Context, repo string) (*GitHubRelease, error) {
	s.latestRepos = append(s.latestRepos, repo)
	return s.release, nil
}

func (s *updateServiceGitHubClientStub) FetchRecentReleases(_ context.Context, repo string, _ int) ([]*GitHubRelease, error) {
	s.recentRepos = append(s.recentRepos, repo)
	return s.recentReleases, s.recentErr
}

func (s *updateServiceGitHubClientStub) DownloadFile(context.Context, string, string, int64) error {
	s.downloadCalls++
	return errors.New("unexpected download")
}

func (s *updateServiceGitHubClientStub) FetchChecksumFile(context.Context, string) ([]byte, error) {
	panic("FetchChecksumFile should not be called when no update is available")
}

func TestUpdateServicePerformUpdateNoUpdateReturnsSentinel(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{
			release: &GitHubRelease{
				TagName: "v0.1.132",
				Name:    "v0.1.132",
			},
		},
		"0.1.132",
		"release",
	)

	err := svc.PerformUpdate(context.Background())

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoUpdateAvailable))
	require.ErrorIs(t, err, ErrNoUpdateAvailable)
}

func TestUpdateServiceCheckUpdateUsesUpstreamRepository(t *testing.T) {
	client := &updateServiceGitHubClientStub{
		release: &GitHubRelease{
			TagName: "v2.0.49",
			Name:    "v2.0.49",
			HTMLURL: "https://github.com/kiss-kedaya/sub2api/releases/tag/v2.0.49",
		},
	}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "v2.0.48-personal.1", "release")

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	require.Equal(t, []string{upstreamMonitorRepo}, client.latestRepos)
	require.True(t, info.HasUpdate)
	require.True(t, info.ReviewRequired)
	require.Equal(t, client.release.HTMLURL, info.ReleaseInfo.HTMLURL)
}

func TestUpdateServicePerformUpdateRequiresReviewWithoutDownloadingUpstream(t *testing.T) {
	t.Setenv("UPDATE_STRATEGY", "binary")
	client := &updateServiceGitHubClientStub{
		release: &GitHubRelease{
			TagName: "v2.0.49",
			Assets: []GitHubAsset{{
				Name:               "sub2api_linux_amd64.tar.gz",
				BrowserDownloadURL: "https://github.com/kiss-kedaya/sub2api/releases/download/v2.0.49/sub2api_linux_amd64.tar.gz",
			}},
		},
	}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "v2.0.48-personal.1", "release")

	err := svc.PerformUpdate(context.Background())

	require.ErrorIs(t, err, ErrUpstreamReviewRequired)
	require.Equal(t, "UPSTREAM_REVIEW_REQUIRED", infraerrors.Reason(err))
	require.Equal(t, 0, client.downloadCalls)
}

func TestUpdateServiceDisabledStrategyRejectsUpdateAndRollback(t *testing.T) {
	t.Setenv("UPDATE_STRATEGY", "disabled")

	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{
			release: &GitHubRelease{
				TagName: "v9.9.9",
				Name:    "v9.9.9",
			},
		},
		"0.1.132",
		"release",
	)

	require.ErrorIs(t, svc.PerformUpdate(context.Background()), ErrUpdateDisabled)
	require.ErrorIs(t, svc.Rollback(), ErrUpdateDisabled)
	require.ErrorIs(t, svc.RollbackToVersion(context.Background(), "0.1.131"), ErrUpdateDisabled)
}

func TestUpdateServiceOrchestratedStrategyStillRequiresUpstreamReview(t *testing.T) {
	t.Setenv("UPDATE_STRATEGY", "orchestrated")
	t.Setenv("UPDATE_ORCHESTRATOR", "")

	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{
			release: &GitHubRelease{
				TagName: "v0.1.133",
				Name:    "v0.1.133",
			},
		},
		"0.1.132",
		"release",
	)

	err := svc.PerformUpdate(context.Background())

	require.ErrorIs(t, err, ErrUpstreamReviewRequired)
}

func TestUpdateServiceNeedsRestartDependsOnStrategy(t *testing.T) {
	t.Setenv("UPDATE_STRATEGY", "orchestrated")
	orchestrated := NewUpdateService(&updateServiceCacheStub{}, &updateServiceGitHubClientStub{}, "0.1.132", "release")
	require.False(t, orchestrated.NeedsRestart())

	t.Setenv("UPDATE_STRATEGY", "binary")
	binary := NewUpdateService(&updateServiceCacheStub{}, &updateServiceGitHubClientStub{}, "0.1.132", "release")
	require.True(t, binary.NeedsRestart())
}

func TestNormalizeUpdateErrorPreservesActionableMessage(t *testing.T) {
	err := normalizeUpdateError(errors.New("checksum mismatch for release asset"))

	require.Equal(t, "UPDATE_FAILED", infraerrors.Reason(err))
	require.Equal(t, "checksum mismatch for release asset", infraerrors.Message(err))
}

func TestOrchestratedRollbackRequiresReleaseVersion(t *testing.T) {
	t.Setenv("UPDATE_STRATEGY", "orchestrated")

	svc := NewUpdateService(&updateServiceCacheStub{}, &updateServiceGitHubClientStub{}, "0.1.132", "release")
	err := svc.Rollback()

	require.Equal(t, "ROLLBACK_VERSION_REQUIRED", infraerrors.Reason(err))
}

func newRollbackTestService(current string, releases []*GitHubRelease) *UpdateService {
	return NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentReleases: releases},
		current,
		"release",
	)
}

func TestUpdateServiceListRollbackVersionsFiltersAndCaps(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.148", PublishedAt: "2026-07-09T00:00:00Z"},                       // newer than current: excluded
		{TagName: "v0.1.147", PublishedAt: "2026-07-08T00:00:00Z"},                       // current: excluded
		{TagName: "v0.1.146-rc1", PublishedAt: "2026-07-07T12:00:00Z", Prerelease: true}, // prerelease: excluded
		{TagName: "v0.1.146", PublishedAt: "2026-07-07T00:00:00Z"},
		{TagName: "v0.1.145", PublishedAt: "2026-07-06T00:00:00Z", Draft: true}, // draft: excluded
		{TagName: "v0.1.144", PublishedAt: "2026-07-05T00:00:00Z"},
		{TagName: "v0.1.144", PublishedAt: "2026-07-05T00:00:00Z"}, // duplicate: excluded
		{TagName: "v0.1.143", PublishedAt: "2026-07-04T00:00:00Z"},
		{TagName: "v0.1.142", PublishedAt: "2026-07-03T00:00:00Z"}, // beyond cap of 3: excluded
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.146", versions[0].Version)
	require.Equal(t, "0.1.144", versions[1].Version)
	require.Equal(t, "0.1.143", versions[2].Version)
}

func TestUpdateServiceRollbackCandidatesUsePersonalRepository(t *testing.T) {
	client := &updateServiceGitHubClientStub{
		recentReleases: []*GitHubRelease{{TagName: "v2.0.47-personal.1"}},
	}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "v2.0.48-personal.1", "release")

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 1)
	require.Equal(t, []string{personalReleaseRepo}, client.recentRepos)
}

func TestCompareVersionsIgnoresPersonalTagSuffix(t *testing.T) {
	require.Equal(t, 0, compareVersions("v2.0.48-personal.1", "2.0.48"))
	require.Less(t, compareVersions("v2.0.48-personal.1", "v2.0.49"), 0)
}

func TestComparePersonalReleaseVersionsOrdersPersonalRevision(t *testing.T) {
	require.Greater(t, comparePersonalReleaseVersions("v2.0.48-personal.2", "v2.0.48-personal.1"), 0)
	require.Less(t, comparePersonalReleaseVersions("v2.0.48-personal.1", "v2.0.48-personal.2"), 0)
	require.Greater(t, comparePersonalReleaseVersions("v2.0.49-personal.1", "v2.0.48-personal.99"), 0)
}

func TestUpdateServiceRollbackCandidatesOrderPersonalRevisions(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v2.0.48-personal.1"},
		{TagName: "v2.0.48-personal.3"},
		{TagName: "v2.0.47-personal.9"},
		{TagName: "v2.0.48-personal.2"},
	}
	svc := newRollbackTestService("v2.0.48-personal.3", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Equal(t, []RollbackVersion{
		{Version: "2.0.48-personal.2"},
		{Version: "2.0.48-personal.1"},
		{Version: "2.0.47-personal.9"},
	}, versions)
}

func TestUpdateServiceListRollbackVersionsSortsUnorderedInput(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.144"},
		{TagName: "v0.1.146"},
		{TagName: "v0.1.145"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.146", versions[0].Version)
	require.Equal(t, "0.1.145", versions[1].Version)
	require.Equal(t, "0.1.144", versions[2].Version)
}

func TestUpdateServiceListRollbackVersionsEmptyWhenNoneOlder(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.147"},
		{TagName: "v0.1.148"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Empty(t, versions)
}

func TestUpdateServiceListRollbackVersionsPropagatesFetchError(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentErr: errors.New("github unavailable")},
		"0.1.147",
		"release",
	)

	_, err := svc.ListRollbackVersions(context.Background())

	require.Error(t, err)
	require.Contains(t, err.Error(), "github unavailable")
}

func TestUpdateServiceRollbackToVersionRejectsDisallowedTargets(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.148"},
		{TagName: "v0.1.147"},
		{TagName: "v0.1.146"},
		{TagName: "v0.1.145"},
		{TagName: "v0.1.144"},
		{TagName: "v0.1.143"},
		{TagName: "v0.1.142"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	for _, target := range []string{
		"",         // empty
		"0.1.147",  // current version
		"v0.1.147", // current version with prefix
		"0.1.148",  // newer than current
		"0.1.142",  // older than the 3 most recent
		"9.9.9",    // nonexistent
	} {
		err := svc.RollbackToVersion(context.Background(), target)
		require.ErrorIs(t, err, ErrRollbackVersionNotAllowed, "target %q should be rejected", target)
	}
}

func TestUpdateServiceRollbackToVersionAcceptsVPrefix(t *testing.T) {
	// No platform asset in the release: the target passes the allowlist check
	// and fails later at asset lookup, proving the version itself was accepted.
	releases := []*GitHubRelease{
		{TagName: "v0.1.147"},
		{TagName: "v0.1.146"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	err := svc.RollbackToVersion(context.Background(), "v0.1.146")

	require.Error(t, err)
	require.NotErrorIs(t, err, ErrRollbackVersionNotAllowed)
	require.Contains(t, err.Error(), "no compatible release found")
}
