DROP INDEX IF EXISTS idx_sessions_variant;
DROP INDEX IF EXISTS idx_sessions_cohort_id;
ALTER TABLE sessions DROP COLUMN IF EXISTS variant;
ALTER TABLE sessions DROP COLUMN IF EXISTS cohort_id;
