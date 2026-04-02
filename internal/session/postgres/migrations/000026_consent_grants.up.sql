ALTER TABLE user_privacy_preferences ADD COLUMN IF NOT EXISTS consent_grants TEXT[] DEFAULT '{}';
