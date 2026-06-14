-- Persisted Memory Galaxy 2D layouts. One row per (scope, entity).
-- scope_key identifies the projection scope (workspace[:user][:agent]);
-- fingerprint busts the layout when the scope's memories change.
CREATE TABLE memory_projections (
    scope_key    TEXT        NOT NULL,
    workspace_id UUID        NOT NULL,
    entity_id    UUID        NOT NULL,
    x            REAL        NOT NULL,
    y            REAL        NOT NULL,
    model        TEXT        NOT NULL,
    basis        TEXT        NOT NULL,
    fingerprint  TEXT        NOT NULL,
    computed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (scope_key, entity_id)
);

CREATE INDEX idx_memory_projections_scope ON memory_projections (scope_key);
CREATE INDEX idx_memory_projections_workspace ON memory_projections (workspace_id);
