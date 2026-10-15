//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func moderationIntegrationFixture(t *testing.T) (context.Context, int64, *settingRepository, *groupRepository) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	var old string
	err := integrationDB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = $1`, service.SettingKeyContentModerationConfig).Scan(&old)
	require.True(t, err == nil || err == sql.ErrNoRows)
	existed := err == nil
	var id int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`INSERT INTO groups (name, platform) VALUES ($1, $2) RETURNING id`,
		fmt.Sprintf("moderation-atomic-%d", time.Now().UnixNano()), service.PlatformOpenAI).Scan(&id))
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		if existed {
			_, err = integrationDB.ExecContext(cleanupCtx, `UPDATE settings SET value = $2 WHERE key = $1`, service.SettingKeyContentModerationConfig, old)
		} else {
			_, err = integrationDB.ExecContext(cleanupCtx, `DELETE FROM settings WHERE key = $1`, service.SettingKeyContentModerationConfig)
		}
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(cleanupCtx, `DELETE FROM scheduler_outbox WHERE group_id = $1`, id)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(cleanupCtx, `DELETE FROM groups WHERE id = $1`, id)
		require.NoError(t, err)
		contentModerationConfigGeneration.Add(1)
	})
	client := testEntClient(t)
	return ctx, id, NewSettingRepository(client).(*settingRepository), newGroupRepositoryWithSQL(client, integrationDB)
}

func readModerationIntegrationGroupIDs(t *testing.T, ctx context.Context) []int64 {
	t.Helper()
	var value string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = $1`, service.SettingKeyContentModerationConfig).Scan(&value))
	var cfg struct {
		GroupIDs []int64 `json:"group_ids"`
	}
	require.NoError(t, json.Unmarshal([]byte(value), &cfg))
	return cfg.GroupIDs
}

func TestModerationPostgresSaveDeleteRace(t *testing.T) {
	for _, saveFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "save-locks-first", false: "delete-locks-first"}[saveFirst], func(t *testing.T) {
			ctx, id, settings, groups := moderationIntegrationFixture(t)
			value := fmt.Sprintf(`{"all_groups":true,"group_ids":[%d],"unknown":"\u0000\ud800"}`, id)
			require.NoError(t, settings.SetContentModerationConfig(ctx, value, []int64{id}))
			tx, err := settings.client.Tx(ctx)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()
			txCtx := dbent.NewTxContext(ctx, tx)
			done := make(chan error, 1)
			if saveFirst {
				require.NoError(t, settings.SetContentModerationConfig(txCtx, value, []int64{id}))
				go func() {
					_, err := groups.DeleteCascade(ctx, id)
					done <- err
				}()
			} else {
				_, err = groups.DeleteCascade(txCtx, id)
				require.NoError(t, err)
				go func() { done <- settings.SetContentModerationConfig(ctx, value, []int64{id}) }()
			}
			select {
			case err := <-done:
				t.Fatalf("competing mutation bypassed the group lock: %v", err)
			case <-time.After(50 * time.Millisecond):
			}
			require.NoError(t, tx.Commit())
			if saveFirst {
				require.NoError(t, <-done)
			} else {
				require.ErrorIs(t, <-done, service.ErrGroupNotFound)
			}
			require.Empty(t, readModerationIntegrationGroupIDs(t, ctx))
		})
	}
}

func TestModerationPostgresBorrowedRollbackPreservesGroupAndConfig(t *testing.T) {
	ctx, id, settings, _ := moderationIntegrationFixture(t)
	value := fmt.Sprintf(`{"group_ids":[%d]}`, id)
	require.NoError(t, settings.SetContentModerationConfig(ctx, value, []int64{id}))
	// Warm another instance before the mutation to cover cross-instance eviction.
	reader := NewSettingRepository(settings.client).(*settingRepository)
	_, err := reader.GetValue(ctx, service.SettingKeyContentModerationConfig)
	require.NoError(t, err)
	before := settings.ContentModerationConfigVersion()
	tx, err := settings.client.Tx(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	groups := newGroupRepositoryWithSQL(tx.Client(), tx)
	_, err = groups.DeleteCascade(ctx, id)
	require.NoError(t, err)
	require.Equal(t, before, settings.ContentModerationConfigVersion())
	require.NoError(t, tx.Rollback())
	require.Equal(t, before, settings.ContentModerationConfigVersion())
	require.Equal(t, []int64{id}, readModerationIntegrationGroupIDs(t, ctx))
	var live bool
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT deleted_at IS NULL FROM groups WHERE id = $1`, id).Scan(&live))
	require.True(t, live)
	// The same operation committed must evict the warmed reader immediately.
	_, err = newGroupRepositoryWithSQL(settings.client, integrationDB).DeleteCascade(ctx, id)
	require.NoError(t, err)
	updated, err := reader.GetValue(ctx, service.SettingKeyContentModerationConfig)
	require.NoError(t, err)
	require.Equal(t, `{"group_ids":[]}`, updated)
}
