package service

import "context"

type LeaderboardRuntime struct {
	Enabled     bool
	RequireAuth bool
}

type leaderboardAccessSettingsReader interface {
	GetLeaderboardAccessSettings(context.Context) (map[string]string, error)
}

// GetLeaderboardRuntime bypasses all settings caches so disabling access or
// requiring authentication takes effect on the next request. Errors close access.
func (s *SettingService) GetLeaderboardRuntime(ctx context.Context) LeaderboardRuntime {
	repo, ok := s.settingRepo.(leaderboardAccessSettingsReader)
	if !ok {
		return LeaderboardRuntime{}
	}
	values, err := repo.GetLeaderboardAccessSettings(ctx)
	if err != nil {
		return LeaderboardRuntime{}
	}
	return LeaderboardRuntime{Enabled: values[SettingKeyLeaderboardEnabled] == "true", RequireAuth: values[SettingKeyLeaderboardRequireAuth] == "true"}
}
