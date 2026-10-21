-- Additive only: existing installations keep all previous migration checksums.
ALTER TABLE groups ADD COLUMN IF NOT EXISTS grok_moderation_refusal_price NUMERIC(20,8);
ALTER TABLE groups ADD CONSTRAINT groups_grok_refusal_price_valid CHECK
 (grok_moderation_refusal_price IS NULL OR
  (grok_moderation_refusal_price >= 0 AND grok_moderation_refusal_price < 'Infinity'::numeric));
ALTER TABLE media_billing_jobs DROP CONSTRAINT media_billing_jobs_kind_valid;
ALTER TABLE media_billing_jobs ADD CONSTRAINT media_billing_jobs_kind_valid
 CHECK (kind IN ('grok_video', 'image_usage', 'grok_refusal'));
CREATE TABLE grok_refusal_blocks (
 user_id BIGINT NOT NULL REFERENCES users(id),
 identity_hash CHAR(64) NOT NULL CHECK (identity_hash ~ '^[0-9a-f]{64}$'),
 job_id TEXT NOT NULL REFERENCES media_billing_jobs(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 PRIMARY KEY (user_id, identity_hash)
);
COMMENT ON TABLE grok_refusal_blocks IS 'Permanent user-scoped Grok refusal identities (SHA-256 only), atomically committed with the frozen billing job';

-- This component is committed with the refusal, independently of absolute Redis
-- dirty snapshots. The base quota counters and flusher never write this column.
ALTER TABLE user_platform_quotas ADD COLUMN grok_refusal_usage JSONB NOT NULL DEFAULT '{}'::jsonb;
