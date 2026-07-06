---
title: "AgentPolicy CRD"
description: "Complete reference for the AgentPolicy custom resource"
sidebar:
  order: 5
---


The AgentPolicy custom resource defines network-level access control rules for AI agents. It configures tool access restrictions and enforcement modes via Istio AuthorizationPolicy.

AgentPolicy does not forward JWT claims. Claim forwarding to downstream tools is configured on the AgentRuntime's external-auth block (`spec.externalAuth.oidc.claimMapping` or the edge-trust equivalent) — see [Configure Agent Authentication](/how-to/security/configure-authentication/).

## API version

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentPolicy
```

## Spec fields

### `selector`

Determines which agents this policy applies to.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `agents` | []string | No | List of AgentRuntime names. If empty, applies to all agents in the namespace. |

```yaml
spec:
  selector:
    agents:
      - customer-service
      - internal-assistant
```

### `toolAccess`

Defines tool allowlist or denylist rules. These are enforced at the Istio network level via generated AuthorizationPolicy resources.

:::caution[Requires a waypoint under ambient mode]
The generated `AuthorizationPolicy` matches on the `X-Omnia-Tool-Name` request header — an L7 attribute. Under Istio ambient mode, L7 header matching is only enforced by a **waypoint proxy**; ztunnel alone enforces L4 only. The operator always creates the `AuthorizationPolicy` regardless, but it has no effect unless the target agent's Service is enrolled behind a waypoint. There is no automatic waypoint provisioning tied to AgentPolicy — verify enrollment separately before relying on `toolAccess` for enforcement.
:::

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `mode` | string | Yes | Access control mode: `allowlist` or `denylist`. |
| `rules` | []ToolAccessRule | Yes | List of tool access rules (minimum 1). |

Each `ToolAccessRule` has:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `registry` | string | Yes | Name of the ToolRegistry resource. |
| `tools` | []string | Yes | List of tool names within the registry (minimum 1). |

**Allowlist example** — only permit specific tools:

```yaml
spec:
  toolAccess:
    mode: allowlist
    rules:
      - registry: customer-tools
        tools:
          - lookup_order
          - check_status
      - registry: common-tools
        tools:
          - search_kb
```

**Denylist example** — block specific tools:

```yaml
spec:
  toolAccess:
    mode: denylist
    rules:
      - registry: admin-tools
        tools:
          - delete_user
          - reset_database
```

### `mode`

Controls how the policy is applied.

| Value | Description |
|-------|-------------|
| `enforce` | (Default) Policy violations block the request. |
| `permissive` | Policy violations are logged but the request is allowed through. |

:::caution
Use `permissive` mode when rolling out new policies to verify behavior before switching to `enforce`. Check logs to confirm the policy is matching as expected.
:::

### `onFailure`

Defines behavior when policy evaluation encounters an error.

| Value | Description |
|-------|-------------|
| `deny` | (Default) Deny the request on evaluation failure. |
| `allow` | Allow the request despite the evaluation error. |

## Status fields

### `phase`

| Value | Description |
|-------|-------------|
| `Active` | Policy is valid and applied. |
| `Error` | Policy has a configuration error. |

### `matchedAgents`

Integer count of AgentRuntime resources matched by the selector.

### `conditions`

Standard Kubernetes conditions indicating the current state of the resource.

### `observedGeneration`

The most recent `.metadata.generation` observed by the controller.

## Print columns

When using `kubectl get agentpolicies`, the following columns are displayed:

| Column | Source |
|--------|--------|
| Mode | `.spec.mode` |
| Phase | `.status.phase` |
| Matched | `.status.matchedAgents` |
| Age | `.metadata.creationTimestamp` |

## Complete example

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentPolicy
metadata:
  name: customer-service-policy
  namespace: production
spec:
  selector:
    agents:
      - customer-service-agent

  toolAccess:
    mode: allowlist
    rules:
      - registry: customer-tools
        tools:
          - lookup_order
          - check_status
          - process_refund
      - registry: common-tools
        tools:
          - search_kb

  mode: enforce
  onFailure: deny
```

Expected status after reconciliation:

```yaml
status:
  phase: Active
  matchedAgents: 1
  observedGeneration: 1
  conditions:
    - type: Ready
      status: "True"
      reason: PolicyApplied
      message: "Istio AuthorizationPolicy created"
```

## Related resources

- [Policy Engine Architecture](/explanation/security/policy-engine/) — conceptual overview
- [ToolPolicy CRD Reference](/reference/policies/toolpolicy/) — application-level CEL policies (Enterprise)
- [Configure Agent Policies](/how-to/security/configure-agent-policies/) — operational guide
- [Securing Agents with Policies](/tutorials/securing-agents/) — end-to-end tutorial
