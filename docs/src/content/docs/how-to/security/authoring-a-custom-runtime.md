---
title: "Authoring a custom runtime"
description: "Build a bring-your-own-container runtime that serves the omnia.runtime.v1 gRPC contract, advertises its capabilities, and passes the conformance suite"
sidebar:
  order: 26
---

A **custom runtime** lets you replace Omnia's built-in PromptKit runtime
container with your own image while keeping the rest of the platform — the
facade sidecar, the policy broker, sessions, exposure, and identity-aware policy
enforcement — unchanged. Use it when you need an orchestration framework Omnia
doesn't ship (a LangChain/LangGraph app, a bespoke agent loop, a house
inference stack) but still want Omnia's protocol translation, tool execution
surface, and policy enforcement in front of it.

Your container is the **runtime**: it serves the
`omnia.runtime.v1.RuntimeService` gRPC service on the pod-internal runtime port,
and the facade sidecar dials it for every turn. Unlike a
[custom facade](/how-to/security/authoring-a-custom-facade/) — which is
Enterprise-licensed — a custom runtime is **not** license-gated; any AgentRuntime
may declare one.

This guide is the end-to-end contract your container must honour. For the
underlying message surface and identity metadata, see
[Facade ↔ runtime protocol](/reference/platform/facade-runtime-protocol/).

:::tip[Start from the reference example]
A minimal, conformant custom runtime lives at
`examples/custom-runtime/av-preprocessor/` in the Omnia repository — it serves
every RPC below, advertises its capabilities honestly, and passes the
conformance suite. `examples/custom-runtime/` also documents running a runtime
offline against a directory of manifests (`OMNIA_CONFIG_DIR`). Read them
alongside this page.
:::

## Prerequisites

- A container image you control, published to a registry the cluster can pull.
- A gRPC server implementation of `omnia.runtime.v1.RuntimeService` (any
  language). Generate your stubs from `api/proto/runtime/v1/runtime.proto` at a
  **pinned git ref** — do not hand-copy the file.

## Step 1 — Declare the custom runtime

Set `spec.framework.type: custom` and point `image` at your container. Only
`promptkit` has a built-in image; `custom` (and `langchain`) **must** supply one
explicitly, or the AgentRuntime blocks with `FrameworkImageUnavailable` rather
than silently running PromptKit:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: my-agent
spec:
  promptPackRef:
    name: my-pack
  framework:
    type: custom
    image: ghcr.io/acme/my-runtime:v1.0.0
```

Alternatively an operator can register a default image for a type cluster-wide
with a repeatable `--framework-image=custom=ghcr.io/acme/my-runtime:v1.0.0`
flag, so individual AgentRuntimes need not repeat it.

## Step 2 — Environment and ports

The operator injects the runtime container's wiring; your container reads these
rather than the AgentRuntime CRD. The core set:

| Variable | Value | Notes |
|----------|-------|-------|
| `OMNIA_AGENT_NAME` | AgentRuntime name | From the Downward API |
| `OMNIA_NAMESPACE` | Pod namespace | From the Downward API |
| `OMNIA_GRPC_PORT` | gRPC listen port | Defaults to **9000** — serve `RuntimeService` here |
| `OMNIA_HEALTH_PORT` | Health/metrics port | Defaults to **9001** |
| `OMNIA_PROMPTPACK_PATH` | Path to the compiled PromptPack | The operator mounts the resolved pack here |

The facade dials your gRPC server at `localhost:9000` (`OMNIA_GRPC_PORT`). The
platform-input contract (PromptPack, ToolRegistry, skills, providers) and a
first-class config-source port are still being finalised in later waves of the
custom-runtime epic; the reference example shows the offline devroot approach in
the meantime.

## Step 3 — Serve the `omnia.runtime.v1` gRPC contract

Implement the `RuntimeService` methods:

- **`Health(HealthRequest) → HealthResponse`** — report `healthy`, the
  `contract_version` you built against, and your `capabilities` (Step 4).
- **`Converse(stream ClientMessage) → stream ServerMessage`** — the bidirectional
  turn stream. Handle every `ClientMessage` field you may receive (it is **not**
  a `oneof` — several may be set at once); never drop a message part silently.
  Your **first** `ServerMessage` on the stream must be a `RuntimeHello` (Step 4).
- **`Invoke(InvocationRequest) → InvocationResponse`** — one-shot function mode
  (`spec.mode: function`). If you only serve `spec.mode: agent`, leave it
  `Unimplemented` **and** do not advertise the `invoke` capability.
- **`HasConversation(HasConversationRequest) → HasConversationResponse`** —
  report whether a named session's working context can still be resumed
  (`RESUMABLE` / `NOT_FOUND` / `UNAVAILABLE`).

Read caller identity from the flat `x-omnia-*` gRPC metadata (see the
[protocol reference](/reference/platform/facade-runtime-protocol/#identity--claims-metadata));
the raw bearer token is deliberately withheld. Never forward the caller's
credentials to third-party tool upstreams.

## Step 4 — Advertise capabilities and negotiate

Return the optional surfaces you actually implement in
`HealthResponse.capabilities` — and send a `RuntimeHello` as your first
`ServerMessage` carrying the same set. The known names are listed in the
[Capabilities reference](/reference/platform/facade-runtime-protocol/#capabilities);
the set is **open**, so advertise names outside it freely if you implement new
behaviour.

Advertisement must be **honest**: the conformance suite fails a runtime that
advertises `invoke` or `duplex_audio` but then returns `Unimplemented`, and the
operator will not schedule a runtime that claims less than the AgentRuntime
requires.

For a duplex session, your `RuntimeHello` may carry a `MediaNegotiation`
counter-offer (the audio format you require); the facade relays it to the client
as a `session_config` message or fails the session closed if it cannot be met.
See
[Per-session negotiation](/reference/platform/facade-runtime-protocol/#per-session-negotiation).

## Step 5 — Verify with the conformance suite

Omnia ships a protocol conformance suite. Build the CLI and point it at your
running runtime:

```sh
go build -o runtime-conformance ./cmd/runtime-conformance
./runtime-conformance --addr localhost:9000
```

It checks, protocol-only and language-agnostically:

- `Health` is healthy and `contract_version` is semver;
- the first `Converse` frame is a `RuntimeHello` whose capabilities match
  `Health`;
- a text turn ends with `done`, with no `done` before the hello;
- an empty/malformed `ClientMessage` is answered on-protocol, never a crash;
- **capability honesty** — an advertised `invoke`/`duplex_audio` works; an
  unadvertised one returns `Unimplemented`.

A non-zero exit means non-conformant, with a per-check table naming what failed.
This is exactly how to measure *any* runtime's gap against the contract —
including an unsupported LangChain container's — before you rely on it.

## See also

- [Facade ↔ runtime protocol](/reference/platform/facade-runtime-protocol/) — the message surface, capability table, and identity metadata.
- `examples/custom-runtime/av-preprocessor/` — a minimal conformant runtime with an A/V-preprocessing seam.
- [Authoring a custom facade](/how-to/security/authoring-a-custom-facade/) — the mirror image (BYO facade, Enterprise-gated).
