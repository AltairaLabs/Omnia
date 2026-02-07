-- Partition management functions for weekly partitions across all session tables.
-- Partitions are aligned to ISO week boundaries (Monday 00:00 UTC).

-- create_weekly_partitions: creates weekly partitions for a given table
-- between start_date and end_date.
CREATE OR REPLACE FUNCTION create_weekly_partitions(
    target_table TEXT,
    start_date   DATE,
    end_date     DATE
) RETURNS INTEGER AS $$
DECLARE
    week_start    DATE;
    week_end      DATE;
    partition_name TEXT;
    created_count  INTEGER := 0;
BEGIN
    -- Align start_date to Monday of its ISO week
    week_start := start_date - ((EXTRACT(ISODOW FROM start_date)::INTEGER - 1) || ' days')::INTERVAL;

    WHILE week_start < end_date LOOP
        week_end := week_start + INTERVAL '7 days';
        partition_name := target_table || '_w' || to_char(week_start, 'IYYY_IW');

        -- Only create if partition doesn't already exist
        IF NOT EXISTS (
            SELECT 1 FROM pg_class c
            JOIN pg_namespace n ON n.oid = c.relnamespace
            WHERE c.relname = partition_name
            AND n.nspname = current_schema()
        ) THEN
            EXECUTE format(
                'CREATE TABLE %I PARTITION OF %I FOR VALUES FROM (%L) TO (%L)',
                partition_name, target_table, week_start, week_end
            );
            created_count := created_count + 1;
        END IF;

        week_start := week_end;
    END LOOP;

    RETURN created_count;
END;
$$ LANGUAGE plpgsql;

-- drop_old_partitions: drops partitions older than retention_days.
CREATE OR REPLACE FUNCTION drop_old_partitions(
    target_table   TEXT,
    retention_days INTEGER
) RETURNS INTEGER AS $$
DECLARE
    cutoff_date   DATE;
    rec           RECORD;
    dropped_count INTEGER := 0;
BEGIN
    cutoff_date := CURRENT_DATE - (retention_days || ' days')::INTERVAL;

    FOR rec IN
        SELECT c.relname AS partition_name,
               pg_get_expr(c.relpartbound, c.oid) AS bound_expr
        FROM pg_class c
        JOIN pg_inherits i ON i.inhrelid = c.oid
        JOIN pg_class parent ON parent.oid = i.inhparent
        JOIN pg_namespace n ON n.oid = parent.relnamespace
        WHERE parent.relname = target_table
        AND n.nspname = current_schema()
        AND c.relispartition
    LOOP
        -- Extract the upper bound date from the partition range expression.
        -- Partition bound format: FOR VALUES FROM ('YYYY-MM-DD') TO ('YYYY-MM-DD')
        DECLARE
            upper_bound DATE;
        BEGIN
            SELECT (regexp_match(rec.bound_expr, 'TO \(''([^'']+)''\)'))[1]::DATE
            INTO upper_bound;

            IF upper_bound IS NOT NULL AND upper_bound <= cutoff_date THEN
                EXECUTE format('DROP TABLE %I', rec.partition_name);
                dropped_count := dropped_count + 1;
            END IF;
        EXCEPTION
            WHEN OTHERS THEN
                -- Skip partitions whose bounds can't be parsed
                NULL;
        END;
    END LOOP;

    RETURN dropped_count;
END;
$$ LANGUAGE plpgsql;

-- manage_session_partitions: orchestrates partition creation and cleanup
-- for all session history tables.
CREATE OR REPLACE FUNCTION manage_session_partitions(
    retention_days  INTEGER DEFAULT 30,
    lookahead_weeks INTEGER DEFAULT 2
) RETURNS TABLE(table_name TEXT, partitions_created INTEGER, partitions_dropped INTEGER) AS $$
DECLARE
    tables TEXT[] := ARRAY['sessions', 'messages', 'tool_calls', 'message_artifacts'];
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

-- Create initial partitions: 4 weeks back + 2 weeks ahead.
SELECT manage_session_partitions(28, 2);
