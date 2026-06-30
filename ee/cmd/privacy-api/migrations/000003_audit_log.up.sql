-- Central privacy/compliance audit log (#1673). privacy-api is the hub: memory-api
-- and session-api forward their enforcement audit events here (durably, via a
-- drain-forwarder over each service's own local audit_log). Columns are stored
-- denormalized as TEXT because this is a read model aggregated from multiple
-- sources — avoiding UUID/INET casts keeps ingest from rejecting heterogeneous
-- inputs. (source_service, source_id) is the idempotency key for at-least-once
-- forwarding: re-delivering the same source row is a no-op.
CREATE TABLE audit_log (
    id             BIGSERIAL    PRIMARY KEY,
    source_service TEXT         NOT NULL,
    source_id      BIGINT       NOT NULL,
    "timestamp"    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    event_type     TEXT         NOT NULL,
    session_id     TEXT,
    user_id        TEXT,
    workspace      TEXT,
    agent_name     TEXT,
    namespace      TEXT,
    query          TEXT,
    result_count   INTEGER,
    ip_address     TEXT,
    user_agent     TEXT,
    reason         TEXT,
    metadata       JSONB        NOT NULL DEFAULT '{}',
    UNIQUE (source_service, source_id)
);

-- Covers the enforcement-stats aggregate: WHERE workspace = $1 AND event_type = ANY(...).
CREATE INDEX idx_privacy_audit_workspace_event
    ON audit_log (workspace, event_type, "timestamp" DESC);
