-- Audit drain-forwarder bookkeeping (#1673). memory-api ships its local audit_log
-- rows to privacy-api's central audit hub; forwarded_at marks a row as delivered
-- so the at-least-once forwarder sends each row once. NULL = not yet forwarded.
ALTER TABLE audit_log ADD COLUMN forwarded_at TIMESTAMPTZ;

-- Partial index covers the forwarder's hot query: the unforwarded backlog ordered
-- by id. Once a row is forwarded it leaves the index, keeping it small.
CREATE INDEX idx_audit_log_unforwarded ON audit_log (id) WHERE forwarded_at IS NULL;
