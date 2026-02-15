CREATE TABLE IF NOT EXISTS eval_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id TEXT NOT NULL,
    message_id TEXT,
    agent_name TEXT NOT NULL,
    namespace TEXT NOT NULL,
    promptpack_name TEXT NOT NULL,
    promptpack_version TEXT,
    eval_id TEXT NOT NULL,
    eval_type TEXT NOT NULL,
    trigger TEXT NOT NULL,
    passed BOOLEAN NOT NULL,
    score DECIMAL(5,4),
    details JSONB,
    duration_ms INT,
    judge_tokens INT,
    judge_cost_usd DECIMAL(10,6),
    source TEXT NOT NULL DEFAULT 'worker',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_eval_results_session_id ON eval_results (session_id);
CREATE INDEX idx_eval_results_agent_namespace ON eval_results (agent_name, namespace);
CREATE INDEX idx_eval_results_eval_id_created ON eval_results (eval_id, created_at);
CREATE INDEX idx_eval_results_created_at ON eval_results (created_at);
