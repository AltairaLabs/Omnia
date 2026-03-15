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

-- Recreate tool_calls partitions after drop/recreate.
SELECT create_weekly_partitions('tool_calls', (CURRENT_DATE - INTERVAL '28 days')::DATE, (CURRENT_DATE + INTERVAL '14 days')::DATE);

-- Revert manage_session_partitions to exclude provider_calls.
CREATE OR REPLACE FUNCTION manage_session_partitions(
    retention_days  INTEGER DEFAULT 30,
    lookahead_weeks INTEGER DEFAULT 2
) RETURNS TABLE(table_name TEXT, partitions_created INTEGER, partitions_dropped INTEGER) AS $$
DECLARE
    tables TEXT[] := ARRAY['sessions', 'messages', 'tool_calls', 'message_artifacts', 'audit_log'];
    tbl    TEXT;
    created INTEGER;
    dropped INTEGER;
    start_date DATE;
    end_date   DATE;
BEGIN
    start_date := CURRENT_DATE - (retention_days || ' days')::INTERVAL;
    end_date   := CURRENT_DATE + (lookahead_weeks * 7 || ' days')::INTERVAL;

    FOREACH tbl IN ARRAY tables LOOP
        created := create_weekly_partitions(tbl, start_date, end_date);
        dropped := drop_old_partitions(tbl, retention_days);

        table_name := tbl;
        partitions_created := created;
        partitions_dropped := dropped;
        RETURN NEXT;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

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
