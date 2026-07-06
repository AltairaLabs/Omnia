# Doctor Service

Diagnostic tool that probes every Omnia service in a workspace / service-group
and reports pass/fail. Runs in two modes from the same binary:

- **CLI (`--run-once`)**: runs all checks once, prints a JSON `RunResult` to
  stdout (bracketed by `=== DOCTOR-RUN-RESULT-BEGIN/END ===` sentinels so a
  parser can slice it out of interleaved log lines), then exits. With
  `--exit-code` it exits non-zero if any check failed — usable as a CI / smoke
  gate.
- **HTTP server (default)**: long-running service on `--addr` (default `:8080`)
  that serves a diagnostic dashboard (`GET /{$}`), streams check results over
  SSE, and re-runs the suite on demand. The check set is rebuilt per run so a
  Doctor pod that started before its Workspace existed recovers on the next run
  instead of permanently using fallback URLs (#1040).

## Owns

- The diagnostic check registry and runner (`internal/doctor`,
  `internal/doctor/checks`) — reachability, CRD presence, and end-to-end
  round-trip checks against the platform's services.
- Per-run service discovery: when `--workspace` is set it resolves each
  service-group's session-api / memory-api URLs via
  `pkg/servicediscovery` (falling back to flag / in-cluster DNS URLs when
  discovery fails).
- Management-plane token exchange: when `--mgmt-plane-token-url` (or
  `OMNIA_MGMT_PLANE_TOKEN_URL`) is set, Doctor exchanges its Kubernetes
  ServiceAccount token for a management-plane JWT at the dashboard's
  `/api/auth/service-token` endpoint and attaches it to every WebSocket dial.
- Sequential ordering where checks depend on each other (e.g. the `Sessions`
  check reads the `Agent` check's `LastSessionID`).

## What it dials

| Target | Purpose |
|--------|---------|
| **Agent facade** (`<agent>.<ns>`) | WebSocket round-trip. Dials the agent's **internal management-plane twin port** read from `AgentRuntime.status.managementEndpoints.ws`, falling back to the external facade port (8080) when the agent advertises no management endpoints (mgmt plane disabled or an older agent). |
| **Session API** (`omnia-session-api`) | Reachability + session CRUD round-trip (create/read/delete a probe session). |
| **Memory API** (`omnia-memory-api`) | Reachability + memory CRUD round-trip, consolidation-worker liveness (scrapes `/metrics`), and privacy checks. |
| **Operator API** (`omnia-operator`) | REST API reachability. |
| **Dashboard** (`omnia-dashboard`) | UI reachability. |
| **Arena Controller** (`omnia-arena-controller`) | Reachability + privacy checks. |
| **Ollama** (optional) | Embedding backend reachability. |
| **Redis** (optional) | TCP reachability. Registered only when a URL arrives via `--redis-url` / `REDIS_URL` — a Redis-less OSS install skips the check rather than surfacing a false failure. |
| **Kubernetes API** | CRD presence checks, workspace-UID resolution, and reading `status.managementEndpoints` for the facade dial. Skipped (with CRD checks) when no in-cluster client is available. |

## Inputs

- **Flags** (see `cmd/doctor/main.go`): `--addr`, `--run-once`, `--exit-code`,
  `--namespace`, `--agent-namespace`, `--agent-name`, `--workspace`,
  `--service-group`, per-service URL overrides
  (`--session-api-url`, `--memory-api-url`, `--ollama-url`, `--operator-url`,
  `--dashboard-url`, `--arena-url`), `--redis-url`, `--mgmt-plane-token-url`.
- **Environment**: `REDIS_URL`, `OMNIA_MGMT_PLANE_TOKEN_URL` (defaults for the
  matching flags).
- **HTTP** (server mode): `GET /api/v1/run` (SSE stream), `POST /api/v1/run`
  (trigger a run), `GET /api/v1/results/latest`, `GET /healthz`.

## Outputs

- **stdout** (run-once mode): a JSON `RunResult` with per-check status +
  summary, delimited by the begin/end sentinels.
- **HTTP** (server mode): the diagnostic dashboard HTML, SSE `TestResult`
  events, and the latest cached `RunResult`.
- **Probe traffic** to the services above (WebSocket, HTTP, TCP). Session and
  memory checks create and then delete their own probe records; Doctor holds no
  durable state of its own.

## Does NOT Own

- Any data-plane persistence — Doctor is a read-mostly diagnostic client. The
  authoritative stores stay with session-api / memory-api / privacy-api.
- Policy enforcement, LLM interaction, or session recording.
- Continuous monitoring / alerting — Doctor answers "is the platform wired up
  correctly right now?", not "what is the long-run health?" (that is
  Prometheus's job).

## Observability

**Metrics**: None of its own. Doctor *reads* other services' `/metrics`
endpoints as part of its checks (e.g. consolidation-worker liveness); it does
not expose a Prometheus endpoint.

**Traces**: None.

## Dependencies

- Session API + Memory API HTTP endpoints (per-workspace, discovered or
  flag-supplied).
- Operator, Dashboard, Arena Controller HTTP endpoints.
- Kubernetes API (optional) — CRD checks, workspace UID, management-endpoint
  resolution.
- Dashboard `/api/auth/service-token` (optional) — management-plane JWT
  exchange.
- Ollama, Redis (both optional).
