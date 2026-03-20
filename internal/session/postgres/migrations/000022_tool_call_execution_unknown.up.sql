-- Remove the execution column — it was hardcoded and did not accurately
-- represent whether a tool ran server-side or was delegated to the client.
ALTER TABLE tool_calls DROP COLUMN IF EXISTS execution;
