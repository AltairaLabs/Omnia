-- Change tool_call_id from UUID to TEXT.
-- LLM providers use non-UUID identifiers for tool calls (e.g., "call_abc123",
-- "toolu_01xyz"), causing INSERT failures when the column is UUID-typed.

-- Drop the partial index first (it references the column type)
DROP INDEX IF EXISTS idx_messages_tool_call_id;

-- Alter column type
ALTER TABLE messages ALTER COLUMN tool_call_id TYPE TEXT USING tool_call_id::TEXT;

-- Recreate the partial index
CREATE INDEX idx_messages_tool_call_id ON messages (tool_call_id)
    WHERE tool_call_id IS NOT NULL;
