-- Drop dead schema columns shipped in 000004 but never wired.
--
-- The stateful-memory design (docs/superpowers/specs/2026-04-26-stateful-memory-design.md)
-- went through several drafts; the v1-v3 drafts proposed a two-stage
-- raw / belief layer with a per-row salience score. v4 dropped both
-- in favour of the single-observation model with server-side dedup.
-- Migration 000004 was authored against the v3 draft and shipped
-- the columns regardless.
--
-- No code reads or writes either column. They take up row width on
-- every observation indefinitely; drop them before users start
-- inserting non-default values that would later complicate a drop.

DROP INDEX IF EXISTS idx_memory_observations_unreflected;
ALTER TABLE memory_observations DROP COLUMN IF EXISTS layer;
ALTER TABLE memory_observations DROP COLUMN IF EXISTS salience;
