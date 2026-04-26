DROP INDEX IF EXISTS idx_memory_relations_walk;
DROP INDEX IF EXISTS idx_memory_observations_unreflected;

ALTER TABLE memory_observations
    DROP COLUMN IF EXISTS embedding_model,
    DROP COLUMN IF EXISTS salience,
    DROP COLUMN IF EXISTS layer,
    DROP COLUMN IF EXISTS body_size_bytes,
    DROP COLUMN IF EXISTS summary;

DROP INDEX IF EXISTS idx_memory_entities_about_unique;

ALTER TABLE memory_entities
    DROP COLUMN IF EXISTS title,
    DROP COLUMN IF EXISTS about_key,
    DROP COLUMN IF EXISTS about_kind;
