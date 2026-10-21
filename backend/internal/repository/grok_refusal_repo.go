package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

var _ service.GrokRefusalRepository = (*usageBillingRepository)(nil)

type grokRefusalQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Bounded SQL batches cover every prefix; no last-N window or global fallback.
func findGrokRefusalJob(ctx context.Context, db grokRefusalQuerier, user int64, keys []string) (string, error) {
	for start := 0; start < len(keys); start += 128 {
		end := start + 128
		if end > len(keys) {
			end = len(keys)
		}
		args := []any{user}
		slots := make([]string, 0, end-start)
		for _, key := range keys[start:end] {
			args = append(args, key)
			slots = append(slots, fmt.Sprintf("$%d", len(args)))
		}
		var id string
		err := db.QueryRowContext(ctx, `SELECT job_id FROM grok_refusal_blocks WHERE user_id = $1 AND identity_hash IN (`+strings.Join(slots, ",")+`) LIMIT 1`, args...).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	}
	return "", nil
}

func (r *usageBillingRepository) FindGrokRefusal(ctx context.Context, user int64, keys []string) (bool, error) {
	if err := r.validateMediaBillingRepository(); err != nil {
		return false, err
	}
	id, err := findGrokRefusalJob(ctx, r.db, user, keys)
	return id != "", err
}

// The short user-scoped transaction serializes only confirmed refusal commits.
// Already in-flight upstream requests can finish, but cannot create a second fee.
// Dedup, accounting effects, the recoverable log job and block are one commit.
func (r *usageBillingRepository) CommitGrokRefusal(ctx context.Context, input *service.MediaBillingJob, lookup, write []string) (*service.MediaBillingJob, bool, error) {
	if err := r.validateMediaBillingRepository(); err != nil {
		return nil, false, err
	}
	job, _, intentJSON, err := normalizeNewMediaBillingJob(input)
	if err != nil {
		return nil, false, err
	}
	if job.Kind != service.MediaBillingKindGrokRefusal || len(write) == 0 {
		return nil, false, errors.New("invalid Grok refusal identity")
	}
	if err := validateMediaBillingIntentOwner(job, job.Intent); err != nil {
		return nil, false, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('grok_refusal:' || $1::text, 0))`, job.UserID); err != nil {
		return nil, false, err
	}
	existingID, err := findGrokRefusalJob(ctx, tx, job.UserID, lookup)
	if err != nil {
		return nil, false, err
	}
	if existingID != "" {
		existing, err := r.getMediaBillingJobByIDTx(ctx, tx, existingID, false)
		if err != nil {
			return nil, false, err
		}
		// Learn additional strong IDs/full histories from an already in-flight
		// duplicate without ever creating another fee.
		if err := insertGrokRefusalKeys(ctx, tx, job.UserID, existingID, write); err != nil {
			return nil, false, err
		}
		if err := tx.Commit(); err != nil {
			return nil, false, err
		}
		return existing, false, nil
	}
	// Request-level idempotency is distinct from conversation identity.
	existing, err := r.getMediaBillingJobByIDTx(ctx, tx, job.ID, false)
	if err == nil {
		if existing.UserID != job.UserID || existing.Kind != job.Kind || existing.Intent == nil || existing.Intent.Command.RequestFingerprint != job.Intent.Command.RequestFingerprint {
			return nil, false, service.ErrUsageBillingRequestConflict
		}
		if err := insertGrokRefusalKeys(ctx, tx, job.UserID, existing.ID, write); err != nil {
			return nil, false, err
		}
		if err := tx.Commit(); err != nil {
			return nil, false, err
		}
		return existing, false, nil
	}
	if !errors.Is(err, service.ErrMediaBillingJobNotFound) {
		return nil, false, err
	}
	cmd := job.Intent.Command
	cmd.Normalize()
	claimed, err := r.claimUsageBillingKey(ctx, tx, &cmd)
	if err != nil {
		return nil, false, err
	}
	if !claimed {
		return nil, false, service.ErrUsageBillingRequestConflict
	}
	effects := &service.UsageBillingApplyResult{Applied: true}
	if err := r.applyUsageBillingEffects(ctx, tx, &cmd, effects); err != nil {
		return nil, false, err
	}
	if err := applyGrokRefusalQuota(ctx, tx, job); err != nil {
		return nil, false, err
	}
	// billed means DB effects/finalization intent already committed. The worker
	// only finalizes the live wallet and writes the idempotent usage row.
	job.Status = service.MediaBillingBilled
	job.AvailableAt = time.Now().Add(30 * time.Second)
	_, err = tx.ExecContext(ctx, `INSERT INTO media_billing_jobs
 (id, kind, user_id, api_key_id, account_id, group_id, status, intent, available_at)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, job.ID, job.Kind, job.UserID, job.APIKeyID, job.AccountID, job.GroupID, job.Status, []byte(intentJSON), job.AvailableAt)
	if err != nil {
		return nil, false, err
	}
	if err := insertGrokRefusalKeys(ctx, tx, job.UserID, job.ID, write); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	job.CommitResult = effects
	return job, true, nil
}

func insertGrokRefusalKeys(ctx context.Context, tx *sql.Tx, user int64, job string, keys []string) error {
	for _, key := range keys {
		if len(key) != 64 {
			return errors.New("invalid Grok refusal hash")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO grok_refusal_blocks (user_id, identity_hash, job_id) VALUES ($1,$2,$3) ON CONFLICT (user_id, identity_hash) DO NOTHING`, user, key, job); err != nil {
			return err
		}
	}
	return nil
}
