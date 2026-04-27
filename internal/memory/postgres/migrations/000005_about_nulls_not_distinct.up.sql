-- The structured-key dedup unique index needs to treat two writes
-- with NULL agent_id (or NULL virtual_user_id) as conflicting under
-- the same about_kind/about_key. Postgres' default NULLS DISTINCT
-- behaviour treats NULLs as never equal — so the index doesn't fire
-- ON CONFLICT for institutional or non-agent-scoped writes, and
-- duplicate entities sneak through.
--
-- Postgres 16 (which we ship via pgvector/pgvector:pg16) supports
-- NULLS NOT DISTINCT on unique indexes. Drop and recreate.
DROP INDEX IF EXISTS idx_memory_entities_about_unique;

CREATE UNIQUE INDEX idx_memory_entities_about_unique
    ON memory_entities (workspace_id, virtual_user_id, agent_id,
                        about_kind, about_key)
    NULLS NOT DISTINCT
    WHERE about_kind IS NOT NULL AND NOT forgotten;
