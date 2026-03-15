-- Fix eval_results: change session_id from TEXT to UUID, add composite index.

-- 1. Convert session_id from TEXT to UUID.
ALTER TABLE eval_results ALTER COLUMN session_id TYPE UUID USING session_id::uuid;

-- 2. Add composite index for session+message lookups.
CREATE INDEX IF NOT EXISTS idx_eval_results_session_message
    ON eval_results (session_id, message_id);

-- 3. Update cascade delete trigger — no longer needs ::text cast.
CREATE OR REPLACE FUNCTION cascade_delete_session() RETURNS TRIGGER AS $$
BEGIN
    DELETE FROM message_artifacts WHERE session_id = OLD.id;
    DELETE FROM tool_calls WHERE session_id = OLD.id;
    DELETE FROM provider_calls WHERE session_id = OLD.id;
    DELETE FROM runtime_events WHERE session_id = OLD.id;
    DELETE FROM eval_results WHERE session_id = OLD.id;
    DELETE FROM messages WHERE session_id = OLD.id;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;
