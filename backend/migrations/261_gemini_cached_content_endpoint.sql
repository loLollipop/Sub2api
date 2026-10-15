-- Freeze the collection/principal identity before enabling cache lifecycle.
-- Existing bindings intentionally retain an empty value and fail closed. The
-- original endpoint must never be guessed/backfilled from mutable accounts.
ALTER TABLE gemini_cached_contents
    ADD COLUMN IF NOT EXISTS upstream_endpoint_identity VARCHAR(67) NOT NULL DEFAULT ''
    CHECK (upstream_endpoint_identity = '' OR upstream_endpoint_identity ~ '^v1:[0-9a-f]{64}$');
