package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type geminiCachedContentSQLExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type geminiCachedContentRepository struct {
	sql geminiCachedContentSQLExecutor
}

func NewGeminiCachedContentRepository(db *sql.DB) service.GeminiCachedContentRepository {
	return &geminiCachedContentRepository{sql: db}
}

const geminiCachedContentColumns = `id, public_id, user_id, api_key_id, group_id, account_id, upstream_name, model,
 channel_mapped_model, upstream_model, display_name, total_token_count, expire_time, pricing_at, channel_usage,
 COALESCE(mutation_token, ''), mutation_lease_until, created_at, updated_at, upstream_endpoint_identity`

func (r *geminiCachedContentRepository) Create(ctx context.Context, record *service.GeminiCachedContent) error {
	if record == nil || record.APIKeyID <= 0 || record.UserID <= 0 || record.GroupID <= 0 || record.AccountID <= 0 ||
		record.PublicID == "" || record.Model == "" || record.ChannelMappedModel == "" || record.UpstreamModel == "" ||
		record.UpstreamName == "" || !record.HasFrozenEndpoint() || record.PricingAt.IsZero() || record.ExpireTime.IsZero() || record.TotalTokenCount < 0 {
		return errors.New("invalid gemini cache binding")
	}
	channelUsage, err := json.Marshal(record.ChannelUsage)
	if err != nil {
		return err
	}
	row := r.sql.QueryRowContext(ctx, `
INSERT INTO gemini_cached_contents (public_id, user_id, api_key_id, group_id, account_id, upstream_name, model,
 channel_mapped_model, upstream_model, display_name, total_token_count, expire_time, pricing_at, channel_usage, upstream_endpoint_identity)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING id, created_at, updated_at`,
		record.PublicID, record.UserID, record.APIKeyID, record.GroupID, record.AccountID, record.UpstreamName,
		record.Model, record.ChannelMappedModel, record.UpstreamModel, record.DisplayName, record.TotalTokenCount, record.ExpireTime, record.PricingAt, channelUsage, record.UpstreamEndpointIdentity)
	if err := row.Scan(&record.ID, &record.CreatedAt, &record.UpdatedAt); err != nil {
		return translatePersistenceError(err, nil, service.ErrGeminiCachedContentExists)
	}
	return nil
}

func (r *geminiCachedContentRepository) GetForOwner(ctx context.Context, apiKeyID int64, publicID string) (*service.GeminiCachedContent, error) {
	record, err := scanGeminiCachedContent(r.sql.QueryRowContext(ctx, `SELECT `+geminiCachedContentColumns+`
 FROM gemini_cached_contents
 WHERE public_id = $1 AND api_key_id = $2 AND deleted_at IS NULL`, publicID, apiKeyID))
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrGeminiCachedContentNotFound, nil)
	}
	return record, nil
}

func (r *geminiCachedContentRepository) ListForOwner(ctx context.Context, apiKeyID int64, groupIDs []int64, activeAt time.Time, beforeID int64, limit int) ([]*service.GeminiCachedContent, error) {
	if apiKeyID <= 0 || len(groupIDs) == 0 {
		return []*service.GeminiCachedContent{}, nil
	}
	for _, id := range groupIDs {
		if id <= 0 {
			return nil, errors.New("invalid authorized cache group")
		}
	}
	if limit <= 0 {
		limit = service.GeminiCachedContentListDefaultPageSize
	}
	if limit > service.GeminiCachedContentListMaxPageSize {
		limit = service.GeminiCachedContentListMaxPageSize
	}
	rows, err := r.sql.QueryContext(ctx, `SELECT `+geminiCachedContentColumns+`
 FROM gemini_cached_contents
 WHERE api_key_id = $1 AND group_id = ANY($2::bigint[]) AND deleted_at IS NULL AND expire_time > $3 AND ($4 <= 0 OR id < $4)
 ORDER BY id DESC
 LIMIT $5`, apiKeyID, pq.Array(groupIDs), activeAt, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*service.GeminiCachedContent
	for rows.Next() {
		record, err := scanGeminiCachedContent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (r *geminiCachedContentRepository) BeginMutation(ctx context.Context, id int64, baseline time.Time, token string, lease time.Duration) (bool, error) {
	if id <= 0 || baseline.IsZero() {
		return false, errors.New("invalid cache mutation baseline")
	}
	if err := validateGeminiCacheMutation(token, lease); err != nil {
		return false, err
	}
	result, err := r.sql.ExecContext(ctx, `UPDATE gemini_cached_contents
 SET mutation_token = $2, mutation_lease_until = NOW() + make_interval(secs => $3::double precision)
 WHERE id = $1 AND deleted_at IS NULL AND mutation_token IS NULL AND expire_time = $4`,
		id, token, lease.Seconds(), baseline)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected > 0 {
		return true, nil
	}
	var exists bool
	if err := r.sql.QueryRowContext(ctx, `SELECT EXISTS (
 SELECT 1 FROM gemini_cached_contents WHERE id = $1 AND deleted_at IS NULL)`, id).Scan(&exists); err != nil {
		return false, err
	}
	if !exists {
		return false, service.ErrGeminiCachedContentNotFound
	}
	return false, nil
}

func (r *geminiCachedContentRepository) TakeOverStaleMutation(ctx context.Context, id int64, token string, lease time.Duration) (*service.GeminiCachedContent, error) {
	if id <= 0 {
		return nil, errors.New("invalid cache binding ID")
	}
	if err := validateGeminiCacheMutation(token, lease); err != nil {
		return nil, err
	}
	record, err := scanGeminiCachedContent(r.sql.QueryRowContext(ctx, `UPDATE gemini_cached_contents
 SET mutation_token = $2, mutation_lease_until = NOW() + make_interval(secs => $3::double precision)
 WHERE id = $1 AND deleted_at IS NULL AND mutation_token IS NOT NULL AND mutation_lease_until < NOW()
 RETURNING `+geminiCachedContentColumns, id, token, lease.Seconds()))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (r *geminiCachedContentRepository) CommitMutation(ctx context.Context, id int64, token string, expireTime time.Time) error {
	if id <= 0 || strings.TrimSpace(token) == "" || expireTime.IsZero() {
		return errors.New("invalid cache mutation commit")
	}
	result, err := r.sql.ExecContext(ctx, `UPDATE gemini_cached_contents
 SET expire_time = $3, updated_at = NOW(), mutation_token = NULL, mutation_lease_until = NULL
 WHERE id = $1 AND deleted_at IS NULL AND mutation_token = $2 AND mutation_lease_until > NOW()`, id, token, expireTime)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrGeminiCachedContentMutationConflict
	}
	return nil
}

func (r *geminiCachedContentRepository) ReleaseMutation(ctx context.Context, id int64, token string) error {
	if id <= 0 || strings.TrimSpace(token) == "" {
		return errors.New("invalid cache mutation release")
	}
	_, err := r.sql.ExecContext(ctx, `UPDATE gemini_cached_contents
 SET mutation_token = NULL, mutation_lease_until = NULL
 WHERE id = $1 AND mutation_token = $2`, id, token)
	return err
}

func (r *geminiCachedContentRepository) ListStaleMutations(ctx context.Context, limit int) ([]int64, error) {
	if limit <= 0 {
		limit = service.GeminiCachedContentListDefaultPageSize
	}
	if limit > service.GeminiCachedContentListMaxPageSize {
		limit = service.GeminiCachedContentListMaxPageSize
	}
	rows, err := r.sql.QueryContext(ctx, `SELECT id FROM gemini_cached_contents
 WHERE mutation_token IS NOT NULL AND deleted_at IS NULL AND mutation_lease_until < NOW()
 ORDER BY mutation_lease_until
 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *geminiCachedContentRepository) SoftDelete(ctx context.Context, id int64, token string) error {
	if id <= 0 || strings.TrimSpace(token) == "" {
		return errors.New("invalid cache delete lease")
	}
	result, err := r.sql.ExecContext(ctx, `UPDATE gemini_cached_contents
 SET deleted_at = NOW(), updated_at = NOW(), mutation_token = NULL, mutation_lease_until = NULL
 WHERE id = $1 AND deleted_at IS NULL AND mutation_token = $2 AND mutation_lease_until > NOW()`, id, token)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrGeminiCachedContentMutationConflict
	}
	return nil
}

func validateGeminiCacheMutation(token string, lease time.Duration) error {
	if strings.TrimSpace(token) == "" || len(token) > 64 || lease <= 0 || lease > 10*time.Minute {
		return errors.New("invalid gemini cache mutation lease")
	}
	return nil
}

type geminiCachedContentScanner interface {
	Scan(dest ...any) error
}

func scanGeminiCachedContent(row geminiCachedContentScanner) (*service.GeminiCachedContent, error) {
	var record service.GeminiCachedContent
	var channelUsage []byte
	if err := row.Scan(
		&record.ID,
		&record.PublicID,
		&record.UserID,
		&record.APIKeyID,
		&record.GroupID,
		&record.AccountID,
		&record.UpstreamName,
		&record.Model,
		&record.ChannelMappedModel,
		&record.UpstreamModel,
		&record.DisplayName,
		&record.TotalTokenCount,
		&record.ExpireTime,
		&record.PricingAt,
		&channelUsage,
		&record.MutationToken,
		&record.MutationLeaseUntil,
		&record.CreatedAt,
		&record.UpdatedAt,
		&record.UpstreamEndpointIdentity,
	); err != nil {
		return nil, err
	}
	if len(channelUsage) > 0 {
		if err := json.Unmarshal(channelUsage, &record.ChannelUsage); err != nil {
			return nil, err
		}
	}
	return &record, nil
}
