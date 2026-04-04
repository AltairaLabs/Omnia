-- Enable pgvector extension for embedding storage
CREATE EXTENSION IF NOT EXISTS vector;

-- Entity: a named, typed object in a user's memory graph.
CREATE TABLE memory_entities (
    id              UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID            NOT NULL,
    virtual_user_id TEXT,
    agent_id        UUID,
    name            TEXT            NOT NULL,
    kind            TEXT            NOT NULL,
    source_type     TEXT            NOT NULL DEFAULT 'conversation_extraction',
    trust_model     TEXT            NOT NULL DEFAULT 'inferred',
    metadata        JSONB           NOT NULL DEFAULT '{}',
    embedding       vector(1536),
    purpose         TEXT            NOT NULL DEFAULT 'support_continuity',
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ     NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ,
    forgotten       BOOLEAN         NOT NULL DEFAULT false
);

CREATE INDEX idx_memory_entities_scope ON memory_entities (workspace_id, virtual_user_id, kind);
CREATE INDEX idx_memory_entities_purpose ON memory_entities (workspace_id, virtual_user_id, purpose) WHERE NOT forgotten;
CREATE INDEX idx_memory_entities_embedding ON memory_entities USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

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

-- Observation: a timestamped fact attached to an entity.
CREATE TABLE memory_observations (
    id                  UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id           UUID            NOT NULL REFERENCES memory_entities(id) ON DELETE CASCADE,
    content             TEXT            NOT NULL,
    structured          JSONB,
    confidence          REAL            NOT NULL DEFAULT 1.0,
    sensitivity_cleared BOOLEAN         NOT NULL DEFAULT false,
    embedding           vector(1536),
    source_type         TEXT            NOT NULL DEFAULT 'conversation_extraction',
    session_id          UUID,
    turn_range          INT[],
    extraction_model    TEXT,
    observed_at         TIMESTAMPTZ     NOT NULL DEFAULT now(),
    valid_until         TIMESTAMPTZ,
    superseded_by       UUID            REFERENCES memory_observations(id),
    created_at          TIMESTAMPTZ     NOT NULL DEFAULT now(),
    access_count        INT             NOT NULL DEFAULT 0,
    accessed_at         TIMESTAMPTZ
);

CREATE INDEX idx_memory_observations_entity ON memory_observations (entity_id, observed_at DESC);
CREATE INDEX idx_memory_observations_source ON memory_observations (entity_id, source_type);
CREATE INDEX idx_memory_observations_embedding ON memory_observations USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

-- User privacy preferences with consent grants.
CREATE TABLE user_privacy_preferences (
    user_id         TEXT            PRIMARY KEY,
    opt_out_all     BOOLEAN         DEFAULT FALSE,
    opt_out_workspaces TEXT[]       DEFAULT '{}',
    opt_out_agents  TEXT[]          DEFAULT '{}',
    consent_grants  TEXT[]          DEFAULT '{}',
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_privacy_prefs_updated ON user_privacy_preferences (updated_at);
