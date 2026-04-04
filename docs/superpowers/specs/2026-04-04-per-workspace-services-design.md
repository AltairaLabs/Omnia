# Per-Workspace Session-API and Memory-API

**Issue:** #715
**Date:** 2026-04-04
**Status:** Draft

## Problem

Session-api and memory-api are singleton Helm-managed Deployments serving all workspaces. This creates four problems:

1. **Embedding provider is global** — all workspaces share one embedding config. Different workspaces can't use different providers.
2. **No workspace isolation** — a misbehaving workspace's load affects all others.
3. **Configuration is static** — changing embedding provider requires redeploying the entire service.
4. **Tenant boundaries are leaky** — service-level config doesn't match workspace-level data boundaries.

## Decision Record

Key decisions made during design:

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Database isolation | Dedicated database per instance | Underlying database technology may change; clean separation |
| Migration | Delete and recreate | Pre-release, no production data to preserve |
| Service grouping | Paired (session + memory per group) | Enables cross-referencing memory with session data |
| Config location | Inline in Workspace CRD | Instances are workspace-scoped; separate CRD is premature |
| Managed vs external | Both modes supported | External mode allows bespoke Helm-deployed or cloud-managed services |
| Embedding provider | Optional with warning logs | Zero-config path, but memory-api logs warnings since semantic search is disabled |
| Backward compatibility | None required | Pre-release software, no migration path needed |
| Database provisioning | User-provisioned, operator consumes | Real deployments use external databases (RDS, Cloud SQL, etc.) |

## Design

### 1. Workspace CRD Extension

`WorkspaceSpec` gains a `Services` field — a slice of named, paired service groups:

```go
type WorkspaceSpec struct {
    // ... existing fields ...
    Services []WorkspaceServiceGroup `json:"services,omitempty"`
}

type WorkspaceServiceGroup struct {
    Name     string                `json:"name"`
    Mode     ServiceMode           `json:"mode,omitempty"`     // "managed" (default) or "external"
    Memory   *MemoryServiceConfig  `json:"memory,omitempty"`
    Session  *SessionServiceConfig `json:"session,omitempty"`
    External *ExternalEndpoints    `json:"external,omitempty"` // required for external mode
}

type ServiceMode string

const (
    ServiceModeManaged  ServiceMode = "managed"
    ServiceModeExternal ServiceMode = "external"
)

type MemoryServiceConfig struct {
    Database    DatabaseConfig               `json:"database"`
    ProviderRef *corev1.LocalObjectReference `json:"providerRef,omitempty"`
    Retention   *MemoryRetentionConfig       `json:"retention,omitempty"`
}

type SessionServiceConfig struct {
    Database  DatabaseConfig          `json:"database"`
    Retention *SessionRetentionConfig `json:"retention,omitempty"`
}

type DatabaseConfig struct {
    SecretRef corev1.LocalObjectReference `json:"secretRef"`
    // Secret must contain key "POSTGRES_CONN" with a full connection string
}

type MemoryRetentionConfig struct {
    DefaultTTL string `json:"defaultTTL,omitempty"` // e.g. "720h"
}

type SessionRetentionConfig struct {
    WarmDays *int32 `json:"warmDays,omitempty"`
}

type ExternalEndpoints struct {
    SessionURL string `json:"sessionURL"`
    MemoryURL  string `json:"memoryURL"`
}
```

**CEL validation rules:**
- `name` is required and unique within the slice
- If `mode == "external"`: `external` is required; `memory` and `session` blocks are ignored
- If `mode == "managed"` (or omitted): `memory.database` and `session.database` are required

**Workspace status extension:**

```go
type WorkspaceStatus struct {
    // ... existing fields ...
    Services []ServiceGroupStatus `json:"services,omitempty"`
}

type ServiceGroupStatus struct {
    Name       string `json:"name"`
    SessionURL string `json:"sessionURL"`
    MemoryURL  string `json:"memoryURL"`
    Ready      bool   `json:"ready"`
}
```

The operator writes resolved URLs into status for both managed and external modes. All consumers read from status.

### 2. AgentRuntime CRD Extension

```go
type AgentRuntimeSpec struct {
    // ... existing fields ...
    ServiceGroup string `json:"serviceGroup,omitempty"` // defaults to "default"
}
```

AgentRuntimes reference a paired service group by name. The facade and runtime resolve URLs from the Workspace status matching this name.

### 3. Operator Reconciliation

The Workspace controller reconciles service groups. For each entry in `spec.services`:

**Managed mode:**
1. Create a Deployment for session-api with startup args `--workspace=<name> --service-group=<name>`
2. Create a Deployment for memory-api with the same startup args
3. Create a Service for each with deterministic names: `session-{workspaceName}-{groupName}`, `memory-{workspaceName}-{groupName}`
4. Write resolved URLs and readiness into `WorkspaceStatus.Services[]`

All Deployments are created in the workspace's namespace with owner references to the Workspace CR for garbage collection.

**External mode:**
1. No Deployments or Services created
2. Copy URLs from `spec.services[].external` into status

**Lifecycle:**
- Service group added → create Deployments + Services
- Service group config changed → update Deployments (pods restart, re-read config from CRD)
- Service group removed → delete Deployments + Services (owner references handle cascading)
- Workspace deleted → all owned resources garbage collected

**Deployment builder:** A shared helper constructs pod specs for session-api and memory-api, similar to the existing `deployment_builder.go` for agent pods.

**RBAC:** The operator's ClusterRole needs permissions to create/manage Deployments, Services, ServiceAccounts, Roles, and RoleBindings in workspace namespaces. Per-workspace service pods need Roles to read Workspace CRDs, Provider CRDs, and Secrets in their namespace.

### 4. K8s Client and Service Resolution

A shared client package handles service discovery for both agents and services, with a local-dev fallback:

**Resolution logic:**
1. Check env var override (local dev, Docker, testing) → return if set
2. If in-cluster → use K8s client to resolve from Workspace CRD

**Namespace-to-Workspace mapping:** The client maps a pod's namespace to the Workspace CR that owns it. This mapping is cached and watched for changes.

**For agents (facade/runtime):**
```go
client.ResolveServiceGroup(ctx) → {SessionURL, MemoryURL}
```
Reads the pod's AgentRuntime CRD → gets `serviceGroup` → reads Workspace status → returns URLs for the matching service group.

**For services (session-api, memory-api):**
```go
client.ResolveServiceConfig(ctx, workspace, serviceGroup) → {DatabaseSecret, ProviderRef, Retention, ...}
```
Takes explicit workspace/service-group from startup args → reads Workspace CRD → resolves referenced Secrets and Provider CRDs.

**Watch support:** The client watches the Workspace CR. Services can react to config changes (e.g., retention policy) without restart. Database connection changes require restart or reconnection.

### 5. Service Binary Changes

**session-api (`cmd/session-api/main.go`):**
- Add `--workspace` and `--service-group` startup flags
- If flags provided: use K8s client to read Workspace CRD, resolve database Secret, retention config
- If flags absent: configure from flags/env vars (local dev mode)
- Fresh migrations — existing migrations deleted, new schema created from scratch

**memory-api (`cmd/memory-api/main.go`):**
- Same `--workspace` and `--service-group` startup flags
- K8s client resolves database Secret, Provider CRD ref (for embeddings), retention config
- If no `providerRef` configured: start without embeddings, log warning on every request that would have used semantic search
- Fresh migrations — same as session-api

**facade (`cmd/agent/main.go`) and runtime:**
- Remove `SESSION_API_URL` / `OMNIA_MEMORY_API_URL` env var reads
- Replace with K8s client resolution via `ResolveServiceGroup(ctx)`
- Env var fallback for local dev

**Each service directory gets:**
- `SERVICE.md` — architectural details: what the service owns, inputs/outputs, data flow, metrics, dependencies
- `CLAUDE.md` — dev instructions: how to run locally, flags, env vars for local dev, test commands

### 6. Downstream Consumer Updates

All code that currently talks to the singleton session-api or memory-api must become workspace-aware:

**Dashboard:**
- Proxy routes currently hit global `omnia-session-api` / `omnia-memory-api` URLs
- Requests must include workspace context; the operator resolves correct service URLs from Workspace status before proxying
- Session list, memory graph, and other views need workspace selector context

**ArenaJob / Arena (`ee/`):**
- ArenaJob controller resolves the service group for the workspace the job runs in
- Same K8s client resolution: read Workspace status → get URLs

**Operator API handlers (`internal/api/`):**
- Any handler that proxies to session-api or memory-api resolves URLs per-workspace from Workspace status

**Session HTTP client (`internal/session/httpclient/`):**
- Currently constructed with a single URL
- Must either accept URL per-call or be constructed per-workspace/service-group

**deployment_builder.go:**
- Remove `SessionAPIURL` field from reconciler struct
- Remove env var injection for `SESSION_API_URL` and `OMNIA_MEMORY_API_URL`

### 7. Helm Chart Changes

**Remove:**
- `charts/omnia/templates/session-api/` (entire directory)
- `charts/omnia/templates/memory-api/` (entire directory)
- Associated values in `values.yaml`

**Update:**
- Operator ClusterRole: add permissions for Deployments, Services, ServiceAccounts, Roles, RoleBindings in workspace namespaces
- CRD manifests: regenerated via `make manifests`
- Remove operator startup config referencing singleton service URLs

### 8. Doctor Smoke Tests

**Add:**
- **Workspace services health:** For each workspace with managed service groups, verify Deployments are running and Services respond to health checks
- **Service config resolution:** Verify each managed service can read its Workspace CRD and resolve its database Secret
- **Agent service discovery:** Verify agents can resolve service URLs from Workspace status for their configured `serviceGroup`
- **External endpoints reachable:** For external mode, verify provided URLs respond to health checks
- **Stale status detection:** Flag workspaces where status URLs don't match running Services

**Remove:**
- Any existing Doctor checks for the singleton `omnia-session-api` / `omnia-memory-api` services

### 9. Testing Strategy

**Unit tests:**
- Workspace CRD validation: CEL rules for managed vs external mode, required fields, name uniqueness
- Service resolution client: mock K8s client, test both in-cluster and local-dev paths
- Workspace controller: reconciliation creates correct Deployments/Services per service group, handles add/update/delete, owner references
- Deployment builder: no longer injects session/memory env vars

**Integration tests (envtest):**
- Create Workspace with managed service group → verify Deployments and Services exist with correct args
- Create Workspace with external service group → verify no Deployments, status has correct URLs
- Update service group config → verify Deployment updated
- Remove service group → verify Deployment and Service deleted
- Create AgentRuntime with `serviceGroup` ref → verify resolution from Workspace status
- Workspace with no `services` block → verify appropriate error/status condition

**Not in scope:**
- Wiring tests (#714 tracks separately)
- E2e tests (would need real Postgres per service group)

## Example Workspace CR

```yaml
apiVersion: omnia.altairalabs.com/v1alpha1
kind: Workspace
metadata:
  name: production
spec:
  displayName: Production
  environment: production
  namespace:
    name: ws-production
  services:
    - name: default
      mode: managed
      memory:
        database:
          secretRef:
            name: memory-db-credentials
        providerRef:
          name: ollama-embeddings
        retention:
          defaultTTL: "720h"
      session:
        database:
          secretRef:
            name: session-db-credentials
        retention:
          warmDays: 30
    - name: compliance
      mode: managed
      memory:
        database:
          secretRef:
            name: compliance-memory-db
        providerRef:
          name: openai-embeddings
        retention:
          defaultTTL: "8760h"
      session:
        database:
          secretRef:
            name: compliance-session-db
        retention:
          warmDays: 365
    - name: legacy
      mode: external
      external:
        sessionURL: "http://my-custom-session-api.legacy:8080"
        memoryURL: "http://my-custom-memory-api.legacy:8080"
```

## Dependencies

- AltairaLabs/PromptKit#842 (Ollama embedding provider) — not blocking; embedding is optional
- AltairaLabs/Omnia#714 (wiring tests) — separate backlog item

## Out of Scope

- Database provisioning automation (operator creating databases)
- Cross-workspace analytics / consolidation layer
- Wiring tests (tracked in #714)
- E2e test coverage for per-workspace services
