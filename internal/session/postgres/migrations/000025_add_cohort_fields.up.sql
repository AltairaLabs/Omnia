ALTER TABLE sessions ADD COLUMN IF NOT EXISTS cohort_id TEXT;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS variant TEXT;
CREATE INDEX IF NOT EXISTS idx_sessions_cohort_id ON sessions (cohort_id) WHERE cohort_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_sessions_variant ON sessions (variant) WHERE variant IS NOT NULL;
