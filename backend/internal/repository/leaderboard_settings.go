package repository

import (
	"context"

	"github.com/Wei-Shaw/sub2api/ent/setting"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Access decisions must observe persisted revocations on every request, even
// while the ordinary settings cache is warm or the database is unavailable.
func (r *settingRepository) GetLeaderboardAccessSettings(ctx context.Context) (map[string]string, error) {
	rows, err := r.client.Setting.Query().Where(setting.KeyIn(service.SettingKeyLeaderboardEnabled, service.SettingKeyLeaderboardRequireAuth)).All(ctx)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string, len(rows))
	for _, row := range rows {
		values[row.Key] = row.Value
	}
	return values, nil
}
