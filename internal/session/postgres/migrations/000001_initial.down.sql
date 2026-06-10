-- Tear down the consolidated initial schema. The product is pre-GA; this drops
-- everything rather than attempting a staged rollback.

DROP TRIGGER IF EXISTS trg_session_cascade_delete ON sessions;

DROP TABLE IF EXISTS provider_usage CASCADE;
DROP TABLE IF EXISTS provider_calls CASCADE;
DROP TABLE IF EXISTS runtime_events CASCADE;
DROP TABLE IF EXISTS message_artifacts CASCADE;
DROP TABLE IF EXISTS tool_calls CASCADE;
DROP TABLE IF EXISTS eval_results CASCADE;
DROP TABLE IF EXISTS audit_log CASCADE;
DROP TABLE IF EXISTS messages CASCADE;
DROP TABLE IF EXISTS sessions CASCADE;
DROP TABLE IF EXISTS deletion_requests CASCADE;
DROP TABLE IF EXISTS user_privacy_preferences CASCADE;

DROP FUNCTION IF EXISTS cascade_delete_session() CASCADE;
DROP FUNCTION IF EXISTS manage_session_partitions(INTEGER, INTEGER);
DROP FUNCTION IF EXISTS drop_old_partitions(TEXT, INTEGER);
DROP FUNCTION IF EXISTS create_weekly_partitions(TEXT, DATE, DATE);
