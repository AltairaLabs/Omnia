# Operator Service

## Owns
- Kubernetes controller-manager reconciling Omnia CRDs:
  - AgentRuntime — creates Facade + Runtime Deployments/Services
  - PromptPack — validates pack schema, reports status
  - ToolRegistry — syncs tool metadata
  - Provider — validates LLM provider configuration
  - Workspace — manages tenant namespaces and storage
  - SessionRetentionPolicy — manages session cleanup/retention
  - AgentPolicy — enforces agent-level policies
- Enterprise controllers (gated behind `--enterprise` flag, registered via `ee/pkg/setup`):
  - SessionPrivacyPolicy — privacy policy inheritance and enforcement
  - ToolPolicy — CEL-based tool call policy enforcement
  - LicenseActivation — license activation and heartbeats (`--license-server-url`, `--cluster-name`)
  - SessionPrivacyPolicy webhook — validates inheritance rules (when webhook certs configured)
- Dashboard server (embedded Next.js app via `dashboard/server.js`)
- REST API for dashboard proxy routes
- Deploy-intent API (`internal/api/deploy`, gated by `--deploy-api-bind-address`) — translates
  a versioned `DeployIntent` into PromptPack/ConfigMap/ToolRegistry(create-only)/AgentPolicy/
  AgentRuntime objects, including per-agent externalAuth/memory/evals mapping; see
  Inputs/Outputs and the "Does NOT Own" note below
- Webhook validation for CRDs
- Prometheus metrics endpoints
- Health probes

## Inputs
- **K8s API**: watch events for all Omnia CRDs
- **K8s API**: watches `HTTPRoute` and `Gateway` (`gateway.networking.k8s.io`) read-only to derive external endpoints (disabled gracefully when Gateway API CRDs are absent; requires operator restart if installed later)
- **HTTP** from Dashboard: proxy requests to Session API and other backends
- **HTTP** (workspace-scoped, dashboard-minted mgmt-plane JWT, editor role required — same
  auth model as the content API): `POST /api/v1/workspaces/{workspace}/deployments`, a
  versioned `DeployIntent` body (`internal/api/deploy`). Gated by `--deploy-api-bind-address`
  (requires `--mgmt-plane-jwks-url`); disabled when the flag is empty.
- Helm chart values at deployment time

## Outputs
- **K8s API**: Deployments, Services, ConfigMaps, PVCs, Events, CRD status updates
- **K8s API**: `AgentRuntime.status.facade.endpoints` — external URLs derived from observed HTTPRoutes (empty if cluster-internal only)
- **K8s API** (deploy-intent API only): creates a `PromptPack` + content `ConfigMap`,
  create-only-creates a `ToolRegistry` (when `tools.handlers` is set — never updates an
  existing one) and an `AgentPolicy` (when `policy.toolBlocklist` is set, denylisting
  against that registry), and upserts one or more `AgentRuntime` objects translated from a
  `DeployIntent` — idempotent (pack/ConfigMap/ToolRegistry create-once, `AgentRuntime`
  rollout-aware upsert) and best-effort (`DeployResult` reports per-object
  created/updated/unchanged/failed). Per-agent `externalAuth`/`memory`/`evals` intent
  fields map onto the `AgentRuntime`'s `spec.externalAuth`/`spec.memory`/`spec.evals`.
- **HTTP** to Dashboard: proxied responses
- **HTTP**: `DeployResult` response to the deploy-intent API caller (200, or 207 on partial failure)
- **Prometheus** metrics: reconciliation counts, retention stats

## Does NOT Own
- Session storage (Session API's job)
- LLM conversation logic (Runtime's job)
- WebSocket/HTTP protocol handling (Facade/Session API's job)
- Tool execution (Runtime's job)
- **AgentRuntime / PromptPack authoring, existing path** — for the dashboard's workspace CRD REST API (the in-app deploy wizard **or**, today, the external `promptarena-deploy-omnia` adapter), the operator only *reconciles* these CRDs; it never constructs their specs. That path writes to the Kubernetes API directly — not through the operator — so a schema-version mismatch surfaces only here, as a reconcile error. See `dashboard/SERVICE.md` → "Deploy / CRD REST API". **Exception:** the deploy-intent API (`POST /api/v1/workspaces/{workspace}/deployments`, `internal/api/deploy`) inverts this for callers that adopt it — the operator *does* construct the PromptPack/ConfigMap/AgentRuntime specs server-side from a versioned, CRD-agnostic `DeployIntent`. This is Plan A of the deploy-intent decoupling epic; the adapter has not migrated to call it yet, so both authoring paths coexist for now.
- Authentication/authorization (external RBAC/Istio)

## Observability

**Metrics** (Prometheus, prefix `omnia_retention_`):
- Retention: `active_policies`, `workspace_overrides`, `reconcile_errors_total`
- Standard controller-runtime metrics (reconciliation counts, queue depth, work duration)

**Traces**: None — uses controller-runtime's built-in logging; tracing config is passed through to Facade/Runtime pods.

## Dependencies
- controller-runtime / client-go (K8s interaction)
- Omnia CRD types (`api/v1alpha1/`)
- Enterprise CRD types (`ee/api/v1alpha1/`) — scheme always registered; controllers gated by `--enterprise`
- Enterprise setup (`ee/pkg/setup`) — registers EE controllers and webhooks
- Dashboard build output (`dashboard/`)
- Schema validation (`internal/schema`)
