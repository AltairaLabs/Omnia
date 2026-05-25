-- Memory-api audit_log table. The ee/pkg/audit.Logger INSERTs into
-- `audit_log` — session-api creates the table in its own DB via its
-- own migration (000006); memory-api needs the same schema in its
-- own DB so consolidation audit rows can land.
--
-- Differences from session-api's version:
--   - No partitioning (memory-api volume is much lower)
--   - No event_type CHECK constraint (we don't pre-enumerate the
--     memory.consolidation.* action kinds the validator emits)
--   - Index on metadata->>'consolidation_run_id' so operators can
--     pivot from a dashboard row to "the consolidation run that
--     touched this observation"

CREATE TABLE IF NOT EXISTS audit_log (
    id              BIGSERIAL       PRIMARY KEY,
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
    metadata        JSONB           DEFAULT '{}'
);

CREATE INDEX idx_audit_log_event_type ON audit_log (event_type, timestamp DESC);
CREATE INDEX idx_audit_log_workspace ON audit_log (workspace, timestamp DESC) WHERE workspace IS NOT NULL;
CREATE INDEX idx_audit_log_consolidation_run_id
    ON audit_log ((metadata->>'consolidation_run_id'), timestamp DESC)
    WHERE metadata->>'consolidation_run_id' IS NOT NULL;
