-- Re-add the execution column with a default of 'server'.
ALTER TABLE tool_calls ADD COLUMN IF NOT EXISTS execution TEXT NOT NULL DEFAULT 'server';
ALTER TABLE tool_calls ADD CONSTRAINT tool_calls_execution_check
    CHECK (execution IN ('server', 'client'));
