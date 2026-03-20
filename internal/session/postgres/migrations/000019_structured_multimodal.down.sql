-- Revert structured multi-modal support.

-- Drop message columns.
ALTER TABLE messages DROP COLUMN IF EXISTS has_media;
ALTER TABLE messages DROP COLUMN IF EXISTS media_types;

-- Drop artifact columns.
ALTER TABLE message_artifacts DROP COLUMN IF EXISTS width;
ALTER TABLE message_artifacts DROP COLUMN IF EXISTS height;
ALTER TABLE message_artifacts DROP COLUMN IF EXISTS duration_ms;
ALTER TABLE message_artifacts DROP COLUMN IF EXISTS channels;
ALTER TABLE message_artifacts DROP COLUMN IF EXISTS sample_rate;
