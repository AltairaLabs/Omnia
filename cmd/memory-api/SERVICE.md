# Memory API Service

## Overview

Per-workspace HTTP service for agentic memory operations. Stores, retrieves,
and searches memory entries with optional semantic search via embeddings.

## Ownership

- Memory entity lifecycle (save, retrieve, search, forget)
- Embedding generation (via Provider CRD reference)
- Embedding schema (the `vector(N)` columns + indexes — application-managed, see below)
- Consent grant management
- Memory retention/TTL enforcement
- Privacy/deletion processing

## Embedding schema (reconciler-owned, #1309)

The `memory_entities.embedding` and `memory_observations.embedding` pgvector
columns are NOT created by the SQL migrations. Their dimension must match the
configured embedding provider, which isn't known until startup, so the migration
can't hardcode it. `EnsureEmbeddingSchema`
(`internal/memory/postgres/embedding_schema.go`) runs once at startup — after
migrations, after the provider is built — and brings both columns to
`provider.Dimensions()` (falling back to 1536 when no provider is configured, so
the columns still exist for consolidation dup-detection). It is the single source
of truth for the embedding-column shape; the migrations are the source of truth
for everything else.

Changing the dimension on a store that already holds embeddings discards them
(every vector must be re-embedded). That path is gated by a single-use consent
marker (`memory_embedding_dim_change_consent`) naming the exact target dimension,
recorded via `POST /admin/embedding-dimension-change` and consumed atomically by
the reconciler on the next reshape. Empty/fresh columns reshape with no consent.

## Inputs

- HTTP REST API on port 8080 (memory CRUD, search, consent, analytics)
  - `POST /api/v1/institutional/ingest` — accepts `{workspace_id, title, url,
    site, text}`; runs the configured IngestionStrategy (default ChunkStrategy)
    and persists each chunk as an institutional memory keyed by
    `about={kind:"sharepoint_doc", key:"<url>#<index>"}` (idempotent re-seed).
    Returns 202 with no body; embeddings backfilled async by ReembedWorker.
    400 on missing `workspace_id`; 500 on no-strategy or strategy error (#1205).
  - `POST /api/v1/memories/retrieve` — accepts `{workspace_id, user_id,
    agent_id, query, types, purposes, min_confidence, limit}`; runs ranked
    retrieval across institutional, agent, user and user-for-agent tiers. With
    an embedding provider configured and a non-empty query it is hybrid: FTS
    rank and pgvector cosine rank fused via RRF (k=60) so semantic-only matches
    surface; without an embedder / on embed failure / empty query it falls back
    to FTS-only multi-tier. The per-tier MemoryPolicy `TierRanker` then biases
    the fused score, and `spec.recall.halfLife.{user,agent,institutional}`
    drives the per-tier recency decay (default 30d per tier). Response
    `{memories:[...], total:N}` (200); 400 on missing
    `workspace_id`. This is the path the agent `memory__recall` tool hits when
    the scope carries `agent_id`.
  - `POST /api/v1/memories/retrieve/semantic` — accepts `{workspace_id, query,
    deny_cel, limit}`; runs workspace-scoped hybrid retrieval (semantic + FTS)
    then applies a CEL deny-filter over each result's metadata. Empty `deny_cel`
    = no filtering; malformed `deny_cel` fails closed (500). Response:
    `{memories:[...], total:N}` (200). 400 on missing `workspace_id` (#1205).
  - `GET /api/v1/memories/aggregate` — workspace-scoped GROUP BY aggregate
    for the operator dashboard (#1004). Composes the analytics:aggregate
    consent filter from Phase D. Supports `groupBy=category|agent|day|tier`;
    tier returns `institutional` / `agent` / `user` counts derived from
    `virtual_user_id` / `agent_id` columns.
  - `GET /api/v1/privacy/consent/stats` (EE only) — workspace-wide consent
    posture for the operator dashboard.
  - `POST /admin/embedding-dimension-change` — records one-shot consent to change
    the embedding vector dimension (`{"target_dim": <1..2000>}`). See "Embedding
    schema" below (#1309).
  - All memory list responses (`/api/v1/memories`, `/memories/search`,
    `/memories/export`, `/institutional/memories`, `/agent-memories`) carry a
    derived `tier` field (institutional / agent / user) on each row (#1017).
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
- `--ingest-chunk-size` / `INGEST_CHUNK_SIZE` — Word count per ingest chunk (default 200)
- `--ingest-chunk-overlap` / `INGEST_CHUNK_OVERLAP` — Overlapping words between adjacent chunks (default 40)

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

### Memory Galaxy projection — consent stance

`GET /api/v1/memories/projection` is a clustering overview that aggregates
memories across users for an operator/demo view. Every memory contributes a
point so cluster shape is faithful, but points whose consent category is
PII-sensitive (`memory:identity`, `memory:location`, `memory:health`) are
**masked**: their identifying and content fields (`id`, `title`, `preview`,
`user`, `userRef`, `category`, `type`) are stripped **server-side before
serialization**, leaving an anonymous, non-interactive dot (`x`, `y`, `tier`,
`confidence`, timestamps, `masked:true`). Dropping `id` is the security
boundary — a masked dot cannot be clicked through to the full memory via
`GET /api/v1/memories/{id}`. Full sensitive content is only available through
the dedicated memory detail/list API with its own access control, never the
galaxy.

Masking is **read-time** (applied on every serve, including the cached coords
path) and **scope-independent** (it applies even with a `user_id` filter, so an
operator cannot enumerate users to defeat it).

**Not yet covered:** retroactive opt-out masking (hiding memories whose user has
since opted out) is deferred to #1642 — the consent table keys on a raw user id
while projection points key on a pseudonym, so the correlation can't be made
reliable until the id contract is unified.

### Metrics

- `omnia_memory_classify_overrides_total{from,to,source}` — how often
  callers mis-tag and which pass caught it.
- `omnia_memory_classify_filled_total{category,source}` — fill-in rate
  per category.
- `omnia_memory_classify_category_total{category,source}` — distribution
  of stored categories.
- `omnia_memory_classify_errors_total{reason}` — embedding failures.

Embedding-pipeline health (the `--metrics-collect-interval` collector,
default 60s, refreshes the two gauges per workspace; #1442):

- `omnia_memory_embedding_coverage{workspace}` — fraction (0..1) of a
  workspace's live entities whose latest active observation carries an
  embedding. Below the projector's dense threshold (0.7) the Memory
  Galaxy renders on the lexical basis.
- `omnia_memory_reembed_backlog{workspace}` — count of active
  observations awaiting (re-)embedding for the current model (the
  re-embed worker's queue depth).
- `omnia_memory_projection_renders_total{workspace,policy,status,basis}`
  — the `basis` label (dense|lexical|unknown) makes a galaxy degrading to
  lexical a queryable/alertable condition.

The "Omnia Memory Pipeline" Grafana dashboard
(`charts/omnia/dashboards/omnia-memory-pipeline.json`) visualises these.

### Future work

A Helm sidecar option (`memoryApi.embeddingSidecar.enabled`) is planned
to provision an Ollama container with `nomic-embed-text` next to each
memory-api Pod. Tracked separately because memory-api is dynamically
provisioned by the operator's WorkspaceReconciler — the sidecar requires
plumbing through `ServiceBuilder.BuildMemoryDeployment`. Until then,
operators wanting embedding-based classification configure an external
embedding Provider CRD via the Workspace.

