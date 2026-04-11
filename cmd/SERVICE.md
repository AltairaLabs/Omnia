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
- Webhook validation for CRDs
- Prometheus metrics endpoints
- Health probes

## Inputs
- **K8s API**: watch events for all Omnia CRDs
- **HTTP** from Dashboard: proxy requests to Session API and other backends
- Helm chart values at deployment time

## Outputs
- **K8s API**: Deployments, Services, ConfigMaps, PVCs, Events, CRD status updates
- **HTTP** to Dashboard: proxied responses
- **Prometheus** metrics: reconciliation counts, retention stats

## Does NOT Own
- Session storage (Session API's job)
- LLM conversation logic (Runtime's job)
- WebSocket/HTTP protocol handling (Facade/Session API's job)
- Tool execution (Runtime's job)
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
