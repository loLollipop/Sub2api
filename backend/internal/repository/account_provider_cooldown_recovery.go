package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

var _ service.ProviderCooldownRecoveryRepository = (*accountRepository)(nil)
var _ service.ProviderCooldownProbeRepository = (*accountRepository)(nil)

func (r *accountRepository) SetObservedModelRateLimit(ctx context.Context, id int64, scope string, observedJSON, replacementJSON []byte) (bool, error) {
	var observed, replacement map[string]json.RawMessage
	if scope == "" || json.Unmarshal(observedJSON, &observed) != nil ||
		json.Unmarshal(replacementJSON, &replacement) != nil || len(replacement) == 0 {
		return false, errors.New("invalid observed model cooldown replacement")
	}
	executor := r.sql
	if tx := dbent.TxFromContext(ctx); tx != nil {
		executor = tx.Client()
	}
	result, err := executor.ExecContext(ctx, `
		WITH changed AS (
			UPDATE accounts
			SET extra = jsonb_set(
				jsonb_set(COALESCE(extra, '{}'::jsonb), '{model_rate_limits}', COALESCE(extra->'model_rate_limits', '{}'::jsonb), true),
				ARRAY['model_rate_limits', $2]::text[], $4::jsonb, true), updated_at = NOW()
			WHERE id = $1 AND deleted_at IS NULL
				AND COALESCE(extra->'model_rate_limits'->$2, 'null'::jsonb) = $3::jsonb
			RETURNING id
		)
		INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
		SELECT $5, id, NULL, NULL FROM changed
	`, id, scope, observedJSON, replacementJSON, service.SchedulerOutboxEventAccountChanged)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	// An outer transaction may still roll back. Its durable outbox entry is
	// published only after commit; never expose its uncommitted snapshot here.
	if affected > 0 && dbent.TxFromContext(ctx) == nil {
		r.syncSchedulerAccountSnapshot(ctx, id)
	}
	return affected > 0, nil
}

func (r *accountRepository) ClearObservedModelRateLimit(ctx context.Context, id int64, scope string, observedJSON []byte) (bool, error) {
	var observed map[string]json.RawMessage
	if scope == "" || json.Unmarshal(observedJSON, &observed) != nil || len(observed) == 0 {
		return false, errors.New("invalid observed model cooldown")
	}
	executor := r.sql
	if tx := dbent.TxFromContext(ctx); tx != nil {
		executor = tx.Client()
	}
	result, err := executor.ExecContext(ctx, `
		WITH changed AS (
			UPDATE accounts
			SET extra = extra #- ARRAY['model_rate_limits', $2]::text[], updated_at = NOW()
			WHERE id = $1 AND deleted_at IS NULL
				AND extra->'model_rate_limits'->$2 = $3::jsonb
			RETURNING id
		)
		INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
		SELECT $4, id, NULL, NULL FROM changed
	`, id, scope, observedJSON, service.SchedulerOutboxEventAccountChanged)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected > 0 && dbent.TxFromContext(ctx) == nil {
		r.syncSchedulerAccountSnapshot(ctx, id)
	}
	return affected > 0, nil
}

func (r *accountRepository) ClearObservedTempUnschedulable(ctx context.Context, id int64, observedUntil time.Time, observedReason string) (bool, error) {
	if observedUntil.IsZero() || observedReason == "" {
		return false, nil
	}
	executor := r.sql
	if tx := dbent.TxFromContext(ctx); tx != nil {
		executor = tx.Client()
	}
	result, err := executor.ExecContext(ctx, `
		WITH changed AS (
			UPDATE accounts
			SET temp_unschedulable_until = NULL, temp_unschedulable_reason = NULL, updated_at = NOW()
			WHERE id = $1 AND deleted_at IS NULL
				AND temp_unschedulable_until = $2 AND temp_unschedulable_reason = $3
			RETURNING id
		)
		INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
		SELECT $4, id, NULL, NULL FROM changed
	`, id, observedUntil, observedReason, service.SchedulerOutboxEventAccountChanged)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected > 0 && dbent.TxFromContext(ctx) == nil {
		r.syncSchedulerAccountSnapshot(ctx, id)
	}
	return affected > 0, nil
}
