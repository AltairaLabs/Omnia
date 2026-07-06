---
title: "ToolPolicy CRD"
description: "Complete reference for the ToolPolicy custom resource"
sidebar:
  order: 6
---

:::note[Enterprise]
ToolPolicy is an Enterprise feature. See [Licensing](/explanation/platform/licensing/) for details.
:::

The ToolPolicy custom resource defines CEL-based access control rules for tool invocations. Rules are evaluated by the **policy-broker** sidecar in the agent pod: the runtime calls it once per server-executed tool call (`POST /v1/decision`) before the tool runs, rather than a proxy intercepting the tool request itself. See [Policy Engine Architecture](/explanation/security/policy-engine/) for the full PDP/PEP model.

## API version

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ToolPolicy
```

## Spec fields

### `selector`

Defines which tools this policy applies to. The broker matches incoming decision requests based on the `X-Omnia-Tool-Registry` and `X-Omnia-Tool-Name` headers.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `registry` | string | Yes | Name of the ToolRegistry to match. |
| `tools` | []string | No | Specific tool names to match. If empty, applies to all tools in the registry. |

```yaml
spec:
  selector:
    registry: customer-tools
    tools:
      - process_refund
      - issue_credit
```

### `rules`

CEL-based deny rules evaluated in order. The first rule whose CEL expression evaluates to `true` denies the request. Minimum 1 rule is required.

Each rule has:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique identifier for the rule. |
| `description` | string | No | Human-readable description of the rule's purpose. |
| `deny.cel` | string | Yes | CEL expression that, when `true`, denies the request. |
| `deny.message` | string | Yes | Message returned to the caller when the rule denies. |

```yaml
spec:
  rules:
    - name: max-refund-amount
      description: "Prevent refunds over $500"
      deny:
        cel: 'double(body.amount) > 500.0'
        message: "Refund amount exceeds the $500 limit"

    - name: require-reason
      description: "All refunds must include a reason"
      deny:
        cel: '!has(body.reason) || body.reason == ""'
        message: "A reason is required for refund requests"
```

#### CEL variables

The following variables are available in CEL expressions:

| Variable | Type | Description |
|----------|------|-------------|
| `headers` | `map<string, string>` | All HTTP request headers (first value only for multi-value headers). |
| `body` | `map<string, dyn>` | Parsed JSON request body. Empty map if body is not JSON. |
| `identity` | `map<string, dyn>` | Structured caller identity (`origin`, `subject`, `endUser`, `workspace`, `agent`, `role`, `claims`) sent by the runtime alongside headers/body, so identity-aware rules don't depend on lossy header-flattening. |

#### CEL string extensions

The CEL environment includes the [cel-go string extensions](https://pkg.go.dev/github.com/google/cel-go/ext#Strings), providing functions like:

- `string.contains(substring)` — check if a string contains a substring
- `string.startsWith(prefix)` — check if a string starts with a prefix
- `string.endsWith(suffix)` — check if a string ends with a suffix
- `string.matches(regex)` — regex matching
- `string.lowerAscii()` — convert to lowercase
- `string.upperAscii()` — convert to uppercase
- `string.trim()` — trim whitespace
- `string.split(separator)` — split into a list

**Example using string extensions:**

```yaml
- name: block-external-urls
  deny:
    cel: 'has(body.url) && !body.url.startsWith("https://internal.")'
    message: "Only internal URLs are allowed"
```

### `requiredClaims`

Claims that must be present as `X-Omnia-Claim-*` headers. If a required claim header is missing, the request is denied before CEL rules are evaluated.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `claim` | string | Yes | Claim name (maps to `X-Omnia-Claim-<Claim>` header). |
| `message` | string | Yes | Error message returned when the claim is missing. |

```yaml
spec:
  requiredClaims:
    - claim: Team
      message: "Team claim is required — configure claim mapping on the AgentRuntime's external-auth block"
    - claim: Customer-Id
      message: "Customer ID claim is required for this tool"
```

:::tip
Required claims depend on the AgentRuntime's external-auth claim mapping (`spec.externalAuth.oidc.claimMapping` or the edge-trust equivalent) being configured for the same agent — see [Configure Agent Authentication](/how-to/security/configure-authentication/). The facade extracts claims from the JWT and forwards them as `X-Omnia-Claim-*` headers; the ToolPolicy verifies they are present. AgentPolicy is unrelated to claim mapping — it governs only tool allow/deny.
:::

### `headerInjection`

Headers to inject into the upstream request after policy evaluation passes. Each rule provides either a static `value` or a dynamic `cel` expression (mutually exclusive).

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `header` | string | Yes | HTTP header name to inject. |
| `value` | string | Conditional | Static header value. Mutually exclusive with `cel`. |
| `cel` | string | Conditional | CEL expression computing the header value. Mutually exclusive with `value`. |

```yaml
spec:
  headerInjection:
    # Static value
    - header: X-Policy-Version
      value: "v1"

    # Dynamic value from claims
    - header: X-Tenant-Id
      cel: 'headers["X-Omnia-Claim-Customer-Id"]'

    # Computed value
    - header: X-Request-Source
      cel: '"policy-broker/" + headers["X-Omnia-Agent-Name"]'
```

### `mode`

Controls how the policy is applied.

| Value | Description |
|-------|-------------|
| `enforce` | (Default) The broker returns `allow: false` for a matched deny rule; the runtime aborts the tool dispatch instead of calling the tool. See [Denial response format](#denial-response-format) below — there is no HTTP 403, since the decision endpoint always answers 200. |
| `audit` | Deny rules are evaluated but the request is allowed through. The decision returned to the runtime carries `wouldDeny: true`; the broker's own `policy_decision` log line for the match has `allowed: true` and a non-empty `deniedBy` naming the rule that would have denied it. |

### `onFailure`

Defines behavior when policy evaluation encounters an error (e.g., CEL expression failure).

| Value | Description |
|-------|-------------|
| `deny` | (Default) Deny the request on evaluation failure. |
| `allow` | Allow the request despite the evaluation error. |

## Status fields

### `phase`

| Value | Description |
|-------|-------------|
| `Active` | Policy is valid, all CEL rules compiled successfully. |
| `Error` | Policy has a configuration error (e.g., invalid CEL expression). |

### `ruleCount`

Integer count of compiled CEL rules.

### `conditions`

Standard Kubernetes conditions indicating the current state of the resource.

### `observedGeneration`

The most recent `.metadata.generation` observed by the controller.

## Print columns

When using `kubectl get toolpolicies`, the following columns are displayed:

| Column | Source |
|--------|--------|
| Registry | `.spec.selector.registry` |
| Mode | `.spec.mode` |
| Phase | `.status.phase` |
| Rules | `.status.ruleCount` |
| Age | `.metadata.creationTimestamp` |

## Denial response format

The broker always answers `POST /v1/decision` with HTTP 200 — it is a decision service, not a reverse proxy, so there is no HTTP-level error status for a policy decision. A denied call is expressed in the decision body itself:

```json
{
  "allow": false,
  "deniedBy": "max-refund-amount",
  "message": "Refund amount exceeds the $500 limit",
  "mode": "enforce",
  "wouldDeny": false,
  "injectedHeaders": null
}
```

The runtime reads `allow: false`, aborts the tool dispatch, and surfaces `message` (with `deniedBy` identifying the rule) as a policy-denied tool-call error instead of invoking the tool.

## Complete example

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ToolPolicy
metadata:
  name: refund-limits
  namespace: production
spec:
  selector:
    registry: customer-tools
    tools:
      - process_refund

  rules:
    - name: max-refund-amount
      description: "Prevent refunds over $500"
      deny:
        cel: 'double(body.amount) > 500.0'
        message: "Refund amount exceeds the $500 limit"

    - name: require-reason
      description: "All refunds must include a reason"
      deny:
        cel: '!has(body.reason) || body.reason == ""'
        message: "A reason is required for refund requests"

    - name: block-banned-customers
      description: "Deny refunds for flagged accounts"
      deny:
        cel: 'has(body.customer_status) && body.customer_status == "banned"'
        message: "Refunds are not available for this account"

  requiredClaims:
    - claim: Team
      message: "Team identity is required"
    - claim: Customer-Id
      message: "Customer ID is required for refund operations"

  headerInjection:
    - header: X-Tenant-Id
      cel: 'headers["X-Omnia-Claim-Customer-Id"]'
    - header: X-Audit-Source
      value: "policy-broker"

  mode: enforce
  onFailure: deny
```

Expected status after reconciliation:

```yaml
status:
  phase: Active
  ruleCount: 3
  observedGeneration: 1
  conditions:
    - type: Ready
      status: "True"
      reason: RulesCompiled
      message: "3 rules compiled successfully"
```

## Related resources

- [Policy Engine Architecture](/explanation/security/policy-engine/) — conceptual overview
- [AgentPolicy CRD Reference](/reference/policies/agentpolicy/) — network-level policies
- [Configure Tool Policies](/how-to/security/configure-tool-policies/) — operational guide
- [Securing Agents with Policies](/tutorials/securing-agents/) — end-to-end tutorial
