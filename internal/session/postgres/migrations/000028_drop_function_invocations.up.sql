-- Drop the dedicated function_invocations table and restore the
-- partition-management orchestrator to the pre-migration-26 table list.
-- Functions now record into the `sessions` table family — same model,
-- same tables, same retention rules — distinguished from agent-mode
-- sessions by the `function` tag in sessions.tags.
DROP TABLE IF EXISTS function_invocations CASCADE;

CREATE OR REPLACE FUNCTION manage_session_partitions(
    retention_days  INTEGER DEFAULT 30,
    lookahead_weeks INTEGER DEFAULT 2
) RETURNS TABLE(table_name TEXT, partitions_created INTEGER, partitions_dropped INTEGER) AS $$
DECLARE
    tables TEXT[] := ARRAY[
        'sessions', 'messages', 'tool_calls', 'provider_calls',
        'runtime_events', 'message_artifacts', 'audit_log'
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
