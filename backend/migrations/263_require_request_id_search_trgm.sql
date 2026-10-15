-- Request-ID fragment search must not silently run without its indexes.
-- Unlike earlier optional search migrations, missing extension privileges are
-- fatal here. A DBA may install pg_trgm first and retry this migration.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_catalog.pg_extension e
        JOIN pg_catalog.pg_namespace n ON n.oid = e.extnamespace
        JOIN pg_catalog.pg_opclass o ON o.opcnamespace = n.oid AND o.opcname = 'gin_trgm_ops'
        JOIN pg_catalog.pg_am a ON a.oid = o.opcmethod AND a.amname = 'gin'
        WHERE e.extname = 'pg_trgm'
          AND pg_catalog.pg_opclass_is_visible(o.oid)
          AND pg_catalog.has_schema_privilege(n.oid, 'USAGE')
    ) THEN
        RAISE EXCEPTION 'request-ID search requires an accessible, visible pg_trgm GIN operator class'
            USING HINT = 'Ask the DBA to install pg_trgm and grant schema USAGE; include its schema in the migration role search_path, then retry.';
    END IF;
END;
$$;
