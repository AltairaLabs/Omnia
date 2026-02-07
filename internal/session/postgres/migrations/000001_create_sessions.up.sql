-- Sessions table: core session metadata for agent conversations.
-- Partitioned by created_at (weekly) for efficient retention management.
CREATE TABLE IF NOT EXISTS sessions (
    id              UUID            NOT NULL,
    agent_name      TEXT            NOT NULL,
    namespace       TEXT            NOT NULL,
    workspace_name  TEXT,
    status          TEXT            NOT NULL DEFAULT 'active',
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ     NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ,
    ended_at        TIMESTAMPTZ,

    -- Denormalized counters (updated by application layer)
    message_count       INTEGER     NOT NULL DEFAULT 0,
    tool_call_count     INTEGER     NOT NULL DEFAULT 0,
    total_input_tokens  BIGINT      NOT NULL DEFAULT 0,
    total_output_tokens BIGINT      NOT NULL DEFAULT 0,
    estimated_cost_usd  NUMERIC(12,6) NOT NULL DEFAULT 0,
    avg_response_time_ms INTEGER,

    -- Request metadata
    user_agent      TEXT,
    client_ip       INET,
    tags            TEXT[]          DEFAULT '{}',

    -- Flexible state storage
    state           JSONB           NOT NULL DEFAULT '{}',

    -- Preview for list views
    last_message_preview TEXT,

    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Check constraint for status values
ALTER TABLE sessions ADD CONSTRAINT sessions_status_check
    CHECK (status IN ('active', 'completed', 'error', 'expired'));

-- Indexes for common query patterns
CREATE INDEX idx_sessions_agent_created ON sessions (agent_name, created_at DESC);
CREATE INDEX idx_sessions_namespace_created ON sessions (namespace, created_at DESC);
CREATE INDEX idx_sessions_workspace_created ON sessions (workspace_name, created_at DESC)
    WHERE workspace_name IS NOT NULL;
CREATE INDEX idx_sessions_status_active ON sessions (status, created_at DESC)
    WHERE status = 'active';
CREATE INDEX idx_sessions_expires_at ON sessions (expires_at)
    WHERE expires_at IS NOT NULL;
CREATE INDEX idx_sessions_tags ON sessions USING GIN (tags);
