DROP INDEX IF EXISTS idx_memory_observations_search_vector;
ALTER TABLE memory_observations DROP COLUMN IF EXISTS search_vector;
