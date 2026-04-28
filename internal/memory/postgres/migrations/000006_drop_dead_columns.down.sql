-- Restore the dead columns dropped by 000006.up.sql so a downgrade
-- doesn't fail with "column already dropped" if the schema is
-- re-applied on top. Values default to whatever 000004 specified.

ALTER TABLE memory_observations
    ADD COLUMN IF NOT EXISTS salience REAL DEFAULT 0.5,
    ADD COLUMN IF NOT EXISTS layer    TEXT NOT NULL DEFAULT 'belief'
                                       CHECK (layer IN ('raw','belief'));

CREATE INDEX IF NOT EXISTS idx_memory_observations_unreflected
    ON memory_observations (entity_id, observed_at)
    WHERE layer = 'raw' AND superseded_by IS NULL;
