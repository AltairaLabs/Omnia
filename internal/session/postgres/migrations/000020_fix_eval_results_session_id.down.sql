-- Revert eval_results session_id to TEXT.

DROP INDEX IF EXISTS idx_eval_results_session_message;

ALTER TABLE eval_results ALTER COLUMN session_id TYPE TEXT USING session_id::text;

-- Drop and recreate the cascade trigger so it uses the reverted function.
DROP TRIGGER IF EXISTS trg_session_cascade_delete ON sessions;

-- Restore cascade delete with ::text cast.
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

CREATE TRIGGER trg_session_cascade_delete
    BEFORE DELETE ON sessions
    FOR EACH ROW
    EXECUTE FUNCTION cascade_delete_session();
