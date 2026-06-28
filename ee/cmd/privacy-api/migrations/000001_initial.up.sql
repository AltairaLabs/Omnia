-- privacy-api owns user privacy preferences (opt-out + consent grants) for one
-- workspace/tenant. This schema is the union of the old session and memory
-- user_privacy_preferences tables: opt-out columns plus consent_grants, which
-- only ever existed in the memory DB (so consent writes 500'd on the session
-- DB the handler was actually wired to — issue #1642).
CREATE TABLE user_privacy_preferences (
    user_id            TEXT            PRIMARY KEY,
    opt_out_all        BOOLEAN         DEFAULT FALSE,
    opt_out_workspaces TEXT[]          DEFAULT '{}',
    opt_out_agents     TEXT[]          DEFAULT '{}',
    consent_grants     TEXT[]          DEFAULT '{}',
    created_at         TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_privacy_prefs_updated ON user_privacy_preferences (updated_at);
