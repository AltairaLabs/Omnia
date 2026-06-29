-- Durable outbox for consent revocations (#1661). privacy-api records each
-- revocation here in the same tx as the consent-grant removal; a replay worker
-- re-delivers any push that was dropped. delivered_at NULL = pending delivery.
CREATE TABLE consent_revocation_outbox (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      TEXT        NOT NULL,
    category     TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at TIMESTAMPTZ
);
CREATE INDEX idx_consent_outbox_undelivered
    ON consent_revocation_outbox (created_at) WHERE delivered_at IS NULL;
