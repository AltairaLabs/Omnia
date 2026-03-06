-- Add a partial GIN index on the search_vector column to speed up full-text
-- search queries that filter on search_vector IS NOT NULL.
-- Note: CONCURRENTLY is not supported on partitioned tables in PostgreSQL.
-- Postgres will automatically create matching indexes on each partition.
CREATE INDEX IF NOT EXISTS idx_messages_search_vector
    ON messages USING gin(search_vector)
    WHERE search_vector IS NOT NULL;
