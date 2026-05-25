-- Add importance column to memory_observations for the Rescore
-- consolidation action. The v1 RescoreWrite carried Importance from
-- the pack but the Postgres writer dropped it ("placeholder for
-- future expansion" per the comment). This migration adds the column;
-- the writer is updated to persist it.
ALTER TABLE memory_observations
    ADD COLUMN importance REAL;
