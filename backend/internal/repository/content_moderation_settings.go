package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/setting"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// This generation is deliberately scoped to moderation configuration. Delivery
// ledger writes must not invalidate every moderation runtime in the process.
var contentModerationConfigGeneration atomic.Uint64

// moderationTransaction requires the owner, even for an injected transaction
// client: only the owner can defer invalidation until the actual commit.
func moderationTransaction(ctx context.Context, client *dbent.Client, exec sqlExecutor) (*dbent.Tx, bool, error) {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return tx, false, nil
	}
	if tx, ok := exec.(*dbent.Tx); ok && tx != nil {
		return tx, false, nil
	}
	tx, err := client.Tx(ctx)
	if errors.Is(err, dbent.ErrTxStarted) {
		return nil, false, fmt.Errorf("content moderation transaction owner unavailable: %w", err)
	}
	return tx, err == nil, err
}

func afterModerationCommit(tx *dbent.Tx, invalidate func()) {
	tx.OnCommit(func(next dbent.Committer) dbent.Committer {
		return dbent.CommitFunc(func(ctx context.Context, committedTx *dbent.Tx) error {
			if err := next.Commit(ctx, committedTx); err != nil {
				return err
			}
			invalidate()
			return nil
		})
	})
}

// pruneContentModerationGroupIDs keeps opaque JSON values as raw tokens. In
// particular, float64 decoding and PostgreSQL JSONB casts would lose precision
// or reject otherwise valid text containing escaped NULs or lone surrogates.
func pruneContentModerationGroupIDs(value string, groupID int64) (string, bool) {
	var config map[string]json.RawMessage
	if err := json.Unmarshal([]byte(value), &config); err != nil || config == nil {
		return value, false
	}
	raw, exists := config["group_ids"]
	if !exists || !strings.HasPrefix(strings.TrimSpace(string(raw)), "[") {
		return value, false
	}
	var ids []json.RawMessage
	if err := json.Unmarshal(raw, &ids); err != nil {
		return value, false
	}
	kept := make([]json.RawMessage, 0, len(ids))
	changed := false
	for _, rawID := range ids {
		id, err := strconv.ParseInt(strings.TrimSpace(string(rawID)), 10, 64)
		if err == nil && id == groupID {
			changed = true
			continue
		}
		kept = append(kept, rawID)
	}
	if !changed {
		return value, false
	}
	// Both marshals operate on tokens already accepted by json.Unmarshal.
	config["group_ids"], _ = json.Marshal(kept)
	updated, err := json.Marshal(config)
	if err != nil {
		return value, false
	}
	return string(updated), true
}

func pruneContentModerationGroup(ctx context.Context, exec sqlExecutor, groupID int64) (bool, error) {
	var value string
	err := scanSingleRow(ctx, exec, `SELECT value FROM settings WHERE key = $1 FOR UPDATE`,
		[]any{service.SettingKeyContentModerationConfig}, &value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	updated, changed := pruneContentModerationGroupIDs(value, groupID)
	if !changed {
		return false, nil
	}
	_, err = exec.ExecContext(ctx, `UPDATE settings SET value = $2, updated_at = NOW() WHERE key = $1`,
		service.SettingKeyContentModerationConfig, updated)
	return err == nil, err
}

// SetContentModerationConfig serializes a save with group deletion using the
// common lock order: live groups first, then the settings row during upsert.
// Retained IDs are checked even when the configuration currently uses all groups.
func (r *settingRepository) SetContentModerationConfig(ctx context.Context, value string, groupIDs []int64) error {
	tx, owned, err := moderationTransaction(ctx, r.client, nil)
	if err != nil {
		return err
	}
	if owned {
		defer func() { _ = tx.Rollback() }()
	}
	if err := lockLiveGroups(ctx, tx.Client(), groupIDs); err != nil {
		return err
	}
	if _, err := tx.Client().ExecContext(ctx, `INSERT INTO settings (key, value, updated_at)
		VALUES ($1, $2, NOW()) ON CONFLICT (key)
		DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at`,
		service.SettingKeyContentModerationConfig, value); err != nil {
		return err
	}
	afterModerationCommit(tx, func() { contentModerationConfigGeneration.Add(1) })
	if owned {
		return tx.Commit()
	}
	return nil
}

func (r *settingRepository) ContentModerationConfigVersion() uint64 {
	return contentModerationConfigGeneration.Load()
}

// Dispatch authorization must see persisted revocations, bypassing the normal
// settings cache (including its 30-second cross-instance visibility window).
func (r *settingRepository) GetContentModerationDispatchSettings(ctx context.Context) (map[string]string, error) {
	rows, err := r.client.Setting.Query().Where(setting.KeyIn(service.SettingKeyRiskControlEnabled, service.SettingKeyContentModerationConfig)).All(ctx)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string, len(rows))
	for _, row := range rows {
		values[row.Key] = row.Value
	}
	return values, nil
}
