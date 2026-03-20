-- Drop unused columns from the sessions table.
-- These were created in migration 000001 but never populated by application code.
ALTER TABLE sessions DROP COLUMN IF EXISTS avg_response_time_ms;
ALTER TABLE sessions DROP COLUMN IF EXISTS user_agent;
ALTER TABLE sessions DROP COLUMN IF EXISTS client_ip;
