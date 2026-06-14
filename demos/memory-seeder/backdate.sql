-- demos/memory-seeder/backdate.sql — DEV ONLY.
--
-- The memory-api write path forces observed_at/created_at to now(), so a fresh
-- seed makes the growth chart a single spike and leaves the consolidation worker
-- with no genuinely-stale candidates. Run this AFTER seeding (against omnia_memory)
-- to (a) spread dates over the last 90 days for a believable growth curve, and
-- (b) age the hot-entity observation clusters past the consolidation staleness
-- threshold so the stale-observations axis has real fodder.
--
-- Usage:
--   kubectl port-forward -n omnia-system svc/omnia-postgres 5432:5432
--   PGPASSWORD=omnia psql -h localhost -U omnia -d omnia_memory -f demos/memory-seeder/backdate.sql

-- 1. Spread non-hot entity creation across the last 90 days.
UPDATE memory_entities
SET created_at = now() - (random() * interval '90 days')
WHERE about_kind IS DISTINCT FROM 'support_topic';

-- 2. Spread each non-hot observation between its entity's creation and now.
UPDATE memory_observations o
SET observed_at = e.created_at + (random() * (now() - e.created_at)),
    created_at  = e.created_at
FROM memory_entities e
WHERE o.entity_id = e.id
  AND e.about_kind IS DISTINCT FROM 'support_topic';

-- 3. Age the hot-entity observation clusters (about_key like 'hot-entity-%')
--    well past the staleness threshold so the stale-observations axis fires.
UPDATE memory_observations o
SET observed_at = now() - interval '45 days',
    created_at  = now() - interval '45 days'
FROM memory_entities e
WHERE o.entity_id = e.id
  AND e.about_kind = 'support_topic'
  AND e.about_key LIKE 'hot-entity-%';
