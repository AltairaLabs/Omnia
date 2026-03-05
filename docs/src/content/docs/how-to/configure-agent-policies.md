---
title: "Configure Agent Policies"
description: "Restrict tool access, map JWT claims, and control agent behavior with AgentPolicy"
sidebar:
  order: 21
---


This guide covers common operational tasks for configuring AgentPolicy resources. For the full field reference, see the [AgentPolicy CRD Reference](/reference/agentpolicy/).

## Prerequisites

- Istio installed in your cluster
- At least one AgentRuntime deployed
- For claim mapping: JWT authentication configured (see [Configure Agent Authentication](/how-to/configure-authentication/))

## Restrict Tool Access with an Allowlist

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

## Block Specific Tools with a Denylist

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

## Map JWT Claims to Headers

Forward user identity from the JWT token to downstream services:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentPolicy
metadata:
  name: identity-propagation
  namespace: production
spec:
  claimMapping:
    forwardClaims:
      - claim: sub
        header: X-Omnia-Claim-Sub
      - claim: team
        header: X-Omnia-Claim-Team
      - claim: org.tenant_id
        header: X-Omnia-Claim-Tenant-Id
```

:::tip
Use dot-notation for nested JWT claims. For example, `org.tenant_id` extracts the value from `{"org": {"tenant_id": "acme"}}`.
:::

This policy applies to **all agents** in the namespace (no `selector.agents` specified). Every tool call will include the mapped claim headers, making them available to ToolPolicy CEL rules and downstream services.

## Apply a Policy to All Agents

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

## Use Permissive Mode for Safe Rollout

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

## Verify Policy Status

Check that your policy is active and matching agents:

```bash
kubectl get agentpolicies -n production
```

Expected output:

```
NAME                    MODE      PHASE    MATCHED   AGE
support-agent-tools     enforce   Active   1         5m
identity-propagation    enforce   Active   3         2m
```

For detailed status including conditions:

```bash
kubectl describe agentpolicy support-agent-tools -n production
```

## Combine Multiple Policies

Multiple AgentPolicies can apply to the same agent. Each policy is translated into its own Istio AuthorizationPolicy. Istio evaluates them independently — a request must pass all matching policies.

```yaml
# Policy 1: Tool restrictions
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
# Policy 2: Identity propagation (applies to all agents)
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentPolicy
metadata:
  name: identity-forwarding
  namespace: production
spec:
  claimMapping:
    forwardClaims:
      - claim: team
        header: X-Omnia-Claim-Team
```

## Related Resources

- [AgentPolicy CRD Reference](/reference/agentpolicy/) — full field specification
- [Policy Engine Architecture](/explanation/policy-engine/) — how policies work
- [Configure Tool Policies](/how-to/configure-tool-policies/) — application-level CEL policies (Enterprise)
- [Securing Agents with Policies](/tutorials/securing-agents/) — end-to-end tutorial
