//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type leaderboardRuntimeRepoStub struct{ *settingPublicRepoStub }

func (r *leaderboardRuntimeRepoStub) GetLeaderboardAccessSettings(context.Context) (map[string]string, error) {
	return r.values, r.err
}

func TestLeaderboardSettingsProjectionAndRuntime(t *testing.T) {
	repo := &leaderboardRuntimeRepoStub{&settingPublicRepoStub{values: map[string]string{SettingKeyLeaderboardEnabled: "true", SettingKeyLeaderboardRequireAuth: "true"}}}
	svc := NewSettingService(repo, &config.Config{})
	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.LeaderboardEnabled)
	require.True(t, settings.LeaderboardRequireAuth)
	injected, err := svc.GetPublicSettingsForInjection(context.Background())
	require.NoError(t, err)
	require.True(t, injected.(*PublicSettingsInjectionPayload).LeaderboardEnabled)
	require.True(t, injected.(*PublicSettingsInjectionPayload).LeaderboardRequireAuth)
	parsed := svc.parseSettings(repo.values)
	require.True(t, parsed.LeaderboardEnabled)
	require.True(t, parsed.LeaderboardRequireAuth)
	require.True(t, svc.GetLeaderboardRuntime(context.Background()).Enabled)
	// No public-settings cache may mask a subsequent switch change or DB error.
	repo.values[SettingKeyLeaderboardEnabled] = "false"
	require.False(t, svc.GetLeaderboardRuntime(context.Background()).Enabled)
	repo.err = errors.New("offline")
	require.Equal(t, LeaderboardRuntime{}, svc.GetLeaderboardRuntime(context.Background()))
	repo.err, repo.values = nil, map[string]string{}
	require.Equal(t, LeaderboardRuntime{}, svc.GetLeaderboardRuntime(context.Background()))
	require.False(t, svc.parseSettings(repo.values).LeaderboardEnabled)
	require.False(t, svc.parseSettings(repo.values).LeaderboardRequireAuth)
}

func TestLeaderboardRuntimeWithoutUncachedReaderClosesAccess(t *testing.T) {
	repo := &settingPublicRepoStub{values: map[string]string{SettingKeyLeaderboardEnabled: "true"}}
	svc := NewSettingService(repo, &config.Config{})
	require.Equal(t, LeaderboardRuntime{}, svc.GetLeaderboardRuntime(context.Background()))
}

func TestLeaderboardSettingsDatabaseDefaultsAndPersistence(t *testing.T) {
	repo := &purchaseProductsRepo{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})
	require.NoError(t, svc.InitializeDefaultSettings(context.Background()))
	require.Equal(t, "false", repo.values[SettingKeyLeaderboardEnabled])
	require.Equal(t, "false", repo.values[SettingKeyLeaderboardRequireAuth])
	settings, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	settings.LeaderboardEnabled, settings.LeaderboardRequireAuth = true, true
	require.NoError(t, svc.UpdateSettings(context.Background(), settings))
	require.Equal(t, "true", repo.values[SettingKeyLeaderboardEnabled])
	require.Equal(t, "true", repo.values[SettingKeyLeaderboardRequireAuth])
}
