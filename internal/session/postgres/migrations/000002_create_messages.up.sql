-- Messages table: conversation messages within sessions.
-- Partitioned by timestamp (weekly) aligned with sessions.
CREATE TABLE IF NOT EXISTS messages (
    id              UUID            NOT NULL,
    session_id      UUID            NOT NULL,
    role            TEXT            NOT NULL,
    content         TEXT            NOT NULL DEFAULT '',
    timestamp       TIMESTAMPTZ     NOT NULL DEFAULT now(),
    input_tokens    INTEGER,
    output_tokens   INTEGER,
    tool_call_id    UUID,
    metadata        JSONB           NOT NULL DEFAULT '{}',
    sequence_num    INTEGER         NOT NULL DEFAULT 0,
    search_vector   tsvector        GENERATED ALWAYS AS (to_tsvector('english', content)) STORED,

    PRIMARY KEY (id, timestamp)
) PARTITION BY RANGE (timestamp);

-- Check constraint for role values
ALTER TABLE messages ADD CONSTRAINT messages_role_check
    CHECK (role IN ('user', 'assistant', 'system', 'tool'));

-- Indexes for common query patterns
CREATE INDEX idx_messages_session_seq ON messages (session_id, sequence_num ASC);
CREATE INDEX idx_messages_search ON messages USING GIN (search_vector);
CREATE INDEX idx_messages_tool_call_id ON messages (tool_call_id)
    WHERE tool_call_id IS NOT NULL;
