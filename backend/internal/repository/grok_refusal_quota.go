package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func applyGrokRefusalQuota(ctx context.Context, tx *sql.Tx, job *service.MediaBillingJob) error {
	intent := job.Intent
	if intent.Command.BalanceCost <= 0 || intent.QuotaPlatform == "" {
		return nil
	}
	var raw []byte
	var monthly sql.NullTime
	err := tx.QueryRowContext(ctx, `SELECT grok_refusal_usage, monthly_window_start FROM user_platform_quotas
	 WHERE user_id = $1 AND platform = $2 AND deleted_at IS NULL
	 AND (daily_limit_usd IS NOT NULL OR weekly_limit_usd IS NOT NULL OR monthly_limit_usd IS NOT NULL) FOR UPDATE`, job.UserID, intent.QuotaPlatform).Scan(&raw, &monthly)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var quota service.GrokRefusalQuotaUsage
	if err := json.Unmarshal(raw, &quota); err != nil {
		return err
	}
	var start *time.Time
	if monthly.Valid {
		start = &monthly.Time
	}
	quota.Add(intent.Command.BalanceCost, time.Now(), start)
	raw, err = json.Marshal(&quota)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE user_platform_quotas SET grok_refusal_usage = $3, updated_at = NOW()
	 WHERE user_id = $1 AND platform = $2 AND deleted_at IS NULL`, job.UserID, intent.QuotaPlatform, raw)
	return err
}

func (a *userPlatformQuotaServiceAdapter) GetGrokRefusalQuotaUsage(ctx context.Context, userID int64, platform string) (*service.GrokRefusalQuotaUsage, error) {
	rows, err := clientFromContext(ctx, a.inner.client).QueryContext(ctx, `SELECT grok_refusal_usage FROM user_platform_quotas WHERE user_id = $1 AND platform = $2 AND deleted_at IS NULL`, userID, platform)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, rows.Err()
	}
	var raw []byte
	if err := rows.Scan(&raw); err != nil {
		return nil, err
	}
	var quota service.GrokRefusalQuotaUsage
	if err := json.Unmarshal(raw, &quota); err != nil {
		return nil, err
	}
	return &quota, nil
}
