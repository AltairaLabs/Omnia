CREATE TABLE IF NOT EXISTS deletion_requests (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  reason TEXT NOT NULL CHECK (reason IN ('gdpr_erasure', 'ccpa_delete', 'user_request')),
  scope TEXT NOT NULL DEFAULT 'all' CHECK (scope IN ('all', 'workspace', 'date_range')),
  workspace TEXT,
  date_from TIMESTAMPTZ,
  date_to TIMESTAMPTZ,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'in_progress', 'completed', 'failed')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  sessions_deleted INT DEFAULT 0,
  errors JSONB DEFAULT '[]'
);
CREATE INDEX idx_deletion_requests_user ON deletion_requests(user_id);
CREATE INDEX idx_deletion_requests_status ON deletion_requests(status);
