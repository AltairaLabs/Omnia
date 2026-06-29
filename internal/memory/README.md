# internal/memory

Per-workspace memory store for agentic memory operations. Backed by
PostgreSQL + pgvector, called by memory-api.

## Aggregate query rules

As of CE2 (#1642, 2026-06-29 product decision), memory aggregates are
**NOT consent-filtered**. Cross-user aggregate queries (e.g.
`GET /api/v1/memories/aggregate`) count all tiers — institutional,
agent, and user — unconditionally.

The previous `AggregateConsentJoin` helper and its backing files
(`aggregate_consent.go`, `aggregate_consent_integration_test.go`) have
been deleted. Do NOT re-introduce a JOIN-based consent sweep here.

Consent revocation is now enforced per-user via the event-driven path:
`POST /api/v1/memories/consent-events` (memory-api CE1). When a user
revokes consent, that endpoint triggers an immediate prune of the
relevant memories for that user. The analytics:aggregate consent
category is owned by privacy-api (#1642); memory-api is read-unfiltered.

- Institutional rows (`virtual_user_id IS NULL AND agent_id IS NULL`)
  and agent-tier rows (`virtual_user_id IS NULL AND agent_id IS NOT NULL`)
  have no user identity and are always included.
- Single-user queries (scoped by a specific `virtual_user_id`) are
  governed by binary opt-out / per-category grants via
  `ShouldRememberCategory` in the privacy middleware — unchanged.

## Opt-in visibility

The `AnalyticsOptInWorker` and its three Prometheus gauges
(`omnia_memory_consent_analytics_optin_ratio`,
`omnia_memory_consent_analytics_users_total`,
`omnia_memory_consent_analytics_worker_errors_total`) have been removed
from memory-api as part of CE2. The opt-in metric will be re-homed to
privacy-api in a follow-up task.
