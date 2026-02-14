-- Add audit_log to the manage_session_partitions function and create initial partitions.
-- Migration 000006 created the partitioned audit_log table but omitted partition creation.

-- Update manage_session_partitions to include audit_log.
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

-- Create initial audit_log partitions: 4 weeks back + 2 weeks ahead.
SELECT create_weekly_partitions('audit_log', (CURRENT_DATE - INTERVAL '28 days')::DATE, (CURRENT_DATE + INTERVAL '14 days')::DATE);
