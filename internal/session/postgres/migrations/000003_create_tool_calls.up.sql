-- Tool calls table: individual tool invocations from messages.
-- Separate from messages to enable per-tool-call queries and aggregation.
-- Partitioned by created_at (weekly) aligned with sessions/messages.
CREATE TABLE IF NOT EXISTS tool_calls (
    id              UUID            NOT NULL,
    message_id      UUID            NOT NULL,
    session_id      UUID            NOT NULL,
    name            TEXT            NOT NULL,
    arguments       JSONB           NOT NULL DEFAULT '{}',
    result          JSONB,
    status          TEXT            NOT NULL DEFAULT 'pending',
    duration_ms     INTEGER,
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT now(),

    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Check constraint for status values
ALTER TABLE tool_calls ADD CONSTRAINT tool_calls_status_check
    CHECK (status IN ('pending', 'success', 'error'));

-- Indexes for common query patterns
CREATE INDEX idx_tool_calls_message ON tool_calls (message_id, created_at);
CREATE INDEX idx_tool_calls_session ON tool_calls (session_id, created_at);
CREATE INDEX idx_tool_calls_name ON tool_calls (name, created_at DESC);
