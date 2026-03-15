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
