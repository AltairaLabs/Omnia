---
title: "ToolPolicy CRD"
description: "Complete reference for the ToolPolicy custom resource"
sidebar:
  order: 6
---

:::note[Enterprise]
ToolPolicy is an Enterprise feature. See [Licensing](/explanation/licensing/) for details.
:::

The ToolPolicy custom resource defines CEL-based access control rules for tool invocations. Rules are evaluated by a policy proxy sidecar that intercepts requests to tool services.

## API Version

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ToolPolicy
```

## Spec Fields

### `selector`

Defines which tools this policy applies to. The proxy matches incoming requests based on the `X-Omnia-Tool-Registry` and `X-Omnia-Tool-Name` headers.

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

#### CEL Variables

The following variables are available in CEL expressions:

| Variable | Type | Description |
|----------|------|-------------|
| `headers` | `map<string, string>` | All HTTP request headers (first value only for multi-value headers). |
| `body` | `map<string, dyn>` | Parsed JSON request body. Empty map if body is not JSON. |

#### CEL String Extensions

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
      message: "Team claim is required — configure claimMapping in your AgentPolicy"
    - claim: Customer-Id
      message: "Customer ID claim is required for this tool"
```

:::tip
Required claims depend on an AgentPolicy with `claimMapping` configured for the same agent. The AgentPolicy extracts claims from the JWT; the ToolPolicy verifies they are present.
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
      cel: '"policy-proxy/" + headers["X-Omnia-Agent-Name"]'
```

### `mode`

Controls how the policy is applied.

| Value | Description |
|-------|-------------|
| `enforce` | (Default) Deny rules block the request with a 403 response. |
| `audit` | Deny rules are evaluated but the request is allowed through. Violations are logged with `wouldDeny: true`. |

### `onFailure`

Defines behavior when policy evaluation encounters an error (e.g., CEL expression failure).

| Value | Description |
|-------|-------------|
| `deny` | (Default) Deny the request on evaluation failure. |
| `allow` | Allow the request despite the evaluation error. |

### `audit`

Configures audit logging for policy decisions.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `logDecisions` | bool | No | Enable logging of all policy decisions (allow and deny). |
| `redactFields` | []string | No | Field names whose values are redacted in audit logs. |

```yaml
spec:
  audit:
    logDecisions: true
    redactFields:
      - credit_card
      - ssn
      - password
```

## Status Fields

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

## Print Columns

When using `kubectl get toolpolicies`, the following columns are displayed:

| Column | Source |
|--------|--------|
| Registry | `.spec.selector.registry` |
| Mode | `.spec.mode` |
| Phase | `.status.phase` |
| Rules | `.status.ruleCount` |
| Age | `.metadata.creationTimestamp` |

## Denial Response Format

When a request is denied, the proxy returns HTTP 403 with:

```json
{
  "error": "policy_denied",
  "rule": "max-refund-amount",
  "message": "Refund amount exceeds the $500 limit"
}
```

## Complete Example

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
      value: "policy-proxy"

  mode: enforce
  onFailure: deny

  audit:
    logDecisions: true
    redactFields:
      - credit_card
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

## Related Resources

- [Policy Engine Architecture](/explanation/policy-engine/) — conceptual overview
- [AgentPolicy CRD Reference](/reference/agentpolicy/) — network-level policies
- [Configure Tool Policies](/how-to/configure-tool-policies/) — operational guide
- [Securing Agents with Policies](/tutorials/securing-agents/) — end-to-end tutorial
