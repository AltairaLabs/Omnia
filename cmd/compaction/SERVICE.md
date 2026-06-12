# Compaction Service

## Owns
- Tiered storage lifecycle management (hot → warm → cold)
- Session archival to cold storage (S3/GCS/Azure)
- TTL-based session expiry and cleanup
- Prometheus metrics for compaction operations

## Data-Retention Contract (warm → cold)

What a compaction run **archives to cold storage** (Parquet) before deleting
the warm copy:

- Session metadata (ID, agent, namespace, workspace, status, timestamps,
  token/cost counters, tags, state, last-message preview)
- **Full message history** — loaded from the warm store per session before
  the Parquet write. A session whose messages cannot be loaded is **skipped**
  (neither archived nor deleted; counted in `SessionsSkipped` and
  `errors_total{operation="load_messages"}`) and retried on the next run.
  Sessions are only deleted from the warm store after the cold write succeeds.

What a compaction run **deletes without archiving** (deliberate decision —
not covered by the Parquet schema):

- `tool_calls`, `provider_calls`, `runtime_events`, `eval_results` rows for
  the compacted sessions, removed by the `trg_session_cascade_delete`
  trigger. These are partitioned detail tables whose primary retention
  mechanism is partition drops; their per-session rows are operational detail
  and are not preserved in cold storage.

Warm-only mode (no cold archive configured) purges expired sessions and all
cascaded rows without archiving anything; dry-run mode neither archives nor
deletes.

## Inputs
- **PostgreSQL**: reads session records for archival candidates
- **Redis**: reads hot cache entries for expiry

## Outputs
- **Cold storage** (S3/GCS/Azure): archived session data
- **PostgreSQL**: deletes archived records from warm store
- **Redis**: evicts expired entries from hot cache
- **Prometheus**: compaction metrics

## Does NOT Own
- Session creation or updates (Session API's job)
- Session query/search (Session API's job)
- Retention policy reconciliation (Operator's job)

## Observability

**Metrics** (Prometheus, prefix `omnia_compaction_`):
- `run_duration_seconds`, `sessions_compacted_total`, `batches_processed_total`
- `errors_total` (by operation), `last_run_timestamp`

**Traces**: None.

## Dependencies
- PostgreSQL (warm store)
- Redis (hot cache)
- Cold storage provider (S3/GCS/Azure)
