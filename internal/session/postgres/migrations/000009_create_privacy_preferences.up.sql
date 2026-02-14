CREATE TABLE IF NOT EXISTS user_privacy_preferences (
  user_id TEXT PRIMARY KEY,
  opt_out_all BOOLEAN DEFAULT FALSE,
  opt_out_workspaces TEXT[] DEFAULT '{}',
  opt_out_agents TEXT[] DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_privacy_prefs_updated ON user_privacy_preferences(updated_at);
