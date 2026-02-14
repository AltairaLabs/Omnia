-- Revert tool_call_id back to UUID.
-- This will fail if any non-UUID values exist in the column.
DROP INDEX IF EXISTS idx_messages_tool_call_id;

ALTER TABLE messages ALTER COLUMN tool_call_id TYPE UUID USING tool_call_id::UUID;

CREATE INDEX idx_messages_tool_call_id ON messages (tool_call_id)
    WHERE tool_call_id IS NOT NULL;
