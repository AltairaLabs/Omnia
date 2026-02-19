-- Index for efficient querying of encrypted messages by key ID and version.
-- Used by the re-encryption service to find messages needing re-encryption.
CREATE INDEX IF NOT EXISTS idx_messages_encryption_meta
    ON messages USING GIN ((metadata->'_encryption'))
    WHERE metadata ? '_encryption';
