---
title: "Configure tool policies"
description: "Write CEL deny rules, inject headers, and audit tool calls with ToolPolicy"
sidebar:
  order: 22
---

:::note[Enterprise]
ToolPolicy is an Enterprise feature. See [Licensing](/explanation/platform/licensing/) for details.
:::

This guide covers common operational tasks for configuring ToolPolicy resources. For the full field reference, see the [ToolPolicy CRD Reference](/reference/policies/toolpolicy/).

## Prerequisites

- Omnia Enterprise license activated
- At least one ToolRegistry deployed
- For claim-based rules: the AgentRuntime's external-auth claim mapping configured so `X-Omnia-Claim-*` headers are populated (see [Configure Agent Authentication](/how-to/security/configure-authentication/))

## Write a CEL deny rule

CEL (Common Expression Language) rules evaluate against request headers and body. A rule denies the request when its expression evaluates to `true`.

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ToolPolicy
metadata:
  name: refund-guardrails
  namespace: production
spec:
  selector:
    registry: customer-tools
    tools:
      - process_refund
  rules:
    - name: max-refund-amount
      description: "Cap refunds at $500"
      deny:
        cel: 'double(body.amount) > 500.0'
        message: "Refund amount exceeds the $500 limit"
```

### Access request body fields

The `body` variable contains the parsed JSON request body:

```yaml
# Check for a specific field value
- name: block-status
  deny:
    cel: 'has(body.status) && body.status == "cancelled"'
    message: "Cannot process cancelled orders"

# Check numeric ranges
- name: quantity-limit
  deny:
    cel: 'has(body.quantity) && int(body.quantity) > 100'
    message: "Quantity exceeds maximum of 100"
```

:::tip
Always guard field access with `has()` to avoid errors when the field is absent. For example, `has(body.amount) && double(body.amount) > 500.0` instead of `double(body.amount) > 500.0`.
:::

### Access request headers

The `headers` variable contains all HTTP headers as a string map:

```yaml
- name: require-internal-source
  deny:
    cel: '!("X-Source" in headers) || headers["X-Source"] != "internal"'
    message: "Only internal requests are allowed"
```

### Use string extensions

CEL string extensions are available for pattern matching:

```yaml
- name: block-external-urls
  deny:
    cel: 'has(body.url) && !body.url.startsWith("https://internal.")'
    message: "Only internal URLs are permitted"

- name: block-sql-patterns
  deny:
    cel: 'has(body.query) && body.query.matches("(?i)(DROP|DELETE|TRUNCATE)")'
    message: "Destructive SQL operations are not allowed"
```

## Require JWT claims

Ensure that specific claims are present before any CEL rules run. This is useful for enforcing that identity propagation is configured correctly.

```yaml
spec:
  requiredClaims:
    - claim: Team
      message: "Team claim is required — ensure the AgentRuntime's external-auth claim mapping is configured"
    - claim: Customer-Id
      message: "Customer ID is required for this tool"
```

Required claims check for the presence of `X-Omnia-Claim-<Claim>` headers. These headers are populated by the AgentRuntime's external-auth claim mapping (see [Configure Agent Authentication](/how-to/security/configure-authentication/)), not by AgentPolicy — AgentPolicy governs only tool allow/deny.

## Inject headers into upstream requests

Add headers to the request after policy evaluation passes. Use static values or CEL expressions:

```yaml
spec:
  headerInjection:
    # Static header
    - header: X-Policy-Version
      value: "v2"

    # Forward a claim as a different header
    - header: X-Tenant-Id
      cel: 'headers["X-Omnia-Claim-Customer-Id"]'

    # Compute a value from multiple inputs
    - header: X-Audit-Trail
      cel: 'headers["X-Omnia-Agent-Name"] + "/" + headers["X-Omnia-Session-Id"]'
```

:::caution
Each header injection rule must set exactly one of `value` or `cel`. Setting both or neither causes a validation error.
:::

## Use audit mode for dry-run

Start with `audit` mode to see what would be denied without blocking requests:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ToolPolicy
metadata:
  name: new-limits
  namespace: production
spec:
  selector:
    registry: customer-tools
  rules:
    - name: strict-amount-check
      deny:
        cel: 'double(body.amount) > 100.0'
        message: "Amount exceeds strict limit"
  mode: audit  # Log violations without blocking
```

The proxy logs audit decisions with `wouldDeny: true`:

```json
{
  "msg": "policy_decision",
  "decision": "deny",
  "wouldDeny": true,
  "mode": "audit",
  "policy": "new-limits",
  "rule": "strict-amount-check",
  "path": "/v1/refund",
  "method": "POST"
}
```

Once satisfied, switch to `enforce`:

```yaml
spec:
  mode: enforce
```

## Configure audit logging and redaction

Enable full decision logging and redact sensitive fields:

```yaml
spec:
  audit:
    logDecisions: true
    redactFields:
      - credit_card
      - ssn
      - api_key
      - password
```

With `logDecisions: true`, every request (allowed and denied) generates a structured log entry. Fields listed in `redactFields` have their values masked in log output.

## Apply a policy to all tools in a registry

Omit the `tools` list in the selector to match every tool in the registry:

```yaml
spec:
  selector:
    registry: customer-tools
    # No tools list — matches all tools in this registry
  rules:
    - name: require-auth-header
      deny:
        cel: '!("Authorization" in headers)'
        message: "Authorization header is required"
```

## Verify policy status

Check that your policies are active and rules compiled:

```bash
kubectl get toolpolicies -n production
```

Expected output:

```
NAME               REGISTRY         MODE      PHASE    RULES   AGE
refund-guardrails  customer-tools   enforce   Active   1       5m
new-limits         customer-tools   audit     Active   1       2m
```

If a policy shows `Error` phase, describe it to see the compilation error:

```bash
kubectl describe toolpolicy refund-guardrails -n production
```

## Common CEL patterns

### Claim-based access control

```yaml
- name: team-restriction
  deny:
    cel: 'headers["X-Omnia-Claim-Team"] != "finance"'
    message: "Only the finance team can use this tool"
```

### Time-based restrictions

```yaml
- name: business-hours-only
  deny:
    cel: 'int(headers["X-Request-Hour"]) < 9 || int(headers["X-Request-Hour"]) > 17'
    message: "This tool is only available during business hours"
```

### Multiple conditions

```yaml
- name: high-value-finance-only
  deny:
    cel: 'double(body.amount) > 1000.0 && headers["X-Omnia-Claim-Team"] != "finance"'
    message: "Only the finance team can process amounts over $1000"
```

## Related resources

- [ToolPolicy CRD Reference](/reference/policies/toolpolicy/) — full field specification
- [Policy Engine Architecture](/explanation/security/policy-engine/) — how policies work
- [Configure Agent Policies](/how-to/security/configure-agent-policies/) — network-level policies
- [Securing Agents with Policies](/tutorials/securing-agents/) — end-to-end tutorial
