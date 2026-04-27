-- Full-text search on memory observations.
--
-- Why: the keyword recall path was doing literal ILIKE substring matching
-- ("%my name%" against "User's name is Slim Shard" -> 0 hits). With FTS the
-- query "my name" tokenises to {name} (stopwords dropped) and matches the
-- observation. ts_rank_cd gives the BM25-style scoring we actually want.
--
-- The vector is GENERATED + STORED so writes don't need application-side
-- maintenance and reads can use the GIN index directly. English config is a
-- pragmatic default — multi-locale callers can switch by re-running this
-- migration with a different parser, but every existing memory in the demo
-- workspace is English.
ALTER TABLE memory_observations
    ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('english', coalesce(content, ''))) STORED;

CREATE INDEX idx_memory_observations_search_vector
    ON memory_observations USING GIN (search_vector);
