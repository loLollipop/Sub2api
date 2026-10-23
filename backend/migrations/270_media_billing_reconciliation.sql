-- Unknown video outcomes retain their balance/subscription reservation while
-- automatic polling is paused. Owner queries may still resolve the outcome.
ALTER TABLE media_billing_jobs DROP CONSTRAINT media_billing_jobs_status_valid;
ALTER TABLE media_billing_jobs ADD CONSTRAINT media_billing_jobs_status_valid
    CHECK (status IN ('creating', 'submitted', 'reconciliation', 'ready', 'billed', 'done', 'failed'));
ALTER TABLE media_billing_jobs ADD CONSTRAINT media_billing_jobs_reconciliation_valid
    CHECK (status <> 'reconciliation' OR
        (kind = 'grok_video' AND intent IS NULL AND BTRIM(upstream_task_id) <> ''));

DROP INDEX IF EXISTS media_billing_jobs_subscription_pending_idx;
CREATE INDEX media_billing_jobs_subscription_pending_idx
    ON media_billing_jobs (subscription_id)
    WHERE subscription_id IS NOT NULL
        AND status IN ('creating', 'submitted', 'reconciliation', 'ready');
