-- Consolidated initial schema for the session-api store.
--
-- This single migration replaces the historical 000001-000028 chain. The
-- product is pre-GA; there is no production data to preserve, so the old
-- incremental migrations (and any DB that ran them) are dropped rather than
-- upgraded in place. Fresh installs only.
--
-- Baked-in revisions over the historical schema (see issue #1301 + schema review):
--   * provider_calls: denormalized namespace/agent_name (+ provider_name for CRD
--     attribution); namespace-leading index replaces the provider-leading one.
--   * provider_usage: NEW workspace-scoped (session-less) usage table for
--     infrastructure spend (embeddings, ingestion, consolidation).
--   * eval_results: now RANGE-partitioned by created_at + in the retention set;
--     judge_tokens/judge_cost_usd removed (judge spend is recorded in
--     provider_calls, source='judge'); namespace-leading index.
--   * messages: dropped the duplicate full GIN index (kept the partial one).

-- ---------------------------------------------------------------------------
-- Tables
-- ---------------------------------------------------------------------------

-- sessions: core session metadata (partitioned weekly by created_at).
CREATE TABLE sessions (
    id                   UUID            NOT NULL,
    agent_name           TEXT            NOT NULL,
    namespace            TEXT            NOT NULL,
    workspace_name       TEXT,
    status               TEXT            NOT NULL DEFAULT 'active',
    created_at           TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ     NOT NULL DEFAULT now(),
    expires_at           TIMESTAMPTZ,
    ended_at             TIMESTAMPTZ,
    message_count        INTEGER         NOT NULL DEFAULT 0,
    tool_call_count      INTEGER         NOT NULL DEFAULT 0,
    total_input_tokens   BIGINT          NOT NULL DEFAULT 0,
    total_output_tokens  BIGINT          NOT NULL DEFAULT 0,
    estimated_cost_usd   NUMERIC(12,6)   NOT NULL DEFAULT 0,
    tags                 TEXT[]          DEFAULT '{}'::text[],
    state                JSONB           NOT NULL DEFAULT '{}',
    last_message_preview TEXT,
    prompt_pack_name     TEXT,
    prompt_pack_version  TEXT,
    cohort_id            TEXT,
    variant              TEXT,
    virtual_user_id      TEXT            NOT NULL,
    CONSTRAINT sessions_status_check CHECK (status IN ('active', 'completed', 'error', 'expired')),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE INDEX idx_sessions_agent_created     ON sessions (agent_name, created_at DESC);
CREATE INDEX idx_sessions_namespace_created ON sessions (namespace, created_at DESC);
CREATE INDEX idx_sessions_workspace_created ON sessions (workspace_name, created_at DESC) WHERE workspace_name IS NOT NULL;
CREATE INDEX idx_sessions_status_active     ON sessions (status, created_at DESC) WHERE status = 'active';
CREATE INDEX idx_sessions_expires_at        ON sessions (expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX idx_sessions_id                ON sessions (id);
CREATE INDEX idx_sessions_cohort_id         ON sessions (cohort_id) WHERE cohort_id IS NOT NULL;
CREATE INDEX idx_sessions_variant           ON sessions (variant) WHERE variant IS NOT NULL;
CREATE INDEX idx_sessions_virtual_user      ON sessions (virtual_user_id, created_at DESC);
CREATE INDEX idx_sessions_tags              ON sessions USING GIN (tags);

-- messages: conversation messages (partitioned weekly by timestamp).
CREATE TABLE messages (
    id            UUID        NOT NULL,
    session_id    UUID        NOT NULL,
    role          TEXT        NOT NULL,
    content       TEXT        NOT NULL DEFAULT '',
    "timestamp"   TIMESTAMPTZ NOT NULL DEFAULT now(),
    input_tokens  INTEGER,
    output_tokens INTEGER,
    tool_call_id  TEXT,
    metadata      JSONB       NOT NULL DEFAULT '{}',
    sequence_num  INTEGER     NOT NULL DEFAULT 0,
    search_vector TSVECTOR    GENERATED ALWAYS AS (to_tsvector('english'::regconfig, content)) STORED,
    has_media     BOOLEAN     NOT NULL DEFAULT false,
    media_types   TEXT[]      NOT NULL DEFAULT '{}'::text[],
    cost_usd      NUMERIC(12,6) NOT NULL DEFAULT 0,
    CONSTRAINT messages_role_check CHECK (role IN ('user', 'assistant', 'system', 'tool')),
    PRIMARY KEY (id, "timestamp")
) PARTITION BY RANGE ("timestamp");

CREATE INDEX idx_messages_session_seq     ON messages (session_id, sequence_num);
-- Single partial GIN on the generated tsvector. (The historical schema also had
-- a non-partial duplicate idx_messages_search — dropped here; GIN never indexes
-- NULLs, so the partial predicate covers the same rows for less write cost.)
CREATE INDEX idx_messages_search_vector   ON messages USING GIN (search_vector) WHERE search_vector IS NOT NULL;
CREATE INDEX idx_messages_tool_call_id    ON messages (tool_call_id) WHERE tool_call_id IS NOT NULL;
CREATE INDEX idx_messages_encryption_meta ON messages USING GIN ((metadata -> '_encryption')) WHERE metadata ? '_encryption';
CREATE INDEX idx_messages_media           ON messages (session_id, "timestamp") WHERE has_media = true;

-- tool_calls: first-class tool invocations (partitioned weekly by created_at).
CREATE TABLE tool_calls (
    id            UUID        NOT NULL,
    session_id    UUID        NOT NULL,
    call_id       TEXT        NOT NULL DEFAULT '',
    name          TEXT        NOT NULL,
    arguments     JSONB       NOT NULL DEFAULT '{}',
    result        JSONB,
    status        TEXT        NOT NULL DEFAULT 'pending',
    duration_ms   BIGINT,
    error_message TEXT,
    labels        JSONB       NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT tool_calls_status_check CHECK (status IN ('pending', 'success', 'error')),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE INDEX idx_tool_calls_session ON tool_calls (session_id, created_at);
CREATE INDEX idx_tool_calls_name    ON tool_calls (name, created_at DESC);
CREATE INDEX idx_tool_calls_call_id ON tool_calls (call_id, created_at) WHERE call_id <> '';

-- provider_calls: per-session LLM provider calls (partitioned weekly by created_at).
-- namespace + agent_name are denormalized from sessions (immutable per session)
-- so the dashboard aggregate filters without a JOIN. provider_name carries the
-- Provider CRD identity for per-provider attribution (provider holds the type).
CREATE TABLE provider_calls (
    id              UUID             NOT NULL,
    session_id      UUID             NOT NULL,
    namespace       TEXT             NOT NULL DEFAULT '',
    agent_name      TEXT             NOT NULL DEFAULT '',
    provider        TEXT             NOT NULL,
    provider_name   TEXT,
    model           TEXT             NOT NULL DEFAULT '',
    status          TEXT             NOT NULL DEFAULT 'pending',
    input_tokens    BIGINT           NOT NULL DEFAULT 0,
    output_tokens   BIGINT           NOT NULL DEFAULT 0,
    cached_tokens   BIGINT           NOT NULL DEFAULT 0,
    cost_usd        DOUBLE PRECISION NOT NULL DEFAULT 0,
    duration_ms     BIGINT,
    finish_reason   TEXT,
    tool_call_count INTEGER          NOT NULL DEFAULT 0,
    error_message   TEXT,
    labels          JSONB            NOT NULL DEFAULT '{}',
    source          TEXT             NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ      NOT NULL DEFAULT now(),
    CONSTRAINT provider_calls_status_check CHECK (status IN ('pending', 'completed', 'failed')),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE INDEX idx_provider_calls_session          ON provider_calls (session_id, created_at);
CREATE INDEX idx_provider_calls_namespace_created ON provider_calls (namespace, created_at DESC);

-- provider_usage: workspace-scoped (session-less) provider spend that does NOT
-- belong to a single conversation — query/ingest/re-embed embeddings, judge
-- calls, consolidation. Partitioned + retention-managed like the session tables.
-- No session_id: namespace is the attribution key. source distinguishes the
-- producer (embedding | ingestion | consolidation | judge | ...).
CREATE TABLE provider_usage (
    id             UUID             NOT NULL DEFAULT gen_random_uuid(),
    namespace      TEXT             NOT NULL,
    workspace_name TEXT,
    provider       TEXT             NOT NULL,
    provider_name  TEXT,
    model          TEXT             NOT NULL DEFAULT '',
    source         TEXT             NOT NULL,
    input_tokens   BIGINT           NOT NULL DEFAULT 0,
    output_tokens  BIGINT           NOT NULL DEFAULT 0,
    cached_tokens  BIGINT           NOT NULL DEFAULT 0,
    cost_usd       DOUBLE PRECISION NOT NULL DEFAULT 0,
    call_count     INTEGER          NOT NULL DEFAULT 1,
    created_at     TIMESTAMPTZ      NOT NULL DEFAULT now(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE INDEX idx_provider_usage_namespace_created ON provider_usage (namespace, created_at DESC);
CREATE INDEX idx_provider_usage_source            ON provider_usage (namespace, source, created_at DESC);

-- message_artifacts: multimodal artifact metadata (partitioned weekly by created_at).
CREATE TABLE message_artifacts (
    id              UUID        NOT NULL DEFAULT gen_random_uuid(),
    message_id      UUID        NOT NULL,
    session_id      UUID        NOT NULL,
    artifact_type   TEXT        NOT NULL,
    mime_type       TEXT        NOT NULL,
    storage_uri     TEXT        NOT NULL,
    size_bytes      BIGINT,
    filename        TEXT,
    checksum_sha256 TEXT,
    metadata        JSONB       NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    width           INTEGER,
    height          INTEGER,
    duration_ms     INTEGER,
    channels        INTEGER,
    sample_rate     INTEGER,
    CONSTRAINT message_artifacts_type_check CHECK (artifact_type IN ('image', 'audio', 'video', 'document', 'file')),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE INDEX idx_message_artifacts_message ON message_artifacts (message_id);
CREATE INDEX idx_message_artifacts_session ON message_artifacts (session_id, created_at);

-- runtime_events: first-class runtime lifecycle events (partitioned weekly by timestamp).
CREATE TABLE runtime_events (
    id            UUID        NOT NULL,
    session_id    UUID        NOT NULL,
    event_type    TEXT        NOT NULL,
    data          JSONB       NOT NULL DEFAULT '{}',
    duration_ms   BIGINT,
    error_message TEXT,
    "timestamp"   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, "timestamp")
) PARTITION BY RANGE ("timestamp");

CREATE INDEX idx_runtime_events_session ON runtime_events (session_id, "timestamp");
CREATE INDEX idx_runtime_events_type    ON runtime_events (event_type, "timestamp" DESC);

-- audit_log: privacy/access audit trail (partitioned weekly by timestamp).
CREATE TABLE audit_log (
    id           BIGSERIAL,
    "timestamp"  TIMESTAMPTZ NOT NULL DEFAULT now(),
    event_type   TEXT        NOT NULL,
    session_id   UUID,
    user_id      TEXT,
    workspace    TEXT,
    agent_name   TEXT,
    namespace    TEXT,
    query        TEXT,
    result_count INTEGER,
    ip_address   INET,
    user_agent   TEXT,
    reason       TEXT,
    metadata     JSONB DEFAULT '{}'::jsonb,
    CONSTRAINT audit_log_event_type_check CHECK (event_type IN ('session_created', 'session_accessed', 'session_searched', 'session_exported', 'session_deleted', 'pii_redacted', 'decryption_requested')),
    PRIMARY KEY (id, "timestamp")
) PARTITION BY RANGE ("timestamp");

CREATE INDEX idx_audit_log_event_type ON audit_log (event_type, "timestamp" DESC);
CREATE INDEX idx_audit_log_session    ON audit_log (session_id, "timestamp" DESC) WHERE session_id IS NOT NULL;
CREATE INDEX idx_audit_log_user       ON audit_log (user_id, "timestamp" DESC) WHERE user_id IS NOT NULL;
CREATE INDEX idx_audit_log_workspace  ON audit_log (workspace, "timestamp" DESC) WHERE workspace IS NOT NULL;

-- eval_results: evaluation verdicts. NOW partitioned weekly by created_at and in
-- the retention set (previously unpartitioned -> unbounded growth + orphans when
-- session partitions were dropped). Judge token/cost are NOT here — the judge's
-- provider call is recorded in provider_calls(source='judge').
CREATE TABLE eval_results (
    id                 UUID         NOT NULL DEFAULT gen_random_uuid(),
    session_id         UUID         NOT NULL,
    message_id         TEXT,
    agent_name         TEXT         NOT NULL,
    namespace          TEXT         NOT NULL,
    promptpack_name    TEXT         NOT NULL,
    promptpack_version TEXT,
    eval_id            TEXT         NOT NULL,
    eval_type          TEXT         NOT NULL,
    trigger            TEXT         NOT NULL,
    passed             BOOLEAN      NOT NULL,
    score              NUMERIC(5,4),
    details            JSONB,
    duration_ms        INTEGER,
    -- Judge token/cost are NOT stored here: the judge's provider call is
    -- recorded in provider_calls(source='judge') for one consistent usage
    -- model (issue #1301). eval_results is the verdict, not the spend.
    source             TEXT         NOT NULL DEFAULT 'worker',
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT now(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE INDEX idx_eval_results_session_id        ON eval_results (session_id);
CREATE INDEX idx_eval_results_session_message   ON eval_results (session_id, message_id);
CREATE INDEX idx_eval_results_eval_id_created   ON eval_results (eval_id, created_at);
-- namespace-leading (dashboards filter by namespace first, agent sometimes).
CREATE INDEX idx_eval_results_namespace_agent_created ON eval_results (namespace, agent_name, created_at DESC);

-- deletion_requests: DSAR / right-to-erasure tracking (not partitioned).
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

-- user_privacy_preferences: per-user recording opt-out (not partitioned).
CREATE TABLE user_privacy_preferences (
    user_id            TEXT        NOT NULL,
    opt_out_all        BOOLEAN     DEFAULT false,
    opt_out_workspaces TEXT[]      DEFAULT '{}'::text[],
    opt_out_agents     TEXT[]      DEFAULT '{}'::text[],
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id)
);

CREATE INDEX idx_privacy_prefs_updated ON user_privacy_preferences (updated_at);

-- ---------------------------------------------------------------------------
-- Partition management
-- ---------------------------------------------------------------------------

CREATE OR REPLACE FUNCTION create_weekly_partitions(
    target_table TEXT,
    start_date   DATE,
    end_date     DATE
) RETURNS INTEGER AS $$
DECLARE
    week_start     DATE;
    week_end       DATE;
    partition_name TEXT;
    created_count  INTEGER := 0;
BEGIN
    week_start := start_date - ((EXTRACT(ISODOW FROM start_date)::INTEGER - 1) || ' days')::INTERVAL;
    WHILE week_start < end_date LOOP
        week_end := week_start + INTERVAL '7 days';
        partition_name := target_table || '_w' || to_char(week_start, 'IYYY_IW');
        IF NOT EXISTS (
            SELECT 1 FROM pg_class c
            JOIN pg_namespace n ON n.oid = c.relnamespace
            WHERE c.relname = partition_name AND n.nspname = current_schema()
        ) THEN
            EXECUTE format(
                'CREATE TABLE %I PARTITION OF %I FOR VALUES FROM (%L) TO (%L)',
                partition_name, target_table, week_start, week_end
            );
            created_count := created_count + 1;
        END IF;
        week_start := week_end;
    END LOOP;
    RETURN created_count;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION drop_old_partitions(
    target_table   TEXT,
    retention_days INTEGER
) RETURNS INTEGER AS $$
DECLARE
    cutoff_date   DATE;
    rec           RECORD;
    dropped_count INTEGER := 0;
BEGIN
    cutoff_date := CURRENT_DATE - (retention_days || ' days')::INTERVAL;
    FOR rec IN
        SELECT c.relname AS partition_name,
               pg_get_expr(c.relpartbound, c.oid) AS bound_expr
        FROM pg_class c
        JOIN pg_inherits i ON i.inhrelid = c.oid
        JOIN pg_class parent ON parent.oid = i.inhparent
        JOIN pg_namespace n ON n.oid = parent.relnamespace
        WHERE parent.relname = target_table
        AND n.nspname = current_schema()
        AND c.relispartition
    LOOP
        DECLARE
            upper_bound DATE;
        BEGIN
            SELECT (regexp_match(rec.bound_expr, 'TO \(''([^'']+)''\)'))[1]::DATE
            INTO upper_bound;
            IF upper_bound IS NOT NULL AND upper_bound <= cutoff_date THEN
                EXECUTE format('DROP TABLE %I', rec.partition_name);
                dropped_count := dropped_count + 1;
            END IF;
        EXCEPTION
            WHEN OTHERS THEN
                NULL;
        END;
    END LOOP;
    RETURN dropped_count;
END;
$$ LANGUAGE plpgsql;

-- manage_session_partitions now also covers eval_results (newly partitioned)
-- and provider_usage (new).
CREATE OR REPLACE FUNCTION manage_session_partitions(
    retention_days  INTEGER DEFAULT 30,
    lookahead_weeks INTEGER DEFAULT 2
) RETURNS TABLE(table_name TEXT, partitions_created INTEGER, partitions_dropped INTEGER) AS $$
DECLARE
    tables TEXT[] := ARRAY[
        'sessions', 'messages', 'tool_calls', 'provider_calls',
        'provider_usage', 'runtime_events', 'message_artifacts',
        'audit_log', 'eval_results'
    ];
    tbl        TEXT;
    created    INTEGER;
    dropped    INTEGER;
    start_date DATE;
    end_date   DATE;
BEGIN
    start_date := CURRENT_DATE - (retention_days || ' days')::INTERVAL;
    end_date   := CURRENT_DATE + (lookahead_weeks * 7 || ' days')::INTERVAL;
    FOREACH tbl IN ARRAY tables LOOP
        created := create_weekly_partitions(tbl, start_date, end_date);
        dropped := drop_old_partitions(tbl, retention_days);
        table_name := tbl;
        partitions_created := created;
        partitions_dropped := dropped;
        RETURN NEXT;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- ---------------------------------------------------------------------------
-- Cascade delete for explicit session deletion (DSAR / manual). Partition-drop
-- retention does NOT fire this trigger; child tables self-clean via their own
-- partition drops. provider_usage is workspace-scoped (no session_id) and is
-- intentionally not cascaded here.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION cascade_delete_session() RETURNS TRIGGER AS $$
BEGIN
    DELETE FROM message_artifacts WHERE session_id = OLD.id;
    DELETE FROM tool_calls       WHERE session_id = OLD.id;
    DELETE FROM provider_calls   WHERE session_id = OLD.id;
    DELETE FROM runtime_events   WHERE session_id = OLD.id;
    DELETE FROM eval_results     WHERE session_id = OLD.id;
    DELETE FROM messages         WHERE session_id = OLD.id;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_session_cascade_delete
    BEFORE DELETE ON sessions
    FOR EACH ROW EXECUTE FUNCTION cascade_delete_session();

-- Create initial partitions: 4 weeks back + 2 weeks ahead for all managed tables.
SELECT manage_session_partitions(28, 2);
