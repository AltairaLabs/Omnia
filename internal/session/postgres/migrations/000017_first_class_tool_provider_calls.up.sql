-- First-class tool_calls and provider_calls tables.
-- Phase 1: drop legacy tool_calls, recreate with new columns, add provider_calls.

-- 1. Drop the existing tool_calls table (never written to, safe to recreate).
DROP TABLE IF EXISTS tool_calls CASCADE;

-- 2. Recreate tool_calls with new columns.
CREATE TABLE tool_calls (
    id              UUID            NOT NULL,
    session_id      UUID            NOT NULL,
    call_id         TEXT            NOT NULL DEFAULT '',
    name            TEXT            NOT NULL,
    arguments       JSONB           NOT NULL DEFAULT '{}',
    result          JSONB,
    status          TEXT            NOT NULL DEFAULT 'pending',
    duration_ms     BIGINT,
    execution       TEXT            NOT NULL DEFAULT 'server',
    error_message   TEXT,
    labels          JSONB           NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT now(),

    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

ALTER TABLE tool_calls ADD CONSTRAINT tool_calls_status_check
    CHECK (status IN ('pending', 'success', 'error'));

ALTER TABLE tool_calls ADD CONSTRAINT tool_calls_execution_check
    CHECK (execution IN ('server', 'client'));

CREATE INDEX idx_tool_calls_session ON tool_calls (session_id, created_at);
CREATE INDEX idx_tool_calls_name ON tool_calls (name, created_at DESC);
CREATE INDEX idx_tool_calls_call_id ON tool_calls (call_id, created_at) WHERE call_id != '';

-- 3. Create provider_calls table (partitioned by created_at, same as tool_calls).
CREATE TABLE provider_calls (
    id              UUID            NOT NULL,
    session_id      UUID            NOT NULL,
    provider        TEXT            NOT NULL,
    model           TEXT            NOT NULL DEFAULT '',
    status          TEXT            NOT NULL DEFAULT 'pending',
    input_tokens    BIGINT          NOT NULL DEFAULT 0,
    output_tokens   BIGINT          NOT NULL DEFAULT 0,
    cached_tokens   BIGINT          NOT NULL DEFAULT 0,
    cost_usd        DOUBLE PRECISION NOT NULL DEFAULT 0,
    duration_ms     BIGINT,
    finish_reason   TEXT,
    tool_call_count INTEGER         NOT NULL DEFAULT 0,
    error_message   TEXT,
    labels          JSONB           NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT now(),

    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

ALTER TABLE provider_calls ADD CONSTRAINT provider_calls_status_check
    CHECK (status IN ('pending', 'completed', 'failed'));

CREATE INDEX idx_provider_calls_session ON provider_calls (session_id, created_at);
CREATE INDEX idx_provider_calls_provider ON provider_calls (provider, created_at DESC);

-- 4. Update manage_session_partitions to include provider_calls.
CREATE OR REPLACE FUNCTION manage_session_partitions(
    retention_days  INTEGER DEFAULT 30,
    lookahead_weeks INTEGER DEFAULT 2
) RETURNS TABLE(table_name TEXT, partitions_created INTEGER, partitions_dropped INTEGER) AS $$
DECLARE
    tables TEXT[] := ARRAY['sessions', 'messages', 'tool_calls', 'provider_calls', 'message_artifacts', 'audit_log'];
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

-- Create initial partitions for tool_calls and provider_calls.
SELECT create_weekly_partitions('tool_calls', (CURRENT_DATE - INTERVAL '28 days')::DATE, (CURRENT_DATE + INTERVAL '14 days')::DATE);
SELECT create_weekly_partitions('provider_calls', (CURRENT_DATE - INTERVAL '28 days')::DATE, (CURRENT_DATE + INTERVAL '14 days')::DATE);

-- 5. Update cascade delete trigger to include provider_calls.
CREATE OR REPLACE FUNCTION cascade_delete_session() RETURNS TRIGGER AS $$
BEGIN
    DELETE FROM message_artifacts WHERE session_id = OLD.id;
    DELETE FROM tool_calls WHERE session_id = OLD.id;
    DELETE FROM provider_calls WHERE session_id = OLD.id;
    DELETE FROM eval_results WHERE session_id = OLD.id::text;
    DELETE FROM messages WHERE session_id = OLD.id;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;
