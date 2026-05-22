DROP INDEX IF EXISTS idx_memory_entities_mutability;
DROP INDEX IF EXISTS idx_memory_observations_mutability;

ALTER TABLE memory_entities
    DROP COLUMN IF EXISTS promotion_proposal_id,
    DROP COLUMN IF EXISTS promoted_at,
    DROP COLUMN IF EXISTS promoted_by_pack,
    DROP COLUMN IF EXISTS promoted_from_ids,
    DROP COLUMN IF EXISTS mutability;

ALTER TABLE memory_observations
    DROP COLUMN IF EXISTS promotion_proposal_id,
    DROP COLUMN IF EXISTS promoted_at,
    DROP COLUMN IF EXISTS promoted_by_pack,
    DROP COLUMN IF EXISTS promoted_from_ids,
    DROP COLUMN IF EXISTS mutability;
