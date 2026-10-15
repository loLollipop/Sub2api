-- Resource bindings for Gemini explicit caching. No route is enabled by this
-- migration. group_id is the actual selected route, not the key's primary group.
-- Keep bindings after an account/key is removed so recovery can reconcile an
-- in-flight upstream mutation before discarding it.
CREATE TABLE IF NOT EXISTS gemini_cached_contents (
    id                   BIGSERIAL PRIMARY KEY,
    public_id            VARCHAR(64) NOT NULL,
    user_id              BIGINT NOT NULL,
    api_key_id           BIGINT NOT NULL,
    group_id             BIGINT NOT NULL,
    account_id           BIGINT NOT NULL,
    upstream_name        VARCHAR(512) NOT NULL,
    model                VARCHAR(255) NOT NULL,
    channel_mapped_model VARCHAR(512) NOT NULL,
    upstream_model       VARCHAR(512) NOT NULL,
    display_name         VARCHAR(512) NOT NULL DEFAULT '',
    total_token_count    BIGINT NOT NULL DEFAULT 0 CHECK (total_token_count >= 0),
    expire_time          TIMESTAMPTZ NOT NULL,
    pricing_at           TIMESTAMPTZ NOT NULL,
    channel_usage        JSONB NOT NULL DEFAULT '{}'::jsonb,
    mutation_token       VARCHAR(64) NULL,
    mutation_lease_until TIMESTAMPTZ NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at           TIMESTAMPTZ NULL,
    CHECK ((mutation_token IS NULL) = (mutation_lease_until IS NULL))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_gemini_cached_contents_public_id
    ON gemini_cached_contents (public_id);
CREATE INDEX IF NOT EXISTS idx_gemini_cached_contents_owner_groups
    ON gemini_cached_contents (api_key_id, group_id, id DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_gemini_cached_contents_account
    ON gemini_cached_contents (account_id);
CREATE INDEX IF NOT EXISTS idx_gemini_cached_contents_pending_mutation
    ON gemini_cached_contents (mutation_lease_until)
    WHERE mutation_token IS NOT NULL AND deleted_at IS NULL;
