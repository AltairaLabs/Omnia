# policy-broker

Operator-injected sidecar (Enterprise) in the agent pod that answers "may
this tool call proceed, and what headers should be injected?" for ToolPolicy
CRDs. It is a **called decision** service, not a reverse proxy: the runtime
makes one HTTP call to it per tool call and gets back a decision, then
dispatches the tool call itself. Traffic to the tool's actual destination
never flows through the broker.

This replaces the pre-P2.4 `policy-proxy` sidecar, which sat inline in the
tool-call path as a reverse proxy. `policy-proxy` never worked in production
(nothing pointed traffic at it) and has been retired end-to-end; ToolPolicy
enforcement now happens exclusively through this decision-broker path.

## Owns

- CEL expression evaluation of ToolPolicy rules against a per-tool-call
  decision request (headers, body, structured caller identity).
- ToolPolicy CRD watching (informer) scoped to the agent's namespace —
  compiles rules on add/update, removes on delete.
- Header-injection evaluation for allowed calls (e.g. stamping identity
  claims onto the outbound tool request).
- Its own audit-style structured logging of policy decisions
  (`policy_decision` / `broker_tool_decision` log lines); skips
  wholly-uninteresting allows (no rule matched) to keep audit noise low.

## Inputs

### `POST /v1/decision` (`:8090`, `POLICY_BROKER_LISTEN_ADDR`)

Called by the runtime's `internal/runtime/tools.PolicyBrokerClient` (via
`OmniaExecutor.dispatch`) once per tool call, over `POLICY_BROKER_URL`
(injected as `http://localhost:8090` by the operator — see
`internal/controller/deployment_builder_env.go`).

Request body (`policy.DecisionRequest`):

```json
{
  "headers": {"X-Omnia-Tool-Name": "...", "...": "..."},
  "body": {"...": "..."},
  "identity": {
    "origin": "...", "subject": "...", "endUser": "...",
    "workspace": "...", "agent": "...", "role": "...",
    "claims": {"...": "..."}
  }
}
```

`identity` is optional (nil when the runtime has no propagated identity);
when present it is rebuilt into an `AuthenticatedIdentity` and attached to
the evaluation context so `identity.*` CEL rules work.

Response body (`policy.DecisionResponse`):

```json
{
  "allow": true,
  "deniedBy": "",
  "message": "",
  "mode": "enforce",
  "wouldDeny": false,
  "injectedHeaders": {"...": "..."}
}
```

`injectedHeaders` is only computed and returned when `allow` is true — a
denied call never receives injected headers. `wouldDeny` surfaces
"would-have-denied" for policies running in dry-run/audit mode without
actually blocking the call.

### Health server (`:8091`, `POLICY_BROKER_HEALTH_ADDR`)

| Path | Description |
|------|-------------|
| `GET /healthz` | Liveness probe |
| `GET /readyz` | Readiness probe |

### K8s API

Watches `ToolPolicy` CRDs (`ee/api/v1alpha1`) in the agent's namespace
(`OMNIA_NAMESPACE`) via `ee/pkg/policy.Watcher` — an initial list-and-compile
on startup, then a poll loop to detect changes.

## Outputs

- `POST /v1/decision` response (above) back to the calling runtime — no
  outbound calls to any tool destination; the broker is never in the data
  path.

## Does NOT Own

- **Proxying tool-call traffic** — the runtime calls the tool's real
  destination itself; the broker only renders a decision.
- **Policy CRD definition** — the Operator manages `ToolPolicy` (and
  `AgentPolicy`) CRDs; the broker only watches and compiles them.
- **Tool execution** — server tools, MCP tools, and platform tools all
  execute in the runtime (`internal/runtime/tools`), not here.

## Fail-closed behavior

The runtime's `PolicyBrokerClient` treats an unreachable/erroring broker as a
**deny** by default (`POLICY_BROKER_FAIL_MODE=closed`, the default — an
enforcement layer that silently no-ops when its decision service is down is
exactly the failure mode this design avoids). `POLICY_BROKER_FAIL_MODE=open`
switches to fail-open for environments that prefer availability over strict
enforcement. When `POLICY_BROKER_URL` is unset entirely, the client is
disabled and every call is allowed with no injected headers (zero behavior
change for deployments that don't run a broker).

## Enterprise gating

policy-broker is only injected when the operator is configured with
`PolicyBrokerImage` (see `internal/controller/policy_broker_sidecar.go`); it
ships as an Enterprise-only sidecar alongside facade + runtime and is not a
standalone reconciled Deployment.

## Dependencies

- **Kubernetes API** — ToolPolicy CRD watch (informer), scoped to the
  agent's namespace.
- **Operator/arena-controller `/api/v1/license`** (optional) — read once at
  startup via `OPERATOR_API_URL` (stamped onto the sidecar by the operator)
  for the license-awareness nag (#1682). policy-broker is enterprise-only, so
  an unlicensed deployment (open-core/absent/expired) logs a startup
  reminder. It **never blocks** — enforcement keeps running; a valid license
  is silent.

## Observability

**Metrics**: None currently — policy decisions are logged, not exported as
Prometheus metrics.

**Traces**: None.
