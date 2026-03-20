-- Re-add columns dropped in the up migration, using original types from 000001.
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS avg_response_time_ms INTEGER;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS user_agent TEXT;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS client_ip INET;
