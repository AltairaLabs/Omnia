-- DSAR (right-to-erasure) orchestration moved from session-api to privacy-api
-- (#1676, Phase 2). privacy-api now owns the deletion_requests lifecycle in its
-- own database (privacy migration 000004); session-api no longer reads or writes
-- this table. Drop it here. Scope guard: this migration touches only
-- deletion_requests — the sessions/media it references are untouched.
DROP TABLE IF EXISTS deletion_requests;
