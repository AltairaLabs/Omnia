-- Add trigger-based cascade delete for sessions.
-- Partitioned tables cannot use FK ON DELETE CASCADE, so we use a trigger
-- to delete child rows when a session is deleted.
CREATE OR REPLACE FUNCTION cascade_delete_session() RETURNS TRIGGER AS $$
BEGIN
    DELETE FROM message_artifacts WHERE session_id = OLD.id;
    DELETE FROM tool_calls WHERE session_id = OLD.id;
    DELETE FROM eval_results WHERE session_id = OLD.id::text;
    DELETE FROM messages WHERE session_id = OLD.id;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

-- The trigger fires BEFORE DELETE on each row so child rows are removed
-- before the parent row is deleted.
CREATE TRIGGER trg_session_cascade_delete
    BEFORE DELETE ON sessions
    FOR EACH ROW
    EXECUTE FUNCTION cascade_delete_session();
