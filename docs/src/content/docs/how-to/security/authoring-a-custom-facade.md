---
title: "Authoring a custom facade"
description: "Build a bring-your-own-container facade that speaks the facade↔runtime gRPC contract and emits Omnia's identity metadata"
sidebar:
  order: 25
---

:::note[Enterprise]
Custom facades are an Enterprise-licensed capability. An AgentRuntime that
declares a `type: custom` facade is **rejected at admission** unless the cluster
has an active Omnia Enterprise license. See [Licensing](/explanation/platform/licensing/).
:::

A **custom facade** lets you replace Omnia's built-in facade container with your
own image while keeping the rest of the platform — the runtime sidecar, the
policy broker, sessions, memory, and exposure — unchanged. Use it when you need
to speak a protocol Omnia doesn't ship (a proprietary chat transport, a
telephony/CCaaS webhook, a bespoke SSE surface) but still want the runtime's LLM
orchestration, tool execution, and identity-aware policy enforcement behind it.

Your container is the **facade**: it terminates whatever protocol your clients
speak on its external port, authenticates those clients itself, and forwards each
turn to the runtime sidecar over the private facade↔runtime gRPC connection —
attaching the caller's identity as gRPC metadata so the runtime and ToolPolicy
broker can make identity-aware decisions.

This guide is the end-to-end contract your container must honour. Every env var,
port, gRPC method, and metadata key below is what the operator actually injects
and expects. For the underlying protocol reference, see
[Facade ↔ runtime protocol](/reference/platform/facade-runtime-protocol/); for
the full AgentRuntime field reference, see the
[AgentRuntime CRD reference](/reference/core/agentruntime/).

:::tip[Start from the reference implementation]
A minimal, conformant custom facade lives at `examples/custom-facade/` in the
Omnia repository. It implements every obligation in this guide — the runtime
gRPC client, identity metadata, the health server, and the optional
management-plane twin. Read it alongside this page and adapt it rather than
starting from scratch.
:::

## Prerequisites

- An active **Omnia Enterprise license** (custom facades are license-gated at
  admission).
- A runtime **`mode: agent`** AgentRuntime. A custom facade is a long-lived
  connection surface, so — like `websocket` — it is only valid in `mode: agent`.
  It cannot be used with `mode: function`.
- A container image you control, published to a registry the cluster can pull.

## Step 1 — Declare the custom facade

Add a single `type: custom` entry to `spec.facades[]` and point `image` at your
container. The `image` field is **required** for `type: custom` (enforced by CEL
on the CRD):

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: my-agent
spec:
  mode: agent
  promptPackRef:
    name: my-pack
  facades:
    - type: custom
      image: ghcr.io/acme/my-facade:v1.0.0
      port: 8080                # external listen port (default 8080)
      managementPlane: true     # serve the dashboard twin (default true)
      expose:
        enabled: true           # optional operator-provisioned HTTPRoute
```

The `port`, `expose`, `managementPlane`, `extraEnv`, `drainTimeout`, and
`clientToolTimeout` fields all apply to a custom facade exactly as they do to a
built-in one.

### The license gate

Admission of a custom facade is enforced entirely by an Enterprise validating
webhook (`AgentRuntimeCustomFacadeValidator`). The webhook is scoped to only
those AgentRuntimes that actually declare a `type: custom` facade and is
configured **`failurePolicy: Fail`** — so if the license controller is down or no
valid license is present, the AgentRuntime is **denied**. Every AgentRuntime that
does *not* declare a custom facade is admitted normally. The core reconciler is
deliberately not license-aware; this webhook is the only license gate for the
feature.

## Step 2 — Code against the injected contract

When your image runs as the facade container, the operator injects the same
wiring it gives the built-in facade. Your container **does not** read the
AgentRuntime CRD — it reads these environment variables and binds these ports.

### Environment variables

| Variable | Value | Notes |
|----------|-------|-------|
| `OMNIA_AGENT_NAME` | AgentRuntime name | From the Downward API (`app.kubernetes.io/instance` label) |
| `OMNIA_NAMESPACE` | Pod namespace | From the Downward API |
| `OMNIA_FACADE_PORT` | External listen port | Defaults to `8080`; matches `facades[].port` |
| `OMNIA_HEALTH_PORT` | Health/metrics port | Always `8081` — serve `/healthz` + `/readyz` here (see [Step 5](#step-5--serve-readiness-and-liveness)) |
| `OMNIA_HANDLER_MODE` | `runtime` | Defaults to `runtime`; a custom agent facade dispatches to the runtime sidecar |
| `OMNIA_RUNTIME_ADDRESS` | `localhost:9000` | The runtime sidecar's gRPC address — injected only in `runtime` handler mode |
| `OMNIA_MGMT_PLANE_JWKS_URL` | Dashboard JWKS URL | Present only when a dashboard is deployed; used to verify management-plane JWTs (see [Step 6](#step-6-optional--serve-the-management-plane-twin)) |
| `OMNIA_VARIANT` | `stable` \| `candidate` | Rollout variant; record it on each session when no `x-omnia-variant` request header is present |
| `POD_IP` | Pod IP | From the Downward API |
| `OMNIA_ROUTE_REDIS_URL` | Redis URL | Injected only when the agent uses a Redis context store |
| `OMNIA_TRACING_ENABLED` / `OMNIA_TRACING_ENDPOINT` / `OMNIA_TRACING_INSECURE` | Tracing config | Injected only when tracing is enabled on the operator |

Any `facades[].extraEnv` you declare on the facade entry is appended verbatim, so
you can pass your own configuration (for example `LOG_LEVEL=debug`) through the
CRD.

### Ports your container must bind

| Port | Purpose | Required |
|------|---------|----------|
| `OMNIA_FACADE_PORT` (default **8080**) | Your external protocol listener | Yes |
| **8081** (`OMNIA_HEALTH_PORT`) | `/healthz` + `/readyz` HTTP endpoints | Yes — the pod never becomes Ready without it |
| **18080** | Management-plane twin listener | Only if `managementPlane` is `true` (the default) — see [Step 6](#step-6-optional--serve-the-management-plane-twin) |

The runtime sidecar's gRPC server is reachable at `localhost:9000`
(`OMNIA_RUNTIME_ADDRESS`); you dial it, you do not bind it.

## Step 3 — Speak the runtime gRPC contract

Your facade is the gRPC **client**; the runtime sidecar is the server. Dial
`OMNIA_RUNTIME_ADDRESS` and speak the `omnia.runtime.v1.RuntimeService` service
defined in `api/proto/runtime/v1/runtime.proto`.

Because a custom facade is an **agent-mode** surface, you use the streaming
`Converse` RPC (the unary `Invoke` RPC is for `mode: function` REST/MCP facades
only):

```protobuf
rpc Converse(stream ClientMessage) returns (stream ServerMessage);
```

For each conversation:

1. Open a `Converse` stream and send a `ClientMessage` carrying the user's turn
   (`session_id`, plus `content` or the multimodal `parts`). Attach identity
   metadata to the call — see [Step 4](#step-4--authenticate-and-propagate-identity).
2. Consume the `ServerMessage` stream. It is a `oneof`; handle each variant:
   - `chunk` — a partial streaming text fragment; append/forward to your client.
   - `tool_call` — a **client-side** tool invocation (`execution == TOOL_EXECUTION_CLIENT`).
     Forward it to your client, collect the result, and send it back on the same
     stream as a `ClientToolResult` inside a `ClientMessage`. Server-side tool
     calls are handled internally by the runtime and never appear on this stream.
   - `done` — turn complete, with `final_content`/`parts` and token `usage`.
   - `error` — a machine-readable `code` and human `message`.
   - `media_chunk` — progressive binary media (raw bytes, no base64).
   - `interruption` — a barge-in signal for duplex audio; clear any buffered
     audio on your client.
3. For bidirectional audio, set `duplex_start` (a `DuplexStart`) on the **first**
   `ClientMessage` of the stream to switch it into duplex mode, then send inbound
   frames as `audio_input`.

The runtime is schema-agnostic and stateful by `session_id`; your facade owns the
external transport and turn framing.

## Step 4 — Authenticate and propagate identity

**Your facade is responsible for authenticating its own protocol.** Omnia does
not sit in front of your external port; whatever bearer token, session cookie,
mTLS peer, or signed webhook your protocol uses, your container validates it.

Once you have authenticated the caller, tell the runtime who they are by
attaching **flat `x-omnia-*` gRPC metadata** to every `Converse` call. This is
the identity contract the runtime rehydrates and forwards to the ToolPolicy
decision broker — without it, `identity.*` CEL rules see empty values. The keys
below are copied verbatim from `pkg/policy/context.go` (`headerKeyMap`); emit a
key **only when its value is non-empty**. gRPC metadata keys are lowercase.

| Metadata key | Meaning |
|--------------|---------|
| `x-omnia-agent-name` | AgentRuntime name (from `OMNIA_AGENT_NAME`) |
| `x-omnia-namespace` | Pod namespace (from `OMNIA_NAMESPACE`) |
| `x-omnia-session-id` | Your conversation/session identifier |
| `x-omnia-request-id` | Per-request trace identifier |
| `x-omnia-user-id` | Authenticated (pseudonymised) caller identity |
| `x-omnia-user-roles` | The caller's role — a **single** role string despite the plural name |
| `x-omnia-user-email` | The caller's email, if known |
| `x-omnia-origin` | A label naming your validator — surfaces as `identity.origin` |
| `x-omnia-workspace` | Workspace the request targets — surfaces as `identity.workspace` |
| `x-omnia-claim-<name>` | One entry per mapped identity claim, value verbatim |
| `x-omnia-consent-grants` | Optional comma-separated per-request consent grants |
| `x-omnia-consent-layer` | Optional diagnostic label for the consent grants |

The `x-omnia-provider` and `x-omnia-model` keys are set by the **runtime**, not by
your facade — do not emit them yourself.

:::danger[Never forward the raw `Authorization` token]
The raw inbound bearer token is **withheld by design**. Omnia deliberately keeps
`Authorization` out of the propagation map: re-emitting the caller's credential
outbound would leak it to arbitrary third-party tool upstreams and could clobber
a tool's own configured credential. Identity travels safely via the
`x-omnia-user-*` and `x-omnia-claim-*` metadata above — **not** as a forwarded
`Authorization` header. Keep the raw token inside your own process if you need it;
never attach it to the runtime gRPC call.
:::

See [Facade ↔ runtime protocol](/reference/platform/facade-runtime-protocol/) for
how each key surfaces into ToolPolicy CEL (`identity.subject`, `identity.role`,
`identity.claims`, `identity.workspace`, `identity.origin`), and
[Configure tool policies](/how-to/security/configure-tool-policies/) for writing
rules against them.

## Step 6 (optional) — Serve the management-plane twin

`managementPlane` defaults to **`true`**. When it is enabled, the operator
allocates an internal twin listener on port **18080**, adds a ClusterIP-only
`facade-mgmt` Service port, and advertises it on the AgentRuntime's
`status.managementEndpoints.ws`. This is how the dashboard's "Try this agent"
debug view and other in-cluster control-plane callers reach your agent.

If you enable the management plane, your container **must**:

- Bind a **second listener on port 18080**, separate from your external port.
- Accept **only dashboard-minted RS256 JWTs** on it, verified against the JWKS at
  `OMNIA_MGMT_PLANE_JWKS_URL` (fetch the signing keys on demand and refresh on
  rotation).
- **Fail closed** — reject any request on the twin that lacks a valid
  management-plane JWT.
- Keep it internal; the operator never routes external traffic to 18080.

If you cannot or do not want to implement the twin, set
**`managementPlane: false`** on the facade entry. The operator then allocates no
internal port, no `facade-mgmt` Service port, and no `status.managementEndpoints`
entry — but the dashboard's "Try this agent" view will not work against your
agent.

## Step 5 — Serve readiness and liveness

The operator gives the custom facade container the same HTTP probes as the
built-in facade, both pointed at the health port **8081** (`OMNIA_HEALTH_PORT`):

- **Readiness:** `GET /readyz` on `:8081`
- **Liveness:** `GET /healthz` on `:8081`

Your container **must** serve both endpoints on port 8081 and return `2xx` when
healthy, or the pod's readiness probe never succeeds, the pod never joins the
Service endpoints, and no traffic reaches it. This is the single most common
reason a custom facade "deploys but never works". Serve `/readyz` only once your
gRPC connection to the runtime and your external listener are both up.

## Exposure is handled for you

You do **not** implement ingress. When you set `expose.enabled: true` on the
facade entry (and the platform has a default-exposure Gateway configured), the
operator provisions a host-based `HTTPRoute` that targets your agent's facade
Service on the facade port. Exposure does not add authentication — that remains
your facade's responsibility per [Step 4](#step-4--authenticate-and-propagate-identity).
See [`facades[].expose`](/reference/core/agentruntime/) in the CRD reference.

## Conformance checklist

A conformant custom facade:

- [ ] Declares exactly one `type: custom` facade with a required `image`, in
      `mode: agent`.
- [ ] Reads `OMNIA_FACADE_PORT`, `OMNIA_HEALTH_PORT`, and `OMNIA_RUNTIME_ADDRESS`
      from the environment rather than hard-coding them.
- [ ] Dials the runtime at `OMNIA_RUNTIME_ADDRESS` and drives the `Converse`
      stream, handling every `ServerMessage` variant and round-tripping
      `ClientToolResult` for client-side tool calls.
- [ ] Authenticates its own external protocol.
- [ ] Emits the `x-omnia-*` identity metadata on every runtime gRPC call and
      **never** forwards the raw `Authorization` token.
- [ ] Serves `/readyz` and `/healthz` on port **8081**.
- [ ] Either serves the management-plane twin on port **18080** (verifying
      dashboard JWTs against `OMNIA_MGMT_PLANE_JWKS_URL`, failing closed) or sets
      `managementPlane: false`.

Work from `examples/custom-facade/` in the repository — it satisfies every item
above.

## See also

- [Facade ↔ runtime protocol](/reference/platform/facade-runtime-protocol/) — the
  gRPC message surface and full `x-omnia-*` metadata contract.
- [AgentRuntime CRD reference](/reference/core/agentruntime/) — the `type: custom`
  facade field, `image`, `managementPlane`, and `expose`.
- [Configure agent authentication](/how-to/security/configure-authentication/) —
  how the built-in facade authenticates, for comparison.
- [Configure tool policies](/how-to/security/configure-tool-policies/) — writing
  CEL rules against the identity your facade propagates.
