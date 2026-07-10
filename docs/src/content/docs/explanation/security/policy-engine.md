---
title: "Policy engine architecture"
description: "Understanding how AgentPolicy and ToolPolicy enforce guardrails for AI agents"
sidebar:
  order: 7
---


Omnia's policy engine provides guardrails for AI agents at two distinct enforcement layers. This document explains *why* policies exist, how the two policy types differ, and how context flows through the system to enable fine-grained access control.

## Why policies?

AI agents can invoke tools, call LLM providers, and act on behalf of users. Without guardrails, an agent could:

- Call tools it shouldn't have access to
- Exceed cost or usage limits
- Act without knowing *who* the end user is
- Bypass compliance requirements

Policies solve this by introducing **declarative, Kubernetes-native access control** that operators configure once and the platform enforces automatically.

## Two policy types

Omnia separates policy enforcement into two layers, each with a distinct enforcement mechanism:

```mermaid
graph TB
    subgraph "Network Layer"
        AP[AgentPolicy] -->|Generates| IAP[Istio AuthorizationPolicy]
        IAP -->|"Enforces at (L7 header match — waypoint only)"| WAYPOINT[Waypoint Proxy<br/>if enrolled]
    end

    subgraph "Application Layer"
        TP[ToolPolicy] -->|Evaluates via| BROKER[Policy Broker Sidecar<br/>PDP]
        RUNTIME[Runtime<br/>PEP] -->|Calls per tool call| BROKER
        BROKER -->|Decision| RUNTIME
    end

    CLIENT[Client Request] --> WAYPOINT
    WAYPOINT -->|Allowed| FACADE[Facade]
    FACADE -->|Tool Call| RUNTIME
    RUNTIME -->|Allowed| UPSTREAM[Tool Service]
```

### AgentPolicy (network-level)

AgentPolicy operates at the **Istio AuthorizationPolicy level**. The operator controller translates each AgentPolicy's `toolAccess` into a live Istio `AuthorizationPolicy` CR (allowlist mode: an `ALLOW` for the listed tools plus a catch-all `DENY`; denylist mode: a `DENY` for the listed tools; `permissive` mode maps to `AUDIT` instead), matching on the `X-Omnia-Tool-Name` request header. This provides:

- **Tool allowlist/denylist** — restrict which tool registries and tools an agent can invoke
- **Enforcement modes** — `enforce` blocks violations; `permissive` audits without blocking

JWT claim mapping is configured separately, on the AgentRuntime — see [JWT claim extraction](#jwt-claim-extraction) below.

:::caution[Ambient mode requires a waypoint]
Omnia runs Istio in **ambient** mode (see the ToolPolicy section below): ztunnel enforces L4 (mTLS, principal) only. `AuthorizationPolicy` rules that match on `request.headers[...]` — which is how `toolAccess` is implemented — are **L7** and only take effect when the target agent's Service is enrolled behind a **waypoint proxy**. The operator always creates the `AuthorizationPolicy` object; nothing currently provisions a waypoint on AgentPolicy's behalf. If the agent isn't otherwise enrolled behind a waypoint (e.g. via canary-rollout mesh traffic routing), the rule is inert and every tool call is allowed through regardless of `toolAccess`. Confirm waypoint enrollment for any agent whose tool-access control matters.
:::

### ToolPolicy (application-level, Enterprise)

ToolPolicy operates at the **application level** as a *called decision broker*, not a reverse proxy in the request path. The **runtime is the enforcement point (PEP)**: its `OmniaExecutor.dispatch` — the single chokepoint all four tool-executor types (HTTP, OpenAPI, gRPC, MCP) funnel through — calls the **policy-broker** sidecar over `POLICY_BROKER_URL` (localhost, `POST /v1/decision`) once per server-executed tool call, before the tool actually runs. The **policy-broker is the decision point (PDP)**: it watches ToolPolicy CRDs and evaluates CEL rules against the request headers, body, and caller identity, then returns a decision. It provides:

- **CEL deny rules** — evaluate request headers, body, and identity using [Common Expression Language](https://github.com/google/cel-go) expressions
- **Required claims** — verify that specific JWT claims are present before allowing the request
- **Header injection** — obligations returned alongside the allow/deny decision; the runtime attaches them to the outbound tool call only when the request is allowed
- **Fail-closed by default** — if the broker is unreachable, the runtime denies the call (a deployment can opt into fail-open instead)
- **Audit logging** — structured logs for deny and audit-mode would-deny decisions (see [Audit logging](#audit-logging) below)

This shape exists because Omnia runs Istio in **ambient** mode, which has no waypoint proxy on tool egress — a reverse proxy sitting passively in the network path would never see traffic routed to it. Calling the broker directly sidesteps transparent interception entirely.

ToolPolicy is an [Enterprise](/explanation/platform/licensing/) feature.

## Context propagation

For policies to make decisions based on *who* is calling and *what* they're calling, identity and request context must flow through every service boundary:

```mermaid
sequenceDiagram
    participant Client
    participant Istio as Istio (Gateway/Waypoint)
    participant Facade
    participant Runtime
    participant Broker as Policy Broker
    participant Tool as Tool Service

    Client->>Istio: WebSocket + JWT
    Istio->>Facade: Forward request (+ x-user-* fallback headers)
    Facade->>Facade: Auth validator verifies JWT, builds AuthenticatedIdentity (origin, workspace, claims)
    Facade->>Runtime: gRPC + x-omnia-* metadata (identity + claims; Authorization withheld)
    Runtime->>Broker: POST /v1/decision (headers + body + reconstructed identity)
    Broker->>Broker: Evaluate CEL rules against headers + body + identity
    Broker->>Runtime: {allow, deniedBy, message, mode, wouldDeny, injectedHeaders}
    Runtime->>Tool: Forward (if allowed) + injected headers
```

### Propagated headers

The following headers are propagated across service boundaries:

| Header | Source | Description |
|--------|--------|-------------|
| `x-omnia-agent-name` | Facade | Name of the AgentRuntime |
| `x-omnia-namespace` | Facade | Kubernetes namespace |
| `x-omnia-session-id` | Facade | Current session identifier |
| `x-omnia-request-id` | Facade | Per-request trace identifier |
| `x-omnia-user-id` | Facade | Authenticated user identity (the facade populates this from its auth chain — or, when the chart's authentication gate is off, from the Istio-injected `x-user-id` header) |
| `x-omnia-user-email` | Facade | User email address |
| `x-omnia-origin` | Facade | Validator that admitted the request — surfaces as `identity.origin` (#1769) |
| `x-omnia-workspace` | Facade | Workspace the request targets — surfaces as `identity.workspace` (#1769) |
| `x-omnia-provider` | Runtime | LLM provider type |
| `x-omnia-model` | Runtime | LLM model name |
| `x-omnia-tool-name` | Runtime | Tool being invoked |
| `x-omnia-tool-registry` | Runtime | ToolRegistry containing the tool |
| `x-omnia-claim-*` | Facade | Mapped JWT claims (e.g., `x-omnia-claim-team`) |
| `x-omnia-param-*` | Runtime | Promoted scalar tool parameters |

> **The `x-omnia-user-*` headers are facade-populated, not Istio-injected.** The facade's auth validator produces an `AuthenticatedIdentity` and the facade emits `x-omnia-user-id` / `-email` from it (`pkg/policy/context.go`). Istio's own `x-user-id` header is only a *fallback source* the facade reads when the chart's `authentication.enabled` gate is off — the outbound `x-omnia-*` metadata always originates at the facade.

> **The raw `Authorization` bearer token is withheld by design.** It is deliberately **not** propagated outbound: re-emitting the caller's inbound token as the outbound `Authorization` on a tool call would leak the user's credential to arbitrary upstreams and clobber a tool's own `authSecretRef` credential. The token stays in the facade's in-process context (for a future on-behalf-of exchange) but never crosses to a tool. User identity travels safely via the `x-omnia-claim-*` headers instead.

In addition to these flattened headers, the runtime reconstructs the caller's identity as a **structured JSON object** (`origin`, `subject`, `endUser`, `workspace`, `agent`, `claims`) and sends it on every decision request, so `identity.*` CEL expressions can address identity directly. Roles are not a separate identity field — they arrive as an ordinary entry in `claims` (`identity.claims.role`), sourced from the edge's role header, the IdP's own role claim, or the API key's stamped role, depending on which validator admitted the request. The facade's in-process `AuthenticatedIdentity` does **not** cross the facade → runtime gRPC hop; instead the runtime rebuilds this object from the flat metadata above (`IdentityPayloadFromPropagation`). Post-#1769, `x-omnia-origin` and `x-omnia-workspace` are propagated as their own headers, so `identity.origin` and `identity.workspace` are now populated on the runtime side (before #1769 they were not carried across the hop and those two fields were always empty). See the [Facade ↔ runtime protocol reference](/reference/platform/facade-runtime-protocol/) for the full metadata contract.

### JWT claim extraction

Claim forwarding is not an AgentPolicy concern — it's configured on the **AgentRuntime**'s external-auth block (`spec.externalAuth.oidc.claimMapping` for customer-IdP OIDC, or the edge-trust equivalent). The facade's auth validator extracts the configured claims from the verified JWT and forwards them as `X-Omnia-Claim-*` headers on every request. See [Configure Agent Authentication](/how-to/security/configure-authentication/) for the field reference.

AgentPolicy itself governs only tool allow/deny at the network level (see below) — it has no claim-mapping configuration. Once claims arrive as `X-Omnia-Claim-*` headers, ToolPolicy's `requiredClaims` and CEL rules can consume them.

## Enforcement modes

Both policy types support a mode that controls whether violations are blocked or only logged:

| Policy Type | Enforce Mode | Permissive/Audit Mode |
|-------------|-------------|----------------------|
| AgentPolicy | `enforce` — Istio blocks the request (requires a waypoint under ambient mode — see the caution above) | `permissive` — Istio maps to `AUDIT`: allows but logs |
| ToolPolicy | `enforce` — broker returns `allow: false`; runtime aborts the tool dispatch | `audit` — broker returns `allow: true` with `wouldDeny: true`; runtime proceeds and logs |

### Failure behavior

Both policy types also support `onFailure` to control what happens when policy evaluation itself fails (e.g., a CEL expression error):

- `deny` (default) — treat evaluation failures as denials
- `allow` — permit the request despite the error

## Audit logging

The policy-broker emits structured JSON logs for every deny decision and, when audit mode is active, for would-deny decisions. Two lines land per non-trivial decision: a shared `policy_decision` line (decision outcome, mode, matched policy/rule, message) and a broker-specific `broker_tool_decision` line carrying `toolName`/`toolRegistry` — since every decision request's path/method is the constant `/v1/decision` POST, tool identity travels in dedicated fields instead:

```json
{"msg":"policy_decision","allowed":true,"deniedBy":"max-refund-amount","message":"Refund amount exceeds $500 limit","mode":"audit","policy":"refund-limits"}
{"msg":"broker_tool_decision","toolName":"process_refund","toolRegistry":"customer-tools","allowed":true,"deniedBy":"max-refund-amount","mode":"audit"}
```

In `audit` mode a matched deny rule sets `deniedBy`/`message` but `allowed` stays `true` (the call proceeds); in `enforce` mode the same match produces `allowed: false`.

Logging is unconditional: the broker emits the `policy_decision`/`broker_tool_decision` pair for every deny and would-deny outcome (skipping only wholly-uninteresting allows). It does **not** redact `body`/`headers` values in those logs — keep sensitive data out of the fields your CEL rules read if broker-log exposure is a concern.

## Architecture: policy broker (PDP/PEP)

The policy-broker runs as a sidecar container in the **agent pod** (alongside facade and runtime), not in the tool service's pod. It never sits in the tool-call request path — it only answers decision requests the runtime makes:

```mermaid
graph LR
    subgraph "Agent Pod"
        RUNTIME[Runtime :PEP] -->|"POST /v1/decision"| BROKER[Policy Broker :8090<br/>PDP]
        BROKER -->|"{allow, deniedBy, injectedHeaders}"| RUNTIME
        RUNTIME -->|Allowed| TOOL[Tool Service]
    end

    subgraph "Control Plane"
        CTRL[Operator Controller] -->|Watch| TP[ToolPolicy CRD]
        BROKER -->|Watch| TP
    end
```

Per server-executed tool call:
1. The runtime's `OmniaExecutor.dispatch` calls the broker with the request headers, body, and structured identity
2. The broker checks required claims
3. The broker evaluates CEL deny rules in order (first match stops)
4. If allowed, the broker evaluates header injection rules and returns the computed headers
5. The runtime attaches any `injectedHeaders` to the outbound tool call and proceeds; on deny, it aborts the dispatch and surfaces a policy-denied error instead of calling the tool
6. If the broker is unreachable, the runtime **fails closed by default** (denies the call); this is configurable per deployment to fail open instead

## Related resources

- [AgentPolicy CRD Reference](/reference/policies/agentpolicy/) — field-by-field specification
- [ToolPolicy CRD Reference](/reference/policies/toolpolicy/) — field-by-field specification (Enterprise)
- [Configure Agent Policies](/how-to/security/configure-agent-policies/) — operational guide
- [Configure Tool Policies](/how-to/security/configure-tool-policies/) — operational guide (Enterprise)
- [Securing Agents with Policies](/tutorials/securing-agents/) — end-to-end tutorial
