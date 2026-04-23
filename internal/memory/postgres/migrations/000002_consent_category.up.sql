-- Phase 4: consent revocation cascade.
--
-- Adds a nullable consent_category column so the memory-api knows which
-- rows to cascade when a user revokes a consent grant. Rows created
-- before this migration have NULL and fall under the default policy.

ALTER TABLE memory_entities
    ADD COLUMN IF NOT EXISTS consent_category TEXT,
    ADD COLUMN IF NOT EXISTS forgotten_at TIMESTAMPTZ;

-- Partial index keeps writes cheap — most rows will have NULL category.
CREATE INDEX IF NOT EXISTS idx_memory_entities_consent_category
    ON memory_entities (workspace_id, virtual_user_id, consent_category)
    WHERE consent_category IS NOT NULL AND forgotten = false;

-- forgotten_at lets the hard-delete pass find rows past grace without
-- scanning every row. updated_at was serving that role in Phase 3 but
-- drifts whenever the row is touched for unrelated reasons.
CREATE INDEX IF NOT EXISTS idx_memory_entities_forgotten_at
    ON memory_entities (forgotten_at)
    WHERE forgotten = true AND forgotten_at IS NOT NULL;
