-- Add index on sessions(id) to speed up ID-only lookups (e.g., sessionExists).
-- The table is partitioned by RANGE(created_at), so queries that filter only on id
-- must probe every partition. This per-partition index makes each probe O(log N)
-- instead of scanning the partition's primary key index on (id, created_at).
-- Note: CONCURRENTLY is not supported on partitioned tables in PostgreSQL.
-- Postgres will automatically create matching indexes on each partition.
CREATE INDEX IF NOT EXISTS idx_sessions_id ON sessions (id);
