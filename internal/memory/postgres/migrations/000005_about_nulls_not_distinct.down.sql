DROP INDEX IF EXISTS idx_memory_entities_about_unique;

CREATE UNIQUE INDEX idx_memory_entities_about_unique
    ON memory_entities (workspace_id, virtual_user_id, agent_id,
                        about_kind, about_key)
    WHERE about_kind IS NOT NULL AND NOT forgotten;
