CREATE TABLE IF NOT EXISTS audit_log (
    id              BIGSERIAL       NOT NULL,
    timestamp       TIMESTAMPTZ     NOT NULL DEFAULT now(),
    event_type      TEXT            NOT NULL,
    session_id      UUID,
    user_id         TEXT,
    workspace       TEXT,
    agent_name      TEXT,
    namespace       TEXT,
    query           TEXT,
    result_count    INTEGER,
    ip_address      INET,
    user_agent      TEXT,
    reason          TEXT,
    metadata        JSONB           DEFAULT '{}',
    PRIMARY KEY (id, timestamp)
) PARTITION BY RANGE (timestamp);

-- Constrain event types
ALTER TABLE audit_log ADD CONSTRAINT audit_log_event_type_check
    CHECK (event_type IN (
        'session_created', 'session_accessed', 'session_searched',
        'session_exported', 'session_deleted', 'pii_redacted',
        'decryption_requested'
    ));

-- Indexes
CREATE INDEX idx_audit_log_session ON audit_log (session_id, timestamp DESC) WHERE session_id IS NOT NULL;
CREATE INDEX idx_audit_log_user ON audit_log (user_id, timestamp DESC) WHERE user_id IS NOT NULL;
CREATE INDEX idx_audit_log_workspace ON audit_log (workspace, timestamp DESC) WHERE workspace IS NOT NULL;
CREATE INDEX idx_audit_log_event_type ON audit_log (event_type, timestamp DESC);
