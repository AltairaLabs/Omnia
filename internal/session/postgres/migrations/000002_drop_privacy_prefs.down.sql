CREATE TABLE user_privacy_preferences (
    user_id            TEXT        NOT NULL,
    opt_out_all        BOOLEAN     DEFAULT false,
    opt_out_workspaces TEXT[]      DEFAULT '{}'::text[],
    opt_out_agents     TEXT[]      DEFAULT '{}'::text[],
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id)
);
CREATE INDEX idx_privacy_prefs_updated ON user_privacy_preferences (updated_at);
