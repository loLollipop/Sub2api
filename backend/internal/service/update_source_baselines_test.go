//go:build unit

package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdateSourceCheckpointsMatchMaintenanceRecords(t *testing.T) {
	read := func(name string, value any) {
		data, err := os.ReadFile(filepath.Join("..", "..", "..", "maintenance", name))
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(data, value))
	}
	var fork struct {
		Repository    string `json:"upstream_repository"`
		Version       string `json:"imported_version"`
		Commit        string `json:"imported_commit"`
		ReleaseTag    string `json:"reviewed_release_tag"`
		ReleaseCommit string `json:"reviewed_release_commit"`
	}
	read("upstream.lock.json", &fork)
	require.Equal(t, "kiss-kedaya/sub2api", fork.Repository)
	require.Equal(t, forkImportedVersion, fork.Version)
	require.Equal(t, forkImportedCommit, fork.Commit)
	require.Equal(t, defaultUpdateSourceBaselines()[0].releaseTag, fork.ReleaseTag)
	require.Equal(t, defaultUpdateSourceBaselines()[0].releaseCommit, fork.ReleaseCommit)
	var original struct {
		Repository    string `json:"source_repository"`
		Commit        string `json:"latest_reviewed_main_commit"`
		ReleaseTag    string `json:"reviewed_release_tag"`
		ReleaseCommit string `json:"reviewed_release_commit"`
	}
	read("original-backports.json", &original)
	require.Equal(t, "Wei-Shaw/sub2api", original.Repository)
	require.Equal(t, originalReviewedCommit, original.Commit)
	require.Equal(t, defaultUpdateSourceBaselines()[1].releaseTag, original.ReleaseTag)
	require.Equal(t, defaultUpdateSourceBaselines()[1].releaseCommit, original.ReleaseCommit)
	for _, name := range []string{"docker-compose.yml", "docker-compose.local.yml", "docker-compose.standalone.yml"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", name))
		require.NoError(t, err)
		require.True(t, strings.Contains(string(data), "UPDATE_CHECK_MODE=${UPDATE_CHECK_MODE:-release}"), name)
	}
}

func TestUpdateSourceOfficialReleaseCheckpointsAreSeparateFromReviewedMain(t *testing.T) {
	sources := defaultUpdateSourceBaselines()
	require.Len(t, sources, 2)
	require.Equal(t, "v2.2.6", sources[0].releaseTag)
	require.Equal(t, forkImportedCommit, sources[0].releaseCommit)
	require.Equal(t, "v0.2.17", sources[1].releaseTag)
	require.Equal(t, "060d0160417782cd962a0c7f460f6cd4565d4916", sources[1].releaseCommit)
	require.NotEqual(t, sources[1].commit, sources[1].releaseCommit, "reviewed main can be later than the latest official release")
}
