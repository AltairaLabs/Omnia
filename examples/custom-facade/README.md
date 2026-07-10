# custom-facade

A minimal, buildable **reference custom facade** for Omnia
(`spec.facades[].type: custom`, #1768/#1773). It is the smallest working
"bring your own" facade you can copy and adapt: it authenticates its own
protocol, then speaks the runtime gRPC contract directly while emitting the
platform's flat `x-omnia-*` identity/claims metadata so the runtime and the
policy broker see the caller's identity.

The goal is not to be feature-rich — it is to show, in real code, exactly what
a third-party facade image must do to be a first-class citizen of the platform.

## What it does

1. **Authenticates its own protocol.** A trivial static bearer-token table
   (`facade.Authenticator`) maps a token to a `Principal` (id, roles, workspace,
   origin, claims). Swap this for your real credential check.
2. **Emits the identity/claims contract.** `Principal.OutboundMetadata` uses the
   public `pkg/policy` helpers (`ToGRPCMetadata`) to attach the flat
   `x-omnia-user-id`, `x-omnia-user-roles`, `x-omnia-origin`,
   `x-omnia-workspace` and per-claim `x-omnia-claim-<name>` metadata.
3. **Speaks the runtime gRPC contract.** `facade.RuntimeClient` dials the
   runtime sidecar (`OMNIA_RUNTIME_ADDRESS`) and runs a `RuntimeService/Converse`
   turn with that metadata attached.
4. **Serves health on the operator-probed port.** `/healthz` and `/readyz` on
   **`:8081`** — this MUST match the built-in facade contract
   (`DefaultFacadeHealthPort = 8081`) or the pod never goes Ready.
5. **Optionally serves the management-plane twin.** When
   `OMNIA_MGMT_PLANE_JWKS_URL` is set, a second listener on **`:18080`** verifies
   the dashboard's RS256 JWT against the JWKS endpoint and **fails closed** on any
   missing / malformed / expired / unknown-signer token.

## Ports

| Port    | Purpose                                                        |
|---------|---------------------------------------------------------------|
| `8080`  | External data-plane: `POST /chat` (bearer-authenticated).     |
| `8081`  | Health: `GET /healthz`, `GET /readyz` (operator probes this). |
| `18080` | Management-plane twin (only when `OMNIA_MGMT_PLANE_JWKS_URL` set). |

## Environment (injected by the operator for a `type: custom` facade)

| Var                        | Meaning                                              |
|----------------------------|------------------------------------------------------|
| `OMNIA_RUNTIME_ADDRESS`    | Runtime sidecar gRPC address (e.g. `localhost:9000`).|
| `OMNIA_AGENT_NAME`         | Agent name; emitted as `x-omnia-agent-name`.         |
| `OMNIA_MGMT_PLANE_JWKS_URL`| Dashboard JWKS endpoint; enables the mgmt twin.      |

## Layout

| Path                 | Purpose                                                   |
|----------------------|-----------------------------------------------------------|
| `main.go`            | Wires the health, data-plane and mgmt-twin HTTP servers.  |
| `facade/identity.go` | `Principal` + identity→`x-omnia-*` emission via pkg/policy.|
| `facade/runtimeclient.go` | Direct `RuntimeService/Converse` client.             |
| `facade/mgmt.go`     | RS256 JWKS verification, fail-closed middleware.           |
| `facade/*_test.go`   | Unit + in-process contract tests (see below).             |
| `Dockerfile`         | Distroless image build (build from the repo root).        |

It imports **only** the public contract packages (`pkg/policy`,
`pkg/runtime/v1`) plus published gRPC / JWT libraries. It deliberately does not
import the built-in facade's internal auth or session packages.

## Build

Build from the **repository root** (the Go module lives there):

```bash
docker build -t reference-custom-facade:test \
  -f examples/custom-facade/Dockerfile .
```

## Tests

```bash
# Runtime-side contract + facade unit tests (in-process, no cluster):
go test ./examples/custom-facade/...

# Broker-side contract test (proves the policy broker sees the emitted identity):
go test ./ee/pkg/policy/ -run CustomFacade
```

- `facade/identity_contract_test.go` starts a stub runtime with the **real**
  `internal/runtime` policy interceptors and asserts the runtime rehydrates the
  caller id, roles, full claim map, origin and workspace from the facade's
  emitted metadata.
- `ee/pkg/policy/custom_facade_broker_contract_test.go` feeds the same
  `Principal` through the production `IdentityPayloadFromPropagation`
  reconstruction into a real `BrokerHandler`, proving a ToolPolicy CEL rule
  matching on `identity.subject/role/origin/workspace/claims.*` fires.
