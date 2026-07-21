---
title: "Facade ↔ runtime protocol"
description: "gRPC message surface and identity/claims metadata contract between the facade and runtime containers"
sidebar:
  order: 7
---


Every Omnia agent pod runs two sidecars: the **facade** (external protocol
translation — WebSocket, REST, A2A, MCP) and the **runtime** (LLM calls, tool
execution, sessions). They talk to each other over a private gRPC connection.
This page is the authoritative reference for two things:

1. the **gRPC message surface** the facade and runtime exchange
   (`api/proto/runtime/v1/runtime.proto`), and
2. the **flat `x-omnia-*` identity/claims metadata** the facade attaches to
   every gRPC call so that the runtime — and the ToolPolicy decision broker it
   calls — can make identity-aware decisions.

For the client-facing WebSocket protocol (browser ↔ facade), see the
[WebSocket protocol reference](/reference/platform/websocket-protocol/). For
*why* the policy engine is shaped the way it is, see
[Policy engine architecture](/explanation/security/policy-engine/).

## Contract version

The contract is versioned. The current version is **1.0.0**, declared in two
places that are asserted equal by `pkg/runtime/contract/version_test.go`:

- the `// Contract-Version:` marker at the top of
  `api/proto/runtime/v1/runtime.proto`
- the `contract.Version` constant in `pkg/runtime/contract/version.go`

The minor version is bumped for additive changes — a new message, a new
optional field, a new `oneof` variant. The major version is bumped for any
change that would break an existing conformant runtime.

## Implementing this contract in your own runtime

Any container that serves `omnia.runtime.v1.RuntimeService` can be an Omnia
runtime: set `spec.framework.type: custom` and `spec.framework.image` on the
AgentRuntime. Only `promptkit` has a built-in image; every other framework type
must supply one explicitly, and blocks with `FrameworkImageUnavailable` if it
does not.

:::caution[Do not hand-copy this proto file]
Published stubs are not available yet — stub distribution lands with a later
wave of the custom-runtime epic. Until then, generate your client from
`api/proto/runtime/v1/runtime.proto` at a **pinned git ref**, record the
`Contract-Version` marker you generated against, and report it back from your
`Health` RPC via `contract_version`. A hand-copied, unpinned `.proto` cannot
tell you when it has diverged: an unsupported LangChain runtime drifted six
months and seven features behind the contract this way — silently dropping
`audio_input`, `client_tool_result`, and `consent_grants` because the
generated types for them did not exist in its copy.
:::

A conformant runtime must:

1. Handle every `ClientMessage` field it may receive, or fail loudly on the ones
   it does not — never drop a message part silently. Note that `ClientMessage`
   is not a `oneof`: several fields may be set on the same message.
2. Emit `ServerMessage` variants for the surfaces it advertises.
3. Serve `Invoke` for function-mode AgentRuntimes (`spec.mode: function`); a
   runtime that only serves `spec.mode: agent` may omit it.
4. Read caller identity from the flat `x-omnia-*` gRPC metadata below —
   `context.invocation_metadata()` or your language's equivalent. The raw
   `Authorization` bearer token is deliberately withheld.
5. Serve `Health`, and report the contract version it was built against.
6. Never forward the caller's credentials to third-party tool upstreams.

Full authoring guidance, the platform-input contract (PromptPack, ToolRegistry,
skills, providers), and the conformance suite land with later waves of the
custom-runtime epic.

## gRPC service surface

The contract is defined in `api/proto/runtime/v1/runtime.proto`
(package `omnia.runtime.v1`). The facade is the gRPC client; the runtime is the
server.

```protobuf
service RuntimeService {
  // Bidirectional streaming for agent conversations.
  rpc Converse(stream ClientMessage) returns (stream ServerMessage);

  // One-shot, non-conversational Function call (mode: function).
  rpc Invoke(InvocationRequest) returns (InvocationResponse);

  // Readiness probe.
  rpc Health(HealthRequest) returns (HealthResponse);
}
```

| RPC | Shape | Used by |
|-----|-------|---------|
| `Converse` | bidi stream `ClientMessage` → `ServerMessage` | `mode: agent` runtimes (WebSocket / A2A facades) |
| `Invoke` | unary `InvocationRequest` → `InvocationResponse` | `mode: function` runtimes (REST / MCP facades, `POST /functions/{name}`) |
| `Health` | unary `HealthRequest` → `HealthResponse` | readiness checks |

### `ClientMessage` (facade → runtime)

| Field | Type | Description |
|-------|------|-------------|
| `session_id` | string | Conversation session for state management |
| `content` | string | User message text (legacy; use `parts` for multimodal) |
| `metadata` | map<string,string> | Optional key-value context |
| `parts` | repeated `ContentPart` | Multimodal content (text, image, audio, video, file); takes precedence over `content` |
| `client_tool_result` | `ClientToolResult` | Result of a client-side tool execution |
| `consent_grants` | repeated string | Per-message consent category grants that override stored consent for this request |
| `duplex_start` | `DuplexStart` | On the first message, switches the stream into bidirectional audio mode |
| `audio_input` | `AudioInputChunk` | One inbound audio frame during a duplex session |

### `ServerMessage` (runtime → facade)

A `oneof` — exactly one variant per message:

| Variant | Type | Description |
|---------|------|-------------|
| `chunk` | `Chunk` | Partial streaming text |
| `tool_call` | `ToolCall` | **Client-side** tool invocation the facade must forward to the browser. Server-side tool calls are handled internally by the runtime and are **not** sent on this stream |
| `done` | `Done` | Turn completion, with `final_content`/`parts` and `usage` |
| `error` | `Error` | Error with machine-readable `code` and human `message` |
| `media_chunk` | `MediaChunk` | Progressive media delivery (raw bytes, no base64) |
| `interruption` | `Interruption` | Barge-in signal; client clears buffered audio |

`ToolCall` carries `execution` (`TOOL_EXECUTION_SERVER` / `TOOL_EXECUTION_CLIENT`);
only `CLIENT` calls require the facade to round-trip a `ClientToolResult` back to
the runtime.

### `Invoke` (function mode)

`InvocationRequest` carries `input_json` (already validated by the facade
against `spec.inputSchema`), an `invocation_id` for correlation, and a
`metadata` map. `InvocationResponse` returns `output_json`, `usage`, and
`duration_ms`. The runtime is schema-agnostic — the facade validates
input and output.

:::note[Identity does not travel as a message field]
None of the messages above carry a user-identity field. Caller identity and
claims travel as **gRPC metadata** on the call, described next — not inside
`ClientMessage`, `InvocationRequest`, or their `metadata` maps.
:::

## Identity & claims metadata

The facade builds a set of propagation fields once per connection
(`internal/facade/server.go`, `buildConnectionContext`) and attaches them as
gRPC metadata on every call via `policy.ToGRPCMetadata`
(`pkg/policy/context.go`). The runtime rehydrates them from incoming metadata in
a gRPC interceptor (`internal/runtime/interceptor.go`,
`extractPolicyFromMetadata`) and exposes them to tool adapters and the ToolPolicy
broker.

The exact set of keys is the single source of truth `headerKeyMap` in
`pkg/policy/context.go`. Every key below is emitted **only when its value is
non-empty**.

### Flat metadata keys

| Metadata key | Populated by | Description |
|--------------|--------------|-------------|
| `x-omnia-agent-name` | Facade | Name of the AgentRuntime |
| `x-omnia-namespace` | Facade | Kubernetes namespace of the agent pod |
| `x-omnia-session-id` | Facade | Current session identifier |
| `x-omnia-request-id` | Facade | Per-request trace identifier |
| `x-omnia-user-id` | Facade | Authenticated caller identity (pseudonymised end-user id) |
| `x-omnia-user-email` | Facade | The caller's email address |
| `x-omnia-provider` | Runtime | LLM provider type |
| `x-omnia-model` | Runtime | LLM model name |
| `x-omnia-origin` | Facade | Validator that admitted the request — surfaces as `identity.origin` (added in #1769) |
| `x-omnia-workspace` | Facade | Workspace the request targets — surfaces as `identity.workspace` (added in #1769) |
| `x-omnia-claim-<name>` | Facade | One entry per mapped JWT claim (see [claim propagation](#claim-propagation)) |
| `x-omnia-consent-grants` | Facade | Comma-separated per-request consent category grants |
| `x-omnia-consent-layer` | Facade | Diagnostic label attributing the grants to a layer (per-message / session / persistent) |

On the subsequent runtime → tool and runtime → policy-broker HTTP hops, the
runtime adds tool-context headers via `SetAllOutboundHeaders`
(`internal/runtime/tools/context_headers.go`):

| Header | Populated by | Description |
|--------|--------------|-------------|
| `x-omnia-tool-name` | Runtime | Tool being invoked |
| `x-omnia-tool-registry` | Runtime | ToolRegistry containing the tool |
| `x-omnia-param-<PascalCase>` | Runtime | Promoted scalar tool parameters |

:::danger[The raw `Authorization` bearer token is withheld by design]
`Authorization` is **deliberately absent** from `headerKeyMap` and is **never**
propagated outbound. The reasoning, quoted from the code
(`pkg/policy/context.go`):

> The caller's inbound bearer token must never be re-emitted as the outbound
> `Authorization` on a tool call: it leaks the user's credential to arbitrary
> third-party upstreams, and it would overwrite a tool's own `authSecretRef`
> credential (the runtime applies that first in `buildHTTPHeaders`, and this map
> used to clobber it). User identity travels safely via the `X-Omnia-Claim-*`
> headers instead. The raw token stays available in-process (`Authorization(ctx)`
> / `ContextKeyAuthorization`) for a future on-behalf-of token exchange — it is
> simply never sent to a tool.

The token is retained in the facade's in-process context, but it does not cross
the gRPC hop as a propagated credential and is never attached to a tool call.
:::

### Claim propagation

Every mapped JWT claim crosses as its own metadata entry:
`x-omnia-claim-<name>`, one per claim, value verbatim. The facade sources these
from the admitting validator's claim map (`AuthenticatedIdentity.Claims`),
regardless of which validator (OIDC, edge-trust, API key, shared token,
management plane) admitted the request.

**Casing matters.** Claim headers are lowercase on the gRPC hop
(`x-omnia-claim-<name>`, because gRPC metadata keys are lowercase). But when the
runtime forwards them to the policy-broker or a downstream HTTP tool, it emits
them through an `http.Request`, so they land **MIME-canonicalized** on the wire:
claim `tier` becomes `X-Omnia-Claim-Tier`, `customer_id` becomes
`X-Omnia-Claim-Customer_id`. Only the first letter of each hyphen-separated
segment is upper-cased — the claim-name segment itself is **not** otherwise
transformed.

Any read path — ToolPolicy `requiredClaims` lookups and CEL rules that reference
`headers['X-Omnia-Claim-*']` — **must** use the canonical form. The single
source of truth is `policy.CanonicalClaimHeader(claim)`; referencing the raw
lowercase prefix silently misses. This is the #1766 canonical-header rule; see
[Configure Tool Policies](/how-to/security/configure-tool-policies/) for how CEL
rules reference claims.

### Role as a claim

Roles are not a structured field. There is no dedicated role header or
identity field — a role rides like any other claim, via
`x-omnia-claim-role`, if the admitting validator's claim map includes a
`role` entry. On the runtime/broker side it surfaces as
`identity.claims.role`, not as a separate `identity.role` field. See
[claim propagation](#claim-propagation) above for how claim headers are
named and canonicalized.

### `identity.workspace` semantics

`x-omnia-workspace` (and therefore the CEL `identity.workspace`) resolves in the
facade via `identityScope` (`internal/facade/identity_scope.go`):

1. **Token workspace scope** — if the admitting validator produced a
   workspace-scoped identity (e.g. a management-plane JWT carrying a `workspace`
   claim), that value is used.
2. **Agent's deployed workspace** — otherwise it falls back to the workspace the
   AgentRuntime is deployed into.

This guarantees `identity.workspace` is non-empty for every validator style.
Note it is distinct from `x-omnia-namespace` (the Kubernetes namespace), which is
propagated separately.

## How identity reaches ToolPolicy CEL

The runtime never has the facade's structured `AuthenticatedIdentity` — that
object is in-process on the facade side only and does **not** cross the gRPC hop
(see the `ContextKeyIdentity` doc in `pkg/policy/context.go`). On the runtime
side, the flat metadata above is all that survives.

When the runtime calls the policy-broker
(`internal/runtime/tools/policy_broker_client.go`), it rebuilds a structured
identity from those flat fields via `policy.IdentityPayloadFromPropagation` and
sends it as the `identity` object on the decision request
(`pkg/policy/broker_contract.go`). The broker exposes it to CEL rules as the
`identity` root (`ee/pkg/policy/evaluator.go`):

| CEL field | Reconstructed from | Notes |
|-----------|--------------------|-------|
| `identity.origin` | `x-omnia-origin` | Validator that admitted the request (added in #1769) |
| `identity.workspace` | `x-omnia-workspace` | Token scope else agent's deployed workspace (added in #1769) |
| `identity.subject` | `x-omnia-user-id` | The wire carries a single pseudonymised caller id |
| `identity.endUser` | `x-omnia-user-id` | Collapses onto the same id — there is no separate propagated actor value |
| `identity.agent` | `x-omnia-agent-name` | The agent this tool call runs under |
| `identity.claims` | `x-omnia-claim-*` | The claim map, verbatim |

When no identity is attached (unauthenticated / dev-mode traffic), the `identity`
root is populated with zero-valued strings and an empty `claims` map, so rules
referencing `identity.*` evaluate without error. Use
`has(identity.claims.<name>)` to test whether a claim is present.

## Source of truth

| Location | Defines |
|----------|---------|
| `api/proto/runtime/v1/runtime.proto` | gRPC message surface (facade ↔ runtime) |
| `pkg/policy/context.go` | `headerKeyMap`, header constants, `ToOutboundHeaders` / `ToGRPCMetadata`, `CanonicalClaimHeader` |
| `internal/runtime/interceptor.go` | Rehydration of metadata into runtime request context |
| `pkg/policy/broker_contract.go` | `DecisionRequest` / `IdentityPayload` wire contract, `IdentityPayloadFromPropagation` |
| `ee/pkg/policy/evaluator.go` | How `identity.*` surfaces into ToolPolicy CEL |
