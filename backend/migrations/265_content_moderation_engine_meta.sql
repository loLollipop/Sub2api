-- Nullable for existing moderation, keyword and cyber-policy audit records.
SET LOCAL lock_timeout = '5s';

ALTER TABLE content_moderation_logs ADD COLUMN IF NOT EXISTS engine_meta JSONB;
