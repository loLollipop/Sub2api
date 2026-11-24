package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContentModerationDisabledMasterCannotBanOrCount(t *testing.T) {
	userID := int64(8)
	repo := &banCountArgsTestRepo{}
	users := &contentModerationTestUserRepo{user: &User{ID: userID, Role: RoleUser, Status: StatusActive}}
	svc := &ContentModerationService{repo: repo, userRepo: users}
	log := &ContentModerationLog{UserID: &userID, Flagged: true}
	cfg := &ContentModerationConfig{Enabled: false, AutoBanEnabled: true, BanThreshold: 1, ViolationWindowHours: 24}
	require.False(t, svc.applyFlaggedAccountSideEffects(context.Background(), cfg, log))
	require.Empty(t, repo.snapshotCountCalls())
	require.Empty(t, users.updated)
	require.Zero(t, log.ViolationCount)
	require.False(t, log.AutoBanned)

	parsed, err := parseContentModerationConfig(`{"enabled":false,"auto_ban_enabled":true}`)
	require.NoError(t, err)
	require.False(t, parsed.AutoBanEnabled)
}

func TestContentModerationCyberDefaultHonorsExplicitPolicy(t *testing.T) {
	for _, raw := range []string{"", `{}`, `{"enabled":true}`} {
		cfg, err := parseContentModerationConfig(raw)
		require.NoError(t, err)
		require.True(t, cfg.CyberPolicyExcludeFromBanCount)
	}
	cfg, err := parseContentModerationConfig(`{"enabled":true,"cyber_policy_exclude_from_ban_count":false}`)
	require.NoError(t, err)
	require.False(t, cfg.CyberPolicyExcludeFromBanCount)
}
