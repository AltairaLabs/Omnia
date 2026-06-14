-- demos/memory-seeder/categorize.sql — DEV ONLY.
--
-- Institutional ingest chunks and agent-tier memories carry no consent_category
-- (the memory-api exposes no category field on those write paths — institutional
-- text is chunked server-side, and /agent-memories has no category field). They
-- are ~85% of all entities, so the category donut and per-memory badges show
-- everything as "unknown". For a populated category UX in the dev demo, spread
-- the uncategorized entities deterministically across the six consent categories.
--
-- Writes BOTH the consent_category column (feeds the GROUP BY category donut via
-- COALESCE(e.consent_category,...)) and metadata.consent_category (feeds the
-- per-memory CategoryBadge in the graph/detail panel).
--
-- Run AFTER seeding (and is order-independent w.r.t. backdate.sql):
--   PGPASSWORD=omnia psql -h localhost -U omnia -d omnia_memory -f demos/memory-seeder/categorize.sql

WITH cats(idx, name) AS (
    VALUES (0, 'memory:identity'), (1, 'memory:context'), (2, 'memory:health'),
           (3, 'memory:location'), (4, 'memory:preferences'), (5, 'memory:history')
),
numbered AS (
    SELECT id, (row_number() OVER (ORDER BY id) % 6) AS idx
    FROM memory_entities
    WHERE consent_category IS NULL AND forgotten = false
)
UPDATE memory_entities e
SET consent_category = c.name,
    metadata = COALESCE(e.metadata, '{}'::jsonb)
               || jsonb_build_object('consent_category', c.name)
FROM numbered n
JOIN cats c ON c.idx = n.idx
WHERE e.id = n.id;
