package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

const (
	updateCheckRelease  = "release"
	updateCheckUpstream = "upstream"
)

// GitHubBranchHeadClient is optional so existing release-only clients remain
// compatible. Branch tracking cannot supply installable release assets.
type GitHubBranchHeadClient interface {
	FetchBranchHead(ctx context.Context, repo, branch string) (string, error)
}

type UpstreamUpdateInfo struct {
	Repository     string `json:"repository"`
	Branch         string `json:"branch"`
	BaselineCommit string `json:"baseline_commit"`
	LatestCommit   string `json:"latest_commit"`
	HasUpdate      bool   `json:"has_update"`
	CompareURL     string `json:"compare_url"`
	Warning        string `json:"warning,omitempty"`
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

func (s *UpdateService) checkUpstreamSources(ctx context.Context) []UpstreamUpdateInfo {
	if s.checkMode != updateCheckUpstream {
		return nil
	}
	client, supported := s.githubClient.(GitHubBranchHeadClient)
	updates := make([]UpstreamUpdateInfo, len(s.upstreamSources))
	var checks sync.WaitGroup
	for i, source := range s.upstreamSources {
		checks.Add(1)
		go func() {
			defer checks.Done()
			updates[i] = checkUpstreamSource(ctx, client, supported, source)
		}()
	}
	checks.Wait()
	return updates
}

func checkUpstreamSource(ctx context.Context, client GitHubBranchHeadClient, supported bool, source updateSourceBaseline) UpstreamUpdateInfo {
	info := UpstreamUpdateInfo{Repository: source.repository, Branch: source.branch, BaselineCommit: source.commit}
	if !allowedUpdateSource(source.repository, source.branch) {
		info.Warning = "Unsupported upstream source"
		return info
	}
	if !validUpdateCommit(source.commit) {
		info.Warning = "Source checkpoint missing or invalid; review status is unknown"
	}
	if !supported {
		info.Warning = joinUpdateWarnings(info.Warning, "GitHub client does not support source tracking")
	} else if head, err := client.FetchBranchHead(ctx, source.repository, source.branch); err != nil {
		info.Warning = joinUpdateWarnings(info.Warning, "Source check failed: "+err.Error())
	} else if !validUpdateCommit(head) {
		info.Warning = joinUpdateWarnings(info.Warning, "GitHub returned an invalid branch commit; review status is unknown")
	} else {
		info.LatestCommit = strings.ToLower(head)
		if validUpdateCommit(source.commit) {
			info.BaselineCommit = strings.ToLower(source.commit)
			info.HasUpdate = info.LatestCommit != info.BaselineCommit
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
	identity := []string{"v2", personalReleaseRepo, s.checkMode}
	if s.checkMode == updateCheckUpstream {
		for _, source := range s.upstreamSources {
			identity = append(identity, source.repository, source.branch, source.commit)
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
		if !allowedUpdateSource(info.Repository, info.Branch) || info.Repository != source.repository || info.Branch != source.branch || !strings.EqualFold(info.BaselineCommit, source.commit) {
			return fmt.Errorf("cached upstream source mismatch")
		}
		valid := validUpdateCommit(info.BaselineCommit) && validUpdateCommit(info.LatestCommit)
		if !valid && info.Warning == "" {
			return fmt.Errorf("cached source status incomplete")
		}
		info.HasUpdate = valid && !strings.EqualFold(info.BaselineCommit, info.LatestCommit)
		info.CompareURL = ""
		if valid {
			info.CompareURL = fmt.Sprintf("https://github.com/%s/compare/%s...%s", source.repository, strings.ToLower(info.BaselineCommit), strings.ToLower(info.LatestCommit))
		}
	}
	return nil
}
