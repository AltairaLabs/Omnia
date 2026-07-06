---
title: "Configure agent policies"
description: "Restrict tool access and control agent behavior with AgentPolicy"
sidebar:
  order: 21
---


This guide covers common operational tasks for configuring AgentPolicy resources. For the full field reference, see the [AgentPolicy CRD Reference](/reference/policies/agentpolicy/).

## Prerequisites

- Istio installed in your cluster
- At least one AgentRuntime deployed
- If running Istio in **ambient** mode: a waypoint proxy enrolled for each agent Service you intend to enforce `toolAccess` on

:::note
AgentPolicy does not forward JWT claims. Claim forwarding to downstream tools is configured on the AgentRuntime's external-auth block — see [Configure Agent Authentication](/how-to/security/configure-authentication/).
:::

:::caution[toolAccess requires a waypoint under ambient mode]
`toolAccess` rules generate an Istio `AuthorizationPolicy` that matches on the `X-Omnia-Tool-Name` request header — an L7 attribute. Ambient mode's ztunnel only enforces L4, so this rule has no effect unless the target agent's Service is enrolled behind a **waypoint proxy**. Nothing in AgentPolicy provisions a waypoint automatically. Verify waypoint enrollment before relying on an allowlist/denylist for enforcement — otherwise every tool call is allowed through unchecked.
:::

## Restrict tool access with an allowlist

Limit an agent to only specific tools by creating an allowlist:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentPolicy
metadata:
  name: support-agent-tools
  namespace: production
spec:
  selector:
    agents:
      - support-agent
  toolAccess:
    mode: allowlist
    rules:
      - registry: customer-tools
        tools:
          - lookup_order
          - check_status
      - registry: knowledge-base
        tools:
          - search_articles
```

The agent can only call the three listed tools. All other tool calls are blocked at the Istio level.

## Block specific tools with a denylist

If most tools should be accessible but a few must be restricted:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentPolicy
metadata:
  name: restrict-dangerous-tools
  namespace: production
spec:
  selector:
    agents:
      - general-assistant
  toolAccess:
    mode: denylist
    rules:
      - registry: admin-tools
        tools:
          - delete_user
          - drop_table
          - reset_credentials
```

## Apply a policy to all agents

Omit the `selector` field or leave `agents` empty to match all agents in the namespace:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentPolicy
metadata:
  name: namespace-wide-policy
  namespace: production
spec:
  toolAccess:
    mode: denylist
    rules:
      - registry: admin-tools
        tools:
          - destructive_action
```

## Use permissive mode for safe rollout

When rolling out a new policy, start in `permissive` mode to verify behavior without blocking traffic:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentPolicy
metadata:
  name: new-restrictions
  namespace: production
spec:
  selector:
    agents:
      - customer-service
  toolAccess:
    mode: allowlist
    rules:
      - registry: customer-tools
        tools:
          - lookup_order
          - check_status
  mode: permissive  # Log violations without blocking
```

Monitor your logs for policy decisions, then switch to `enforce` when confident:

```yaml
spec:
  mode: enforce
```

## Verify policy status

Check that your policy is active and matching agents:

```bash
kubectl get agentpolicies -n production
```

Expected output:

```
NAME                      MODE      PHASE    MATCHED   AGE
support-agent-tools       enforce   Active   1         5m
restrict-dangerous-tools  enforce   Active   1         2m
```

For detailed status including conditions:

```bash
kubectl describe agentpolicy support-agent-tools -n production
```

## Combine multiple policies

Multiple AgentPolicies can apply to the same agent. Each policy is translated into its own Istio AuthorizationPolicy. Istio evaluates them independently — a request must pass all matching policies.

```yaml
# Policy 1: Tool restrictions for a specific agent
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentPolicy
metadata:
  name: tool-restrictions
  namespace: production
spec:
  selector:
    agents: [customer-service]
  toolAccess:
    mode: allowlist
    rules:
      - registry: customer-tools
        tools: [lookup_order, process_refund]
---
# Policy 2: Namespace-wide denylist (applies to all agents)
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentPolicy
metadata:
  name: namespace-wide-denylist
  namespace: production
spec:
  toolAccess:
    mode: denylist
    rules:
      - registry: admin-tools
        tools: [delete_user]
```

## Related resources

- [AgentPolicy CRD Reference](/reference/policies/agentpolicy/) — full field specification
- [Policy Engine Architecture](/explanation/security/policy-engine/) — how policies work
- [Configure Tool Policies](/how-to/security/configure-tool-policies/) — application-level CEL policies (Enterprise)
- [Securing Agents with Policies](/tutorials/securing-agents/) — end-to-end tutorial
