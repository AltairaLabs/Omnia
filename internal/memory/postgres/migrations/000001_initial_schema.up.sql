-- Initial memory-store schema (collapsed).
--
-- This is the faithful snapshot of what migrations 000001..000012 produced,
-- with two deliberate omissions:
--
--   * memory_entities.embedding and memory_observations.embedding (the
--     pgvector columns) and their indexes are NOT created here. They are
--     application-managed: cmd/memory-api builds them at startup sized to
--     the configured embedding provider's Dimensions() via
--     EnsureEmbeddingSchema (internal/memory/postgres/embedding_schema.go).
--     See issue #1309. The vector extension is still created here because
--     the reconciler needs it.
--
-- Collapsed because the memory store is pre-GA; the previous 000001..000012
-- chain is replaced wholesale. Verified byte-identical (schema-only) against
-- the original chain by hack/verify-memory-migration-collapse.sh.

-- Enable pgvector extension for embedding storage (columns added by reconciler).
CREATE EXTENSION IF NOT EXISTS vector;

-- Entity: a named, typed object in a user's memory graph.
CREATE TABLE memory_entities (
    id                    UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id          UUID            NOT NULL,
    virtual_user_id       TEXT,
    agent_id              UUID,
    name                  TEXT            NOT NULL,
    kind                  TEXT            NOT NULL,
    source_type           TEXT            NOT NULL DEFAULT 'conversation_extraction',
    trust_model           TEXT            NOT NULL DEFAULT 'inferred',
    metadata              JSONB           NOT NULL DEFAULT '{}',
    purpose               TEXT            NOT NULL DEFAULT 'support_continuity',
    created_at            TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ     NOT NULL DEFAULT now(),
    expires_at            TIMESTAMPTZ,
    forgotten             BOOLEAN         NOT NULL DEFAULT false,
    consent_category      TEXT,
    forgotten_at          TIMESTAMPTZ,
    about_kind            TEXT,
    about_key             TEXT,
    title                 TEXT,
    mutability            TEXT            NOT NULL DEFAULT 'mutable',
    promoted_from_ids     UUID[]          NOT NULL DEFAULT '{}',
    promoted_by_pack      TEXT,
    promoted_at           TIMESTAMPTZ,
    promotion_proposal_id UUID
);

CREATE INDEX idx_memory_entities_scope ON memory_entities (workspace_id, virtual_user_id, kind);
CREATE INDEX idx_memory_entities_purpose ON memory_entities (workspace_id, virtual_user_id, purpose) WHERE NOT forgotten;
CREATE INDEX idx_memory_entities_consent_category
    ON memory_entities (workspace_id, virtual_user_id, consent_category)
    WHERE consent_category IS NOT NULL AND forgotten = false;
CREATE INDEX idx_memory_entities_forgotten_at
    ON memory_entities (forgotten_at)
    WHERE forgotten = true AND forgotten_at IS NOT NULL;
CREATE UNIQUE INDEX idx_memory_entities_about_unique
    ON memory_entities (workspace_id, virtual_user_id, agent_id, about_kind, about_key)
    NULLS NOT DISTINCT
    WHERE about_kind IS NOT NULL AND NOT forgotten;
CREATE INDEX idx_memory_entities_mutability
    ON memory_entities (workspace_id, mutability)
    WHERE mutability = 'mutable';

-- Relation: a typed, weighted edge between two entities.
CREATE TABLE memory_relations (
    id                UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id      UUID            NOT NULL,
    source_entity_id  UUID            NOT NULL REFERENCES memory_entities(id) ON DELETE CASCADE,
    target_entity_id  UUID            NOT NULL REFERENCES memory_entities(id) ON DELETE CASCADE,
    relation_type     TEXT            NOT NULL,
    weight            REAL,
    metadata          JSONB           NOT NULL DEFAULT '{}',
    created_at        TIMESTAMPTZ     NOT NULL DEFAULT now(),
    expires_at        TIMESTAMPTZ
);

CREATE INDEX idx_memory_relations_source ON memory_relations (workspace_id, source_entity_id, relation_type);
CREATE INDEX idx_memory_relations_target ON memory_relations (workspace_id, target_entity_id, relation_type);
-- Retained from 000004; identical columns to idx_memory_relations_source.
CREATE INDEX idx_memory_relations_walk ON memory_relations (workspace_id, source_entity_id, relation_type);

-- Observation: a timestamped fact attached to an entity.
CREATE TABLE memory_observations (
    id                    UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id             UUID            NOT NULL REFERENCES memory_entities(id) ON DELETE CASCADE,
    content               TEXT            NOT NULL,
    structured            JSONB,
    confidence            REAL            NOT NULL DEFAULT 1.0,
    sensitivity_cleared   BOOLEAN         NOT NULL DEFAULT false,
    source_type           TEXT            NOT NULL DEFAULT 'conversation_extraction',
    session_id            UUID,
    turn_range            INT[],
    extraction_model      TEXT,
    observed_at           TIMESTAMPTZ     NOT NULL DEFAULT now(),
    valid_until           TIMESTAMPTZ,
    superseded_by         UUID            REFERENCES memory_observations(id),
    created_at            TIMESTAMPTZ     NOT NULL DEFAULT now(),
    access_count          INT             NOT NULL DEFAULT 0,
    accessed_at           TIMESTAMPTZ,
    search_vector         tsvector        GENERATED ALWAYS AS (to_tsvector('english', coalesce(content, ''))) STORED,
    summary               TEXT,
    body_size_bytes       INT             GENERATED ALWAYS AS (octet_length(content)) STORED,
    embedding_model       TEXT,
    mutability            TEXT            NOT NULL DEFAULT 'mutable',
    promoted_from_ids     UUID[]          NOT NULL DEFAULT '{}',
    promoted_by_pack      TEXT,
    promoted_at           TIMESTAMPTZ,
    promotion_proposal_id UUID,
    importance            REAL
);

CREATE INDEX idx_memory_observations_entity ON memory_observations (entity_id, observed_at DESC);
CREATE INDEX idx_memory_observations_source ON memory_observations (entity_id, source_type);
CREATE INDEX idx_memory_observations_search_vector ON memory_observations USING GIN (search_vector);
CREATE INDEX idx_memory_observations_active_observed
    ON memory_observations (entity_id, observed_at)
    WHERE superseded_by IS NULL AND valid_until IS NULL;
CREATE INDEX idx_memory_observations_inactive_observed
    ON memory_observations (entity_id, observed_at)
    WHERE superseded_by IS NOT NULL OR valid_until IS NOT NULL;
CREATE INDEX idx_memory_observations_mutability
    ON memory_observations (entity_id, mutability)
    WHERE mutability = 'mutable';

-- User privacy preferences with consent grants.
CREATE TABLE user_privacy_preferences (
    user_id            TEXT            PRIMARY KEY,
    opt_out_all        BOOLEAN         DEFAULT FALSE,
    opt_out_workspaces TEXT[]          DEFAULT '{}',
    opt_out_agents     TEXT[]          DEFAULT '{}',
    consent_grants     TEXT[]          DEFAULT '{}',
    created_at         TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_privacy_prefs_updated ON user_privacy_preferences (updated_at);

-- Workspace registry for cheap ListWorkspaceIDs, maintained by trigger.
CREATE TABLE memory_workspaces (
    workspace_id  UUID PRIMARY KEY,
    last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO memory_workspaces (workspace_id, last_seen_at)
SELECT DISTINCT workspace_id, now()
FROM memory_entities
WHERE workspace_id IS NOT NULL
ON CONFLICT (workspace_id) DO NOTHING;

CREATE OR REPLACE FUNCTION track_memory_workspace() RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO memory_workspaces (workspace_id, last_seen_at)
    VALUES (NEW.workspace_id, now())
    ON CONFLICT (workspace_id) DO UPDATE SET last_seen_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER memory_entities_track_workspace
    AFTER INSERT ON memory_entities
    FOR EACH ROW
    EXECUTE FUNCTION track_memory_workspace();

-- Audit log (memory-api copy of the session-api audit schema; no partitioning).
CREATE TABLE audit_log (
    id              BIGSERIAL       PRIMARY KEY,
    timestamp       TIMESTAMPTZ     NOT NULL DEFAULT now(),
    event_type      TEXT            NOT NULL,
    session_id      UUID,
    user_id         TEXT,
    workspace       TEXT,
    agent_name      TEXT,
    namespace       TEXT,
    query           TEXT,
    result_count    INTEGER,
    ip_address      INET,
    user_agent      TEXT,
    reason          TEXT,
    metadata        JSONB           DEFAULT '{}'
);

CREATE INDEX idx_audit_log_event_type ON audit_log (event_type, timestamp DESC);
CREATE INDEX idx_audit_log_workspace ON audit_log (workspace, timestamp DESC) WHERE workspace IS NOT NULL;
CREATE INDEX idx_audit_log_consolidation_run_id
    ON audit_log ((metadata->>'consolidation_run_id'), timestamp DESC)
    WHERE metadata->>'consolidation_run_id' IS NOT NULL;

-- Per-(policy, workspace, axis) consolidation cron bookkeeping.
CREATE TABLE consolidation_runs (
    policy_name  TEXT        NOT NULL,
    workspace_id TEXT        NOT NULL,
    axis         TEXT        NOT NULL,
    last_ran_at  TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (policy_name, workspace_id, axis)
);
