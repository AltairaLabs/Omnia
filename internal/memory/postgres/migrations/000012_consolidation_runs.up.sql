-- Tracks the last-run timestamp per (policy, workspace, axis) so the
-- consolidation worker can honour per-axis cron schedules across restarts.
CREATE TABLE IF NOT EXISTS consolidation_runs (
    policy_name  TEXT        NOT NULL,
    workspace_id TEXT        NOT NULL, -- workspace UID; TEXT because the
                                       -- legacy worker fallback keys on a
                                       -- non-UUID string
    axis         TEXT        NOT NULL,
    last_ran_at  TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (policy_name, workspace_id, axis)
);
