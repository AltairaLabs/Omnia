DROP INDEX IF EXISTS idx_memory_entities_forgotten_at;
DROP INDEX IF EXISTS idx_memory_entities_consent_category;
ALTER TABLE memory_entities
    DROP COLUMN IF EXISTS forgotten_at,
    DROP COLUMN IF EXISTS consent_category;
