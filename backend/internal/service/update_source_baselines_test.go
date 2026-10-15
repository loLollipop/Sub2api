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
		Repository string `json:"upstream_repository"`
		Version    string `json:"imported_version"`
		Commit     string `json:"imported_commit"`
	}
	read("upstream.lock.json", &fork)
	require.Equal(t, "kiss-kedaya/sub2api", fork.Repository)
	require.Equal(t, forkImportedVersion, fork.Version)
	require.Equal(t, forkImportedCommit, fork.Commit)
	var original struct {
		Repository string `json:"source_repository"`
		Commit     string `json:"latest_reviewed_main_commit"`
	}
	read("original-backports.json", &original)
	require.Equal(t, "Wei-Shaw/sub2api", original.Repository)
	require.Equal(t, originalReviewedCommit, original.Commit)
	for _, name := range []string{"docker-compose.yml", "docker-compose.local.yml", "docker-compose.standalone.yml"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", name))
		require.NoError(t, err)
		require.True(t, strings.Contains(string(data), "UPDATE_CHECK_MODE=${UPDATE_CHECK_MODE:-release}"), name)
	}
}
