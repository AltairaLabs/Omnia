-- Consolidation v1: mutability + lineage columns.
-- mutability gates which rows the consolidation worker may modify.
-- lineage columns retain provenance for forensic walking.

ALTER TABLE memory_observations
    ADD COLUMN mutability             TEXT        NOT NULL DEFAULT 'mutable',
    ADD COLUMN promoted_from_ids      UUID[]      NOT NULL DEFAULT '{}',
    ADD COLUMN promoted_by_pack       TEXT,
    ADD COLUMN promoted_at            TIMESTAMPTZ,
    ADD COLUMN promotion_proposal_id  UUID;

ALTER TABLE memory_entities
    ADD COLUMN mutability             TEXT        NOT NULL DEFAULT 'mutable',
    ADD COLUMN promoted_from_ids      UUID[]      NOT NULL DEFAULT '{}',
    ADD COLUMN promoted_by_pack       TEXT,
    ADD COLUMN promoted_at            TIMESTAMPTZ,
    ADD COLUMN promotion_proposal_id  UUID;

-- Indexes for the consolidation pre-filter exclusion path
-- (WHERE mutability = 'mutable' AND source_type != 'regulated').
CREATE INDEX idx_memory_observations_mutability
    ON memory_observations (entity_id, mutability)
    WHERE mutability = 'mutable';

CREATE INDEX idx_memory_entities_mutability
    ON memory_entities (workspace_id, mutability)
    WHERE mutability = 'mutable';
