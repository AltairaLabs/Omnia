-- Structured multi-modal support (Phase 3).
-- Add queryable columns so "sessions with voice input" doesn't require parsing JSON metadata.

-- 1. Add media indicator columns to messages.
ALTER TABLE messages ADD COLUMN IF NOT EXISTS has_media BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS media_types TEXT[] NOT NULL DEFAULT '{}';

-- Index for media queries (partial — only rows with media).
CREATE INDEX idx_messages_media ON messages (session_id, timestamp)
    WHERE has_media = true;

-- 2. Add queryable metadata columns to message_artifacts.
ALTER TABLE message_artifacts ADD COLUMN IF NOT EXISTS width INTEGER;
ALTER TABLE message_artifacts ADD COLUMN IF NOT EXISTS height INTEGER;
ALTER TABLE message_artifacts ADD COLUMN IF NOT EXISTS duration_ms INTEGER;
ALTER TABLE message_artifacts ADD COLUMN IF NOT EXISTS channels INTEGER;
ALTER TABLE message_artifacts ADD COLUMN IF NOT EXISTS sample_rate INTEGER;

-- 3. Backfill has_media and media_types from existing metadata.
-- Messages with metadata->>'multimodal' = 'true' had media parts recorded.
UPDATE messages SET
    has_media = true,
    media_types = COALESCE(
        (SELECT array_agg(DISTINCT elem->>'type')
         FROM jsonb_array_elements(
             CASE WHEN metadata->>'parts' IS NOT NULL
                  THEN (metadata->>'parts')::jsonb
                  ELSE '[]'::jsonb END
         ) AS elem
         WHERE elem->>'type' IS NOT NULL),
        '{}'::TEXT[]
    )
WHERE metadata->>'multimodal' = 'true';
