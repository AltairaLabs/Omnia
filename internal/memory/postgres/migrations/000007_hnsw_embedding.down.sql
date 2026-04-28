-- Restore the IVFFlat indexes that 000001 created.

DROP INDEX IF EXISTS idx_memory_observations_embedding;

CREATE INDEX idx_memory_entities_embedding
    ON memory_entities USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);

CREATE INDEX idx_memory_observations_embedding
    ON memory_observations USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);
