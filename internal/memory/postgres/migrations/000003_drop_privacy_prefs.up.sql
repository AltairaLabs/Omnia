-- user_privacy_preferences moved to privacy-api (#1642). Memory-api reads
-- preferences over HTTP now. memory_entities.consent_category is per-memory
-- tagging and is intentionally untouched.
DROP INDEX IF EXISTS idx_privacy_prefs_updated;
DROP TABLE IF EXISTS user_privacy_preferences;
