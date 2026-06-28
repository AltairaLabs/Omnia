-- user_privacy_preferences moved to the per-workspace privacy-api consent DB
-- (#1642). session-api no longer hosts or reads consent/opt-out.
DROP INDEX IF EXISTS idx_privacy_prefs_updated;
DROP TABLE IF EXISTS user_privacy_preferences;
