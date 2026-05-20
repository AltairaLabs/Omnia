-- Roll back: re-create the function_invocations table + index + re-add
-- the table to the orchestrator. Mirrors migrations 26 + 27 minus the
-- function-specific Go code which now lives nowhere; recreating the
-- table here is for migration symmetry rather than usability.

CREATE TABLE IF NOT EXISTS function_invocations (
    id              UUID            NOT NULL,
    namespace       TEXT            NOT NULL,
    function_name   TEXT            NOT NULL,
    input_hash      TEXT            NOT NULL,
    output_json     JSONB,
    status          TEXT            NOT NULL,
    duration_ms     INTEGER         NOT NULL DEFAULT 0,
    cost_usd        NUMERIC(12,6)   NOT NULL DEFAULT 0,
    trace_id        TEXT,
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT now(),

    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

ALTER TABLE function_invocations ADD CONSTRAINT function_invocations_status_check
    CHECK (status IN ('success', 'input_invalid', 'output_invalid', 'runtime_error'));

CREATE INDEX idx_function_invocations_function_created
    ON function_invocations (namespace, function_name, created_at DESC);
CREATE INDEX idx_function_invocations_namespace_created
    ON function_invocations (namespace, created_at DESC);
CREATE INDEX idx_function_invocations_trace_id
    ON function_invocations (trace_id)
    WHERE trace_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_function_invocations_namespace_id
    ON function_invocations (namespace, id);

CREATE OR REPLACE FUNCTION manage_session_partitions(
    retention_days  INTEGER DEFAULT 30,
    lookahead_weeks INTEGER DEFAULT 2
) RETURNS TABLE(table_name TEXT, partitions_created INTEGER, partitions_dropped INTEGER) AS $$
DECLARE
    tables TEXT[] := ARRAY[
        'sessions', 'messages', 'tool_calls', 'provider_calls',
        'runtime_events', 'message_artifacts', 'audit_log',
        'function_invocations'
    ];
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

SELECT create_weekly_partitions(
    'function_invocations',
    (CURRENT_DATE - INTERVAL '28 days')::DATE,
    (CURRENT_DATE + INTERVAL '14 days')::DATE
);
