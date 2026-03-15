-- Create runtime_events table for PromptKit lifecycle events.
-- These were previously stored as system messages, polluting the messages table.

CREATE TABLE runtime_events (
    id              UUID            NOT NULL,
    session_id      UUID            NOT NULL,
    event_type      TEXT            NOT NULL,
    data            JSONB           NOT NULL DEFAULT '{}',
    duration_ms     BIGINT,
    error_message   TEXT,
    timestamp       TIMESTAMPTZ     NOT NULL DEFAULT now(),

    PRIMARY KEY (id, timestamp)
) PARTITION BY RANGE (timestamp);

CREATE INDEX idx_runtime_events_session ON runtime_events (session_id, timestamp);
CREATE INDEX idx_runtime_events_type ON runtime_events (event_type, timestamp DESC);

-- Update manage_session_partitions to include runtime_events.
CREATE OR REPLACE FUNCTION manage_session_partitions(
    retention_days  INTEGER DEFAULT 30,
    lookahead_weeks INTEGER DEFAULT 2
) RETURNS TABLE(table_name TEXT, partitions_created INTEGER, partitions_dropped INTEGER) AS $$
DECLARE
    tables TEXT[] := ARRAY['sessions', 'messages', 'tool_calls', 'provider_calls', 'runtime_events', 'message_artifacts', 'audit_log'];
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

-- Create initial partitions for runtime_events.
SELECT create_weekly_partitions('runtime_events', (CURRENT_DATE - INTERVAL '28 days')::DATE, (CURRENT_DATE + INTERVAL '14 days')::DATE);

-- Update cascade delete trigger to include runtime_events.
CREATE OR REPLACE FUNCTION cascade_delete_session() RETURNS TRIGGER AS $$
BEGIN
    DELETE FROM message_artifacts WHERE session_id = OLD.id;
    DELETE FROM tool_calls WHERE session_id = OLD.id;
    DELETE FROM provider_calls WHERE session_id = OLD.id;
    DELETE FROM runtime_events WHERE session_id = OLD.id;
    DELETE FROM eval_results WHERE session_id = OLD.id::text;
    DELETE FROM messages WHERE session_id = OLD.id;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;
