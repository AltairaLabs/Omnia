-- Message artifacts table: binary/media attachments associated with messages.
-- Partitioned by created_at (weekly) aligned with other tables.
CREATE TABLE IF NOT EXISTS message_artifacts (
    id              UUID            NOT NULL DEFAULT gen_random_uuid(),
    message_id      UUID            NOT NULL,
    session_id      UUID            NOT NULL,
    artifact_type   TEXT            NOT NULL,
    mime_type       TEXT            NOT NULL,
    storage_uri     TEXT            NOT NULL,
    size_bytes      BIGINT,
    filename        TEXT,
    checksum_sha256 TEXT,
    metadata        JSONB           NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT now(),

    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Check constraint for artifact_type values
ALTER TABLE message_artifacts ADD CONSTRAINT message_artifacts_type_check
    CHECK (artifact_type IN ('image', 'audio', 'video', 'document', 'file'));

-- Indexes for common query patterns
CREATE INDEX idx_message_artifacts_message ON message_artifacts (message_id);
CREATE INDEX idx_message_artifacts_session ON message_artifacts (session_id, created_at);
