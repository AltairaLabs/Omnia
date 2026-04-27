-- Stateful memory foundations.
--
-- See docs/superpowers/specs/2026-04-26-stateful-memory-design.md.
-- This migration prepares schema for:
--   - structured-key dedup on writes (about_kind, about_key + unique idx)
--   - large-memory rendering (title, summary, body_size_bytes)
--   - two-layer model: raw vs belief observations (layer enum)
--   - implicit-extraction salience (salience)
--   - embedding model versioning for re-embed worker safety
--
-- All columns nullable / generated / defaulted; existing rows are
-- left untouched and behave the same as today.

ALTER TABLE memory_entities
    ADD COLUMN about_kind  TEXT,
    ADD COLUMN about_key   TEXT,
    ADD COLUMN title       TEXT;

-- One active entity per (scope, about_kind, about_key) when set.
-- "Active" = not forgotten; supersession lives on observations,
-- not entities, so the entity itself is the stable handle the
-- structured-key path supersedes against.
CREATE UNIQUE INDEX idx_memory_entities_about_unique
    ON memory_entities (workspace_id, virtual_user_id, agent_id,
                        about_kind, about_key)
    WHERE about_kind IS NOT NULL AND NOT forgotten;

ALTER TABLE memory_observations
    ADD COLUMN summary          TEXT,
    ADD COLUMN body_size_bytes  INT GENERATED ALWAYS AS
                                 (octet_length(content)) STORED,
    ADD COLUMN layer            TEXT NOT NULL DEFAULT 'belief'
                                 CHECK (layer IN ('raw','belief')),
    ADD COLUMN salience         REAL DEFAULT 0.5,
    ADD COLUMN embedding_model  TEXT;

-- Reflection worker hot path: walk unreflected raw observations
-- per entity, oldest first.
CREATE INDEX idx_memory_observations_unreflected
    ON memory_observations (entity_id, observed_at)
    WHERE layer = 'raw' AND superseded_by IS NULL;

-- Relations walk hot path (Phase 3 episode threading + future
-- multi-hop graph reads).
CREATE INDEX idx_memory_relations_walk
    ON memory_relations (workspace_id, source_entity_id, relation_type);
