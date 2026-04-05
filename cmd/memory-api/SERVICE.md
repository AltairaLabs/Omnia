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
