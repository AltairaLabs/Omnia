-- Revert to original event types only.
ALTER TABLE audit_log DROP CONSTRAINT IF EXISTS audit_log_event_type_check;
ALTER TABLE audit_log ADD CONSTRAINT audit_log_event_type_check CHECK (
    event_type = ANY(ARRAY[
        'session_created', 'session_accessed', 'session_searched',
        'session_exported', 'session_deleted',
        'pii_redacted', 'decryption_requested'
    ])
);
