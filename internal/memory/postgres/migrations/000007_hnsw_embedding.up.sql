-- Switch the observation embedding index from IVFFlat to HNSW.
--
-- IVFFlat with lists=100 puts ~10k rows/centroid at 1M observations
-- per workspace; with the probes settings needed for fanout=50
-- recall quality, every cosine query scans 100k+ vectors. HNSW
-- maintains a single navigable graph that gives constant-cost
-- top-K lookups regardless of total row count — the right index
-- for the hot recall path.
--
-- m=16 / ef_construction=64 are the pgvector defaults that balance
-- recall quality against build time. ef_search defaults to 40 and
-- is tuned per-session via SET LOCAL when higher recall is needed.
--
-- The entity-level vector index (memory_entities.embedding) is
-- dropped — nothing reads it; the memory_observations vector is
-- the source of truth for cosine queries.

DROP INDEX IF EXISTS idx_memory_entities_embedding;
DROP INDEX IF EXISTS idx_memory_observations_embedding;

CREATE INDEX idx_memory_observations_embedding
    ON memory_observations USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);
