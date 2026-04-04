-- Add memory and consent event types to the audit_log check constraint.
-- The constraint is on the parent partitioned table and inherited by all partitions.
ALTER TABLE audit_log DROP CONSTRAINT IF EXISTS audit_log_event_type_check;
ALTER TABLE audit_log ADD CONSTRAINT audit_log_event_type_check CHECK (
    event_type = ANY(ARRAY[
        'session_created', 'session_accessed', 'session_searched',
        'session_exported', 'session_deleted',
        'pii_redacted', 'decryption_requested',
        'memory_created', 'memory_accessed', 'memory_deleted', 'memory_exported',
        'consent_granted', 'consent_revoked',
        'deletion_requested', 'deletion_completed', 'deletion_failed'
    ])
);
