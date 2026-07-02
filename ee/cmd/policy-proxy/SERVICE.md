# Policy Proxy Service (Enterprise)

## Owns
- HTTP reverse proxy with policy enforcement
- CEL expression evaluation against request context
- AgentPolicy and ToolPolicy watching from K8s
- Request header injection (user identity, claims)
- Audit logging of policy decisions

## Inputs
- **HTTP** from clients: proxied requests
- **K8s API**: AgentPolicy and ToolPolicy CRD watches

## Outputs
- **HTTP** to upstream services: proxied requests (with injected headers)
- **HTTP** to clients: allow/deny responses

## Does NOT Own
- Policy definition (Operator manages CRDs)
- Agent runtime (Facade + Runtime's job)
- Session management (Session API's job)

## Observability

**Metrics**: None currently — policy decisions are logged.

**Traces**: None.

## Dependencies
- K8s client (policy watching)
- CEL evaluator
- Operator/arena-controller `/api/v1/license` (optional) — read once at startup
  via `OPERATOR_API_URL` (stamped onto the sidecar by the operator) for the
  license-awareness nag (#1682). policy-proxy is enterprise-only, so an
  unlicensed deployment (open-core/absent/expired) logs a startup reminder. It
  **never blocks** — enforcement keeps running; a valid license is silent.
