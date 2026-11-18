-- Cover API key usage aggregation without fetching actual_cost from the heap.
-- Concurrent builds avoid blocking writes. The migration runner clears INVALID
-- remnants before retrying an interrupted build.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_api_key_created_cost
    ON usage_logs (api_key_id, created_at)
    INCLUDE (actual_cost);
