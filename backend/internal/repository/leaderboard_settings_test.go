//go:build unit

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestLeaderboardRuntimeBypassesWarmRepositoryCache(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		enabled string
		auth    string
		err     error
		want    service.LeaderboardRuntime
	}{
		{name: "disabled by another instance", enabled: "false", auth: "false"},
		{name: "authentication required by another instance", enabled: "true", auth: "true", want: service.LeaderboardRuntime{Enabled: true, RequireAuth: true}},
		{name: "database unavailable", err: errors.New("database offline")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock := newSettingRepositorySQLMock(t)
			keys := []string{service.SettingKeyLeaderboardEnabled, service.SettingKeyLeaderboardRequireAuth}
			mock.ExpectQuery(`SELECT .* FROM "settings" WHERE .* IN`).WithArgs(keys[0], keys[1]).
				WillReturnRows(sqlmock.NewRows([]string{"id", "key", "value", "updated_at"}).
					AddRow(int64(1), keys[0], "true", time.Now()).
					AddRow(int64(2), keys[1], "false", time.Now()))
			values, err := repo.GetMultiple(ctx, keys)
			require.NoError(t, err)
			require.Equal(t, "true", values[keys[0]])
			// The ordinary reader really is warm and does not touch the DB again.
			values, err = repo.GetMultiple(ctx, keys)
			require.NoError(t, err)
			require.Equal(t, "false", values[keys[1]])
			require.NoError(t, mock.ExpectationsWereMet())

			query := mock.ExpectQuery(`SELECT .* FROM "settings" WHERE .* IN`).WithArgs(keys[0], keys[1])
			if tc.err != nil {
				query.WillReturnError(tc.err)
			} else {
				query.WillReturnRows(sqlmock.NewRows([]string{"id", "key", "value", "updated_at"}).
					AddRow(int64(1), keys[0], tc.enabled, time.Now()).
					AddRow(int64(2), keys[1], tc.auth, time.Now()))
			}
			svc := service.NewSettingService(repo, &config.Config{})
			require.Equal(t, tc.want, svc.GetLeaderboardRuntime(ctx))
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
