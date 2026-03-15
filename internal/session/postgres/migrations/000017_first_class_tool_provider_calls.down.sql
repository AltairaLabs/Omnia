-- Revert first-class tool_calls and provider_calls.

-- Drop provider_calls entirely.
DROP TABLE IF EXISTS provider_calls CASCADE;

-- Drop and recreate original tool_calls.
DROP TABLE IF EXISTS tool_calls CASCADE;

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

ALTER TABLE tool_calls ADD CONSTRAINT tool_calls_status_check
    CHECK (status IN ('pending', 'success', 'error'));

CREATE INDEX idx_tool_calls_message ON tool_calls (message_id, created_at);
CREATE INDEX idx_tool_calls_session ON tool_calls (session_id, created_at);
CREATE INDEX idx_tool_calls_name ON tool_calls (name, created_at DESC);

-- Revert cascade delete trigger (remove provider_calls).
CREATE OR REPLACE FUNCTION cascade_delete_session() RETURNS TRIGGER AS $$
BEGIN
    DELETE FROM message_artifacts WHERE session_id = OLD.id;
    DELETE FROM tool_calls WHERE session_id = OLD.id;
    DELETE FROM eval_results WHERE session_id = OLD.id::text;
    DELETE FROM messages WHERE session_id = OLD.id;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;
