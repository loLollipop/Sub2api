//go:build unit

package admin

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestLeaderboardSettingsAuditTracksBothSwitches(t *testing.T) {
	before := &service.SystemSettings{}
	after := &service.SystemSettings{LeaderboardEnabled: true, LeaderboardRequireAuth: true}
	changed := diffSettings(before, after, nil, nil, UpdateSettingsRequest{})
	require.ElementsMatch(t, []string{"leaderboard_enabled", "leaderboard_require_auth"}, changed)
	require.Empty(t, diffSettings(after, after, nil, nil, UpdateSettingsRequest{}))
}
