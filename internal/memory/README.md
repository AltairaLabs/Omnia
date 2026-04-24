# internal/memory

Per-workspace memory store for agentic memory operations. Backed by
PostgreSQL + pgvector, called by memory-api.

## Aggregate query rules

Any query that scans `memory_entities` across more than one
`virtual_user_id` MUST use `AggregateConsentJoin` (see
`aggregate_consent.go`) to filter out users who have not granted the
`analytics:aggregate` consent category.

- Institutional rows (`virtual_user_id IS NULL AND agent_id IS NULL`)
  are exempt — they are not user data.
- Agent-tier rows (`virtual_user_id IS NULL AND agent_id IS NOT NULL`)
  are exempt for the same reason.
- Single-user queries (scoped by a specific `virtual_user_id`) do NOT
  need this filter; per-user analytics are governed by the existing
  binary opt-out / per-category grants via `ShouldRememberCategory` in
  the privacy middleware.

Example:

```go
join, where := memory.AggregateConsentJoin("e")
sql := `SELECT COUNT(*) FROM memory_entities e ` + join +
       ` WHERE e.workspace_id = $1 AND e.forgotten = false AND ` + where
```

See `aggregate_consent.go` for the full contract and
`aggregate_consent_integration_test.go` for the decision table.

## Opt-in visibility

The `AnalyticsOptInWorker` exposes:

- `omnia_memory_consent_analytics_optin_ratio` — gauge (0..1), global
- `omnia_memory_consent_analytics_users_total{granted="true"|"false"}` —
  absolute count gauges
- `omnia_memory_consent_analytics_worker_errors_total{reason}` — counter

Per-workspace granularity is deferred; `user_privacy_preferences` is
user-scoped, not workspace-scoped.
