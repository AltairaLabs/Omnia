# Memory API Service

## Overview

Per-workspace HTTP service for agentic memory operations. Stores, retrieves,
and searches memory entries with optional semantic search via embeddings.

## Ownership

- Memory entity lifecycle (save, retrieve, search, forget)
- Embedding generation (via Provider CRD reference)
- Consent grant management
- Memory retention/TTL enforcement
- Privacy/deletion processing

## Inputs

- HTTP REST API on port 8080 (memory CRUD, search, consent)
- Health/readiness probes on port 8081
- Metrics on port 9090

## Configuration

**In-cluster (managed by operator):**
Configured via Workspace CRD. Operator creates Deployment with:
- `--workspace=<name>` — Workspace CRD name
- `--service-group=<name>` — Service group within workspace

Reads database Secret, embedding Provider CRD ref, and retention config from
the Workspace CRD.

**Local dev:**
- `--postgres-conn` / `POSTGRES_CONN` — PostgreSQL connection string
- `--embedding-provider` / `EMBEDDING_PROVIDER` — Provider CRD name (optional)
- `--default-ttl` / `DEFAULT_TTL` — Default memory TTL (optional)

## Data Flow

Agent Pod (runtime) → HTTP → Memory API → PostgreSQL

## Dependencies

- PostgreSQL (required) — memory storage
- Provider CRD + embedding model (optional) — semantic search
- Redis (optional) — event publishing

## Warning: No Embedding Provider

If started without a providerRef in the Workspace CRD, semantic search is
disabled. The service logs warnings on search requests that would have used
embeddings.

## Consent classification (EE)

When `--enterprise=true`, the privacy middleware runs a consent-category
validator on every memory write:

1. The caller's `metadata.consent_category` (or `category` field on
   `SaveMemoryRequest`) is the primary signal.
2. A PII regex pass (`ee/pkg/privacy/classify`) classifies content into
   `memory:health`, `memory:identity`, `memory:location` based on
   structured patterns.
3. If `--embedding-provider` is set, an embedding-similarity pass
   classifies content against per-category exemplar centroids covering
   `memory:preferences`, `memory:context`, `memory:history` plus
   reinforcement of the regex-detected categories.
4. The validator merges all signals using upgrade-only semantics:
   detected categories override the caller's claim only when more
   restrictive (`memory:health` > `memory:identity`/`memory:location` >
   the rest). `analytics:aggregate` is left alone.

Disable embedding-based classification by leaving `--embedding-provider`
unset; the validator degrades to regex-only, populating the column for
the three PII categories. Health/preferences/context recall is reduced
in this mode.

The validator is gated by `--enterprise`; OSS deployments are unaffected
and `consent_category` stays `NULL` (binary opt-out is the only gate).

### Metrics

- `omnia_memory_classify_overrides_total{from,to,source}` — how often
  callers mis-tag and which pass caught it.
- `omnia_memory_classify_filled_total{category,source}` — fill-in rate
  per category.
- `omnia_memory_classify_category_total{category,source}` — distribution
  of stored categories.
- `omnia_memory_classify_errors_total{reason}` — embedding failures.

### Future work

A Helm sidecar option (`memoryApi.embeddingSidecar.enabled`) is planned
to provision an Ollama container with `nomic-embed-text` next to each
memory-api Pod. Tracked separately because memory-api is dynamically
provisioned by the operator's WorkspaceReconciler — the sidecar requires
plumbing through `ServiceBuilder.BuildMemoryDeployment`. Until then,
operators wanting embedding-based classification configure an external
embedding Provider CRD via the Workspace.

