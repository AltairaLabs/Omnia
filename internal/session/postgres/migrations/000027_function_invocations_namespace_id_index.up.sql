-- Add a (namespace, id) index on function_invocations so single-row Get
-- by (namespace, id) doesn't fan out across every partition. The primary
-- key is (id, created_at) — without created_at in the WHERE clause the
-- planner can't prune partitions, so an unindexed Get reads every weekly
-- partition the table has. The dashboard drill-down view (PR 6) hits
-- this path; this index keeps it constant-time as the table grows.
CREATE INDEX IF NOT EXISTS idx_function_invocations_namespace_id
    ON function_invocations (namespace, id);
