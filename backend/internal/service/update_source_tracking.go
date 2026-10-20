package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	updateCheckRelease  = "release"
	updateCheckUpstream = "upstream"
)

// GitHubBranchHeadClient is retained for compatibility; release monitoring never calls it.
type GitHubBranchHeadClient interface {
	FetchBranchHead(ctx context.Context, repo, branch string) (string, error)
}

// GitHubReleaseCommitClient resolves a release tag to its exact commit, without
// falling back to target_commitish or a moving branch. It is optional so existing
// release-only clients remain compatible.
type GitHubReleaseCommitClient interface {
	FetchReleaseCommit(ctx context.Context, repo, tag string) (string, error)
}

type UpstreamUpdateInfo struct {
	BaselineReleaseTag string `json:"baseline_release_tag"`
	ReleaseTag         string `json:"release_tag"`
	PublishedAt        string `json:"published_at"`
	ReleaseURL         string `json:"release_url"`
	Repository         string `json:"repository"`
	Branch             string `json:"branch"`
	BaselineCommit     string `json:"baseline_commit"`
	LatestCommit       string `json:"latest_commit"`
	HasUpdate          bool   `json:"has_update"`
	CompareURL         string `json:"compare_url"`
	Warning            string `json:"warning,omitempty"`
}

type updateCachePayload struct {
	Identity         string               `json:"identity"`
	Latest           string               `json:"latest"`
	ReleaseInfo      *ReleaseInfo         `json:"release_info"`
	Timestamp        int64                `json:"timestamp"`
	ReleaseTimestamp int64                `json:"release_timestamp"`
	Warning          string               `json:"warning,omitempty"`
	UpstreamUpdates  []UpstreamUpdateInfo `json:"upstream_updates,omitempty"`
}

func loadUpdateCheckMode() (string, string) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("UPDATE_CHECK_MODE")))
	switch mode {
	case "", updateCheckRelease:
		return updateCheckRelease, ""
	case updateCheckUpstream:
		return updateCheckUpstream, ""
	default:
		return updateCheckRelease, "Invalid UPDATE_CHECK_MODE; using release checks"
	}
}

func validUpdateCommit(commit string) bool {
	if len(commit) != 40 {
		return false
	}
	_, err := hex.DecodeString(commit)
	return err == nil
}

func allowedUpdateSource(repo, branch string) bool {
	return branch == "main" && (repo == "kiss-kedaya/sub2api" || repo == "Wei-Shaw/sub2api")
}

func joinUpdateWarnings(first, second string) string {
	if first == "" {
		return second
	}
	if second == "" || first == second {
		return first
	}
	return first + "; " + second
}

func validUpdateReleaseTag(tag string) bool {
	if tag == "" || strings.TrimSpace(tag) != tag || strings.ContainsAny(tag, " ~^:?*[\\\t\r\n\x00") {
		return false
	}
	for _, part := range strings.Split(tag, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	for _, char := range tag {
		if char < 32 || char == 127 {
			return false
		}
	}
	return true
}

func upstreamReleaseURL(repo, tag string) string {
	return fmt.Sprintf("https://github.com/%s/releases/tag/%s", repo, url.PathEscape(tag))
}

func validUpstreamRelease(info UpstreamUpdateInfo) bool {
	_, err := time.Parse(time.RFC3339, info.PublishedAt)
	return validUpdateReleaseTag(info.ReleaseTag) && err == nil &&
		info.ReleaseURL == upstreamReleaseURL(info.Repository, info.ReleaseTag)
}

func (s *UpdateService) checkUpstreamSources(ctx context.Context) []UpstreamUpdateInfo {
	if s.checkMode != updateCheckUpstream {
		return nil
	}
	client, _ := s.githubClient.(GitHubReleaseCommitClient)
	updates := make([]UpstreamUpdateInfo, len(s.upstreamSources))
	var checks sync.WaitGroup
	for i, source := range s.upstreamSources {
		checks.Add(1)
		go func() {
			defer checks.Done()
			updates[i] = checkUpstreamSource(ctx, s.githubClient, client, source)
		}()
	}
	checks.Wait()
	return updates
}

func checkUpstreamSource(ctx context.Context, releases GitHubReleaseClient, commits GitHubReleaseCommitClient, source updateSourceBaseline) UpstreamUpdateInfo {
	info := UpstreamUpdateInfo{Repository: source.repository, Branch: source.branch, BaselineCommit: source.releaseCommit, BaselineReleaseTag: source.releaseTag}
	if !allowedUpdateSource(source.repository, source.branch) {
		info.Warning = "Unsupported upstream source"
		return info
	}
	if !validUpdateCommit(source.releaseCommit) || !validUpdateReleaseTag(source.releaseTag) {
		info.Warning = "Reviewed release checkpoint missing or invalid; review status is unknown"
	}
	if commits == nil {
		info.Warning = joinUpdateWarnings(info.Warning, "GitHub client does not support release commit tracking")
		return info
	}
	release, err := releases.FetchLatestRelease(ctx, source.repository)
	if err != nil {
		info.Warning = joinUpdateWarnings(info.Warning, "Release check failed: "+err.Error())
		return info
	}
	if release == nil || release.Draft || release.Prerelease || !validUpdateReleaseTag(release.TagName) {
		info.Warning = joinUpdateWarnings(info.Warning, "No valid official release; review status is unknown")
		return info
	}
	info.ReleaseTag, info.PublishedAt = release.TagName, release.PublishedAt
	info.ReleaseURL = upstreamReleaseURL(source.repository, release.TagName)
	if !validUpstreamRelease(info) {
		info.Warning = joinUpdateWarnings(info.Warning, "Invalid release publication metadata; review status is unknown")
		info.ReleaseTag, info.PublishedAt, info.ReleaseURL = "", "", ""
		return info
	}
	commit, err := commits.FetchReleaseCommit(ctx, source.repository, release.TagName)
	if err != nil {
		info.Warning = joinUpdateWarnings(info.Warning, "Release commit check failed: "+err.Error())
	} else if !validUpdateCommit(commit) {
		info.Warning = joinUpdateWarnings(info.Warning, "GitHub returned an invalid release commit; review status is unknown")
	} else {
		info.LatestCommit = strings.ToLower(commit)
		if info.Warning == "" {
			info.BaselineCommit = strings.ToLower(info.BaselineCommit)
			info.HasUpdate = info.ReleaseTag != source.releaseTag || info.LatestCommit != info.BaselineCommit
			info.CompareURL = fmt.Sprintf("https://github.com/%s/compare/%s...%s", source.repository, info.BaselineCommit, info.LatestCommit)
		}
	}
	return info
}

func hasUpstreamUpdate(updates []UpstreamUpdateInfo) bool {
	for _, info := range updates {
		if info.HasUpdate {
			return true
		}
	}
	return false
}

func (s *UpdateService) updateCacheIdentity() string {
	identity := []string{"v3-official-releases", personalReleaseRepo, s.checkMode}
	if s.checkMode == updateCheckUpstream {
		for _, source := range s.upstreamSources {
			identity = append(identity, source.repository, source.branch, source.releaseTag, source.releaseCommit)
		}
	}
	data, _ := json.Marshal(identity)
	return string(data)
}

// Validate cached identity and recompute derived state. An old/faulty payload
// must never turn a missing checkpoint or failed check into "reviewed".
func (s *UpdateService) validateCachedUpstream(updates []UpstreamUpdateInfo) error {
	if s.checkMode != updateCheckUpstream {
		if len(updates) != 0 {
			return fmt.Errorf("unexpected cached upstream sources")
		}
		return nil
	}
	if len(updates) != len(s.upstreamSources) {
		return fmt.Errorf("cached upstream sources missing")
	}
	for i, source := range s.upstreamSources {
		info := &updates[i]
		if !allowedUpdateSource(info.Repository, info.Branch) || info.Repository != source.repository || info.Branch != source.branch || !strings.EqualFold(info.BaselineCommit, source.releaseCommit) || info.BaselineReleaseTag != source.releaseTag {
			return fmt.Errorf("cached upstream source mismatch")
		}
		valid := validUpdateReleaseTag(info.BaselineReleaseTag) && validUpdateCommit(info.BaselineCommit) && validUpdateCommit(info.LatestCommit) && validUpstreamRelease(*info)
		if (info.ReleaseTag != "" || info.PublishedAt != "" || info.ReleaseURL != "") && !validUpstreamRelease(*info) {
			return fmt.Errorf("cached release metadata invalid")
		}
		if !valid && info.Warning == "" {
			return fmt.Errorf("cached source status incomplete")
		}
		info.HasUpdate = valid && info.Warning == "" && (info.ReleaseTag != source.releaseTag || !strings.EqualFold(info.BaselineCommit, info.LatestCommit))
		info.CompareURL = ""
		if valid && info.Warning == "" {
			info.CompareURL = fmt.Sprintf("https://github.com/%s/compare/%s...%s", source.repository, strings.ToLower(info.BaselineCommit), strings.ToLower(info.LatestCommit))
		}
	}
	return nil
}
