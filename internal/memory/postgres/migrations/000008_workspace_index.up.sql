-- Workspace registry table for cheap ListWorkspaceIDs.
--
-- The original ListWorkspaceIDs ran `SELECT DISTINCT workspace_id
-- FROM memory_entities`. With 1M entities and no covering index
-- on workspace_id alone, that's a seq-scan + hash-aggregate every
-- tombstone / compaction / retention worker tick. Cost grows linearly
-- with entity count.
--
-- memory_workspaces holds one row per workspace touched by a write.
-- Maintained by an INSERT … ON CONFLICT DO UPDATE trigger on
-- memory_entities so callers don't need to remember to populate it.
-- ListWorkspaceIDs becomes `SELECT workspace_id FROM memory_workspaces`
-- — instant regardless of entity count.

CREATE TABLE memory_workspaces (
    workspace_id  UUID PRIMARY KEY,
    last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Backfill from existing memory_entities so workspaces created
-- before this migration also surface to the workers.
INSERT INTO memory_workspaces (workspace_id, last_seen_at)
SELECT DISTINCT workspace_id, now()
FROM memory_entities
WHERE workspace_id IS NOT NULL
ON CONFLICT (workspace_id) DO NOTHING;

-- Trigger maintains the registry on every entity insert. UPDATE on
-- conflict so last_seen_at advances — handy for "workspaces touched
-- in the last X" diagnostics later.
CREATE OR REPLACE FUNCTION track_memory_workspace() RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO memory_workspaces (workspace_id, last_seen_at)
    VALUES (NEW.workspace_id, now())
    ON CONFLICT (workspace_id) DO UPDATE SET last_seen_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER memory_entities_track_workspace
    AFTER INSERT ON memory_entities
    FOR EACH ROW
    EXECUTE FUNCTION track_memory_workspace();

-- Compaction discovery + recall scoring both filter by
--   superseded_by IS NULL AND valid_until IS NULL  (the "still
-- live" predicate) AND observed_at in a workspace-scoped JOIN.
-- A targeted partial index on the immutable side of the active
-- predicate keeps these from full-scanning observations. Postgres
-- partial-index predicates must be IMMUTABLE so we can't embed
-- `valid_until > now()` here — the runtime SQL still filters with
-- the time check, the index just narrows the candidate set.
CREATE INDEX IF NOT EXISTS idx_memory_observations_active_observed
    ON memory_observations (entity_id, observed_at)
    WHERE superseded_by IS NULL AND valid_until IS NULL;

-- Mirror partial for tombstone GC: it walks observations marked
-- inactive (superseded or expired). Same IMMUTABLE constraint —
-- runtime filters add the time bound.
CREATE INDEX IF NOT EXISTS idx_memory_observations_inactive_observed
    ON memory_observations (entity_id, observed_at)
    WHERE superseded_by IS NOT NULL OR valid_until IS NOT NULL;
