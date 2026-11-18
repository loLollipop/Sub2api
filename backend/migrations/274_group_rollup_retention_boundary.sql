-- Retention publishes retained_from before deleting expired source rows.
-- Those deletes must preserve already published totals for the first retained
-- day. Changes to surviving rows still invalidate their published buckets.
CREATE OR REPLACE FUNCTION invalidate_group_usage_rollup_state()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    affected_date DATE;
    published_before DATE;
    retention_boundary TIMESTAMPTZ;
    configured_timezone TEXT := current_setting('TimeZone');
BEGIN
    SELECT closed_before, retained_from
    INTO published_before, retention_boundary
    FROM usage_group_rollup_state
    WHERE id = 1
    FOR UPDATE;

    IF OLD.group_id IS NOT NULL AND OLD.created_at >= retention_boundary THEN
        affected_date := (OLD.created_at AT TIME ZONE configured_timezone)::date;
    END IF;
    IF TG_OP = 'UPDATE' THEN
        IF NEW.group_id IS NOT NULL AND NEW.created_at >= retention_boundary THEN
            affected_date := LEAST(affected_date,
                (NEW.created_at AT TIME ZONE configured_timezone)::date);
        END IF;
    END IF;

    IF published_before > affected_date THEN
        UPDATE usage_group_rollup_state
        SET closed_before = LEAST(closed_before, affected_date),
            updated_at = NOW()
        WHERE id = 1;
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;
