# Operator Service

## Owns
- Kubernetes controller-manager reconciling Omnia CRDs:
  - AgentRuntime — creates Facade + Runtime Deployments/Services
  - PromptPack — validates pack schema, reports status
  - ToolRegistry — syncs tool metadata
  - Provider — validates LLM provider configuration
  - Workspace — manages tenant namespaces and storage
  - SessionRetentionPolicy — manages session cleanup/retention
  - AgentPolicy, ToolPolicy — enforces policies
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

## Dependencies
- controller-runtime / client-go (K8s interaction)
- Omnia CRD types (`api/v1alpha1/`)
- Dashboard build output (`dashboard/`)
- Schema validation (`internal/schema`)
