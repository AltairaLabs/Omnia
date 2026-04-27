-- Reverse the workspace registry table + trigger.

DROP TRIGGER IF EXISTS memory_entities_track_workspace ON memory_entities;
DROP FUNCTION IF EXISTS track_memory_workspace();
DROP TABLE IF EXISTS memory_workspaces;
