-- Keep exact B-tree indexes. These additional indexes prepare bounded fragment
-- search. This migration alone does not change any public search semantics.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_request_id_trgm
    ON usage_logs USING gin (request_id gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_upstream_request_id_trgm
    ON usage_logs USING gin (upstream_request_id gin_trgm_ops);

-- Migration033 treated these as optional. Ensure they exist before enabling
-- the same request-ID modes on the admin and user error-list paths.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ops_error_logs_request_id_trgm
    ON ops_error_logs USING gin (request_id gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ops_error_logs_client_request_id_trgm
    ON ops_error_logs USING gin (client_request_id gin_trgm_ops);
