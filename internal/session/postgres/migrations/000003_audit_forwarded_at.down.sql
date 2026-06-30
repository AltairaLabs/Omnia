DROP INDEX IF EXISTS idx_audit_log_unforwarded;
ALTER TABLE audit_log DROP COLUMN IF EXISTS forwarded_at;
