-- Recreate the deletion_requests table as it existed in 000001_initial, so the
-- migration is reversible. (privacy-api owns the live DSAR lifecycle; this is only
-- for down-migration parity.)
CREATE TABLE deletion_requests (
    id               TEXT        NOT NULL,
    virtual_user_id  TEXT        NOT NULL,
    reason           TEXT        NOT NULL,
    scope            TEXT        NOT NULL DEFAULT 'all',
    workspace        TEXT,
    date_from        TIMESTAMPTZ,
    date_to          TIMESTAMPTZ,
    status           TEXT        NOT NULL DEFAULT 'pending',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at       TIMESTAMPTZ,
    completed_at     TIMESTAMPTZ,
    sessions_deleted INTEGER     DEFAULT 0,
    errors           JSONB       DEFAULT '[]'::jsonb,
    CONSTRAINT deletion_requests_reason_check CHECK (reason IN ('gdpr_erasure', 'ccpa_delete', 'user_request')),
    CONSTRAINT deletion_requests_scope_check  CHECK (scope IN ('all', 'workspace', 'date_range')),
    CONSTRAINT deletion_requests_status_check CHECK (status IN ('pending', 'in_progress', 'completed', 'failed')),
    PRIMARY KEY (id)
);

CREATE INDEX idx_deletion_requests_user   ON deletion_requests (virtual_user_id);
CREATE INDEX idx_deletion_requests_status ON deletion_requests (status);
