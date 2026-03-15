# Compaction Service

## Owns
- Tiered storage lifecycle management (hot → warm → cold)
- Session archival to cold storage (S3/GCS/Azure)
- TTL-based session expiry and cleanup
- Prometheus metrics for compaction operations

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
