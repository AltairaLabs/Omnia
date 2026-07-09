---
title: "Manage workspaces"
description: "Create and configure workspaces for team isolation and access control"
sidebar:
  order: 8
---

This guide covers creating workspaces, configuring access control, and setting network isolation for multi-tenant deployments.

## Prerequisites

- Omnia operator deployed with Workspace controller enabled
- `kubectl` access to the cluster
- (For production) Identity provider configured for OIDC

## Create a workspace

A workspace provides an isolated environment for a team with its own namespace, RBAC, and network isolation.

### Basic workspace

Create a minimal workspace:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Workspace
metadata:
  name: my-team
spec:
  displayName: "My Team"
  namespace:
    name: omnia-my-team
    create: true
```

Apply it:

```bash
kubectl apply -f workspace.yaml
```

The controller will:
1. Create the namespace `omnia-my-team`
2. Create ServiceAccounts for each role (owner, editor, viewer)
3. Set up RBAC bindings

:::caution[A basic workspace persists nothing until you add a service group]
The minimal workspace above creates the namespace and RBAC, but **no session or memory
backend** — its `spec.services[]` is empty. Agents deployed into it start and chat, but
**silently fall back to an in-memory session store**: conversations, token usage, and cost
are **not persisted**, dashboard session views stay empty, and memory is unavailable. Before
running real agents, add at least one service group (conventionally `default`) — see
[Configure workspace service groups](/how-to/workspaces/configure-service-groups/).
:::

### Verify workspace status

```bash
kubectl get workspace my-team -o yaml
```

Check the status section:

```yaml
status:
  phase: Ready
  namespace:
    name: omnia-my-team
  serviceAccounts:
    owner: workspace-my-team-owner-sa
    editor: workspace-my-team-editor-sa
    viewer: workspace-my-team-viewer-sa
```

:::caution[A Ready workspace is only visible to users who have access to it]
The dashboard lists only the workspaces the **current user resolves to a role on**. A
workspace with no `roleBindings`, `directGrants`, or `anonymousAccess` reconciles to
`Ready` but is visible to **nobody** — an empty dashboard almost always means "no access",
not "no workspace". Confirm what the API actually serves for the current user with
`curl http://localhost:3000/api/workspaces` (an empty `{"workspaces":[],"count":0}` is the
"no access" signal).

The controller computes each user's role from three sources, matched against the identity
and **group claims** the dashboard's authentication establishes for the request:

- **`roleBindings`** — map IdP **group** names to a role; the user's group claims must
  exactly match a binding's `groups` (see [Role bindings](#role-bindings-with-idp-groups)).
- **`directGrants`** — grant a role to a specific user identity, with optional expiry
  (see [Direct user grants](#direct-user-grants)).
- **`anonymousAccess`** — applies only when the dashboard runs in anonymous mode.

Which identity and groups a request carries depends on how the dashboard is authenticated,
so configure that first, then make your `roleBindings.groups` match the group names your IdP
emits:

- **Production:** configure [dashboard authentication](/how-to/security/configure-dashboard-auth/)
  (proxy or OAuth mode) so users arrive with an identity and IdP group claims, and
  [agent authentication](/how-to/security/configure-authentication/) for programmatic
  callers. For scoped machine access, see
  [Durable API keys → scope a key to workspaces](/how-to/security/api-keys/#scope-a-key-to-workspaces).
- **Local dev only** (`dashboard.auth.mode=anonymous`, the mode the Tilt/dev stack ships):
  there is no real identity, so grant the anonymous user access explicitly to see the
  workspace:

  ```yaml
  spec:
    anonymousAccess:
      enabled: true
      role: owner   # never in production — anyone reaching the dashboard gets this role
  ```
:::

## Configure access control

### Role bindings with IdP groups

Map identity provider groups to workspace roles:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Workspace
metadata:
  name: customer-support
spec:
  displayName: "Customer Support Team"
  namespace:
    name: omnia-customer-support
    create: true

  roleBindings:
    # Team leads get full control
    - groups:
        - "cs-admins@acme.com"
      role: owner

    # Engineers can create and modify resources
    - groups:
        - "cs-engineers@acme.com"
        - "support-team"
      role: editor

    # Contractors get read-only access
    - groups:
        - "cs-contractors@acme.com"
      role: viewer
```

When users authenticate via OIDC, their group claims are matched against these bindings.

### ServiceAccount access for CI/CD

Grant access to ServiceAccounts for automated pipelines:

```yaml
spec:
  roleBindings:
    # ArgoCD can deploy agents
    - serviceAccounts:
        - name: argocd-application-controller
          namespace: argocd
      role: editor

    # GitHub Actions can deploy
    - serviceAccounts:
        - name: github-actions
          namespace: ci-system
      role: editor
```

### Direct user grants

For exceptions (use sparingly):

```yaml
spec:
  directGrants:
    # Temporary admin access for incident response
    - user: oncall@acme.com
      role: owner
      expires: "2026-02-01T00:00:00Z"
```

:::caution
Direct grants don't scale. Use IdP groups for most access control. Direct grants are for temporary exceptions only.
:::

### Anonymous access

For development environments without authentication:

```yaml
spec:
  anonymousAccess:
    enabled: true
    role: viewer  # Read-only for anonymous users
```

:::danger
Never enable anonymous access with `editor` or `owner` roles in production. Anonymous users could modify or delete resources.
:::

## Limit resource usage

:::note
The Workspace controller does **not** currently manage resource quotas or budgets. The
`spec` has no `quotas` field — applying one is rejected by the API server with
`strict decoding error: unknown field "spec.quotas"`. Tracking enforcement is
[issue #1781](https://github.com/AltairaLabs/Omnia/issues/1781).
:::

Until workspace-native quotas land, apply a standard Kubernetes
[`ResourceQuota`](https://kubernetes.io/docs/concepts/policy/resource-quotas/) directly to
the workspace namespace. The controller creates the namespace for you (see above), so you
can quota it like any other namespace:

```yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  name: workspace-limits
  namespace: omnia-customer-support  # Workspace namespace
spec:
  hard:
    requests.cpu: "50"
    requests.memory: "100Gi"
    limits.cpu: "100"
    limits.memory: "200Gi"
    configmaps: "100"
    secrets: "50"
    persistentvolumeclaims: "20"
```

```bash
kubectl apply -f resourcequota.yaml
kubectl describe resourcequota workspace-limits -n omnia-customer-support
```

## Set environment and tags

### Environment tier

Classify workspaces by environment:

```yaml
spec:
  environment: production  # development | staging | production
```

This enables environment-based policies and promotion workflows.

### Cost attribution tags

Add tags for cost tracking:

```yaml
spec:
  defaultTags:
    team: "customer-support"
    cost-center: "CC-1234"
    business-unit: "support-ops"
```

These tags are applied to all resources created in the workspace.

## Configure network isolation

Network isolation restricts traffic to and from your workspace namespace using Kubernetes NetworkPolicies. This provides defense-in-depth for multi-tenant environments.

### Enable basic isolation

Add network isolation to restrict traffic:

```yaml
spec:
  networkPolicy:
    isolate: true
```

This automatically creates a NetworkPolicy that:
- Allows DNS queries to `kube-system`
- Allows all traffic within the workspace namespace
- Allows traffic to/from namespaces labeled `omnia.altairalabs.ai/shared: true`
- Allows egress to external IPs (for LLM APIs) but blocks other private IP ranges

### Verify NetworkPolicy

Check that the NetworkPolicy was created:

```bash
kubectl get networkpolicy -n omnia-customer-support
```

You should see:

```
NAME                                   POD-SELECTOR   AGE
workspace-customer-support-isolation   <none>         1m
```

### Allow ingress from load balancer

If agents need to receive traffic from an ingress controller:

```yaml
spec:
  networkPolicy:
    isolate: true
    allowFrom:
      - peers:
          - namespaceSelector:
              matchLabels:
                kubernetes.io/metadata.name: ingress-nginx
```

### Allow egress to internal services

To allow agents to connect to internal databases or services:

```yaml
spec:
  networkPolicy:
    isolate: true
    allowTo:
      - peers:
          - ipBlock:
              cidr: 10.0.0.0/8  # Internal network
        ports:
          - protocol: TCP
            port: 5432  # PostgreSQL
          - protocol: TCP
            port: 6379  # Redis
```

### Restrict external API access

For high-security environments, disable external API access:

```yaml
spec:
  networkPolicy:
    isolate: true
    allowExternalAPIs: false
    allowTo:
      # Only allow specific external endpoints
      - peers:
          - ipBlock:
              cidr: 104.18.0.0/16  # Example: specific API provider
        ports:
          - protocol: TCP
            port: 443
```

:::caution
Disabling `allowExternalAPIs` blocks agents from reaching LLM provider APIs unless you explicitly allow them. Make sure to add egress rules for any external services your agents need.
:::

### Allow private networks (local development)

For local development or when agents need to access services on private networks (e.g., a local Ollama instance):

```yaml
spec:
  networkPolicy:
    isolate: true
    allowPrivateNetworks: true
```

This removes the RFC 1918 private IP exclusions (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`), allowing agents to reach services on your local network.

:::caution
Only enable `allowPrivateNetworks` in development environments. In production, use specific `allowTo` rules for required internal services instead.
:::

### Disable isolation

To remove network restrictions, either delete the `networkPolicy` section or set `isolate: false`:

```yaml
spec:
  networkPolicy:
    isolate: false
```

The controller will automatically delete the NetworkPolicy.

## Provide session and memory backends

A workspace's namespace and RBAC don't give its agents anywhere to persist conversations or
memory — that comes from a **service group** (`spec.services[]`), which provisions the
session-api and memory-api the agents use.

:::caution
Without a ready service group, agents still start but **silently fall back to a
non-persistent in-memory session store** — no session history, tokens, cost, or memory. Most
real workspaces need a group named `default`.
:::

Setting this up (database secrets, managed vs external mode, pointing agents at a group) is
covered in its own guide: **[Configure workspace service groups](/how-to/workspaces/configure-service-groups/)**.

## Deploy resources to a workspace

Once your workspace is ready — and its service group is provisioned — deploy agents to its
namespace:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: support-bot
  namespace: omnia-customer-support  # Workspace namespace
spec:
  serviceGroup: default              # session/memory backend (see service groups guide)
  promptPackRef:
    name: support-prompts
  providers:
    - name: default
      providerRef:
        name: claude-provider
```

The dashboard automatically scopes resources to the current workspace.

## Use the dashboard

### Switch workspaces

The dashboard shows a workspace selector in the header. Users only see workspaces they have access to.

### View workspace resources

When you select a workspace, the dashboard shows:
- Agents deployed in that workspace
- PromptPacks in the workspace namespace
- Events and logs scoped to that workspace

### Access control in dashboard

The dashboard enforces role-based access:

| Role | Can View | Can Create/Edit | Can Delete | Can Manage Members |
|------|----------|-----------------|------------|-------------------|
| viewer | Yes | No | No | No |
| editor | Yes | Yes | Yes | No |
| owner | Yes | Yes | Yes | Yes |

## Complete example

A production-ready workspace with all features:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Workspace
metadata:
  name: customer-support
spec:
  displayName: "Customer Support Team"
  description: "Team responsible for customer support AI agents"
  environment: production

  defaultTags:
    team: "customer-support"
    cost-center: "CC-1234"

  namespace:
    name: omnia-customer-support
    create: true
    labels:
      environment: production
      team: customer-support

  roleBindings:
    - groups:
        - "cs-admins@acme.com"
      role: owner

    - groups:
        - "cs-engineers@acme.com"
      role: editor

    - groups:
        - "cs-contractors@acme.com"
      role: viewer

    - serviceAccounts:
        - name: argocd-application-controller
          namespace: argocd
      role: editor

  networkPolicy:
    isolate: true
    allowFrom:
      - peers:
          - namespaceSelector:
              matchLabels:
                kubernetes.io/metadata.name: ingress-nginx
    allowTo:
      - peers:
          - ipBlock:
              cidr: 10.0.0.0/8
        ports:
          - protocol: TCP
            port: 5432
```

## Troubleshooting

### Workspace stuck in pending

**Symptom:** Workspace phase remains `Pending`

**Check:**
1. Verify namespace doesn't already exist with conflicting labels
2. Check operator logs: `kubectl logs -n omnia-system deploy/omnia-controller-manager`
3. Ensure `spec.namespace.create: true` if namespace should be auto-created

### Access denied to workspace

**Symptom:** User can't access workspace in dashboard

**Check:**
1. Verify user's groups in JWT token (decode at jwt.io)
2. Confirm group names match exactly in `roleBindings`
3. Check if anonymous access is enabled (for development)

### ServiceAccount token issues

**Symptom:** API calls fail with authentication errors

**Check:**
1. Verify ServiceAccounts exist: `kubectl get sa -n omnia-customer-support`
2. Check RoleBindings: `kubectl get rolebindings -n omnia-customer-support`
3. Ensure workspace phase is `Ready`

### Quota exceeded

**Symptom:** Cannot create new resources

Quotas are not managed by the Workspace controller; if you applied a native
`ResourceQuota` to the namespace (see [Limit resource usage](#limit-resource-usage)):

**Check:**
1. View current usage: `kubectl describe resourcequota -n omnia-customer-support`
2. Review the `ResourceQuota` `spec.hard` limits
3. Clean up unused resources or raise the limits

### Network connectivity issues

**Symptom:** Agents can't reach external APIs or internal services

**Check:**
1. Verify NetworkPolicy exists: `kubectl get networkpolicy -n omnia-customer-support`
2. Check if `allowExternalAPIs: false` is blocking external traffic
3. Inspect the NetworkPolicy rules: `kubectl describe networkpolicy workspace-customer-support-isolation -n omnia-customer-support`
4. Add custom `allowTo` rules for required services

**Debug with a test pod:**
```bash
kubectl run -n omnia-customer-support debug --rm -it --image=busybox -- sh
# Inside the pod:
nslookup api.anthropic.com  # Test DNS
wget -qO- https://api.anthropic.com  # Test external access
```

### Agents not receiving traffic

**Symptom:** Ingress traffic doesn't reach agents

**Check:**
1. Ensure ingress controller namespace is allowed in `allowFrom`:
   ```yaml
   allowFrom:
     - peers:
         - namespaceSelector:
             matchLabels:
               kubernetes.io/metadata.name: ingress-nginx
   ```
2. Verify the ingress controller namespace has the correct labels
3. Check that the NetworkPolicy allows the required ports

## Next steps

- [Configure workspace service groups](/how-to/workspaces/configure-service-groups/) - Provision the session/memory backends agents need
- [Multi-Tenancy Architecture](/explanation/platform/multi-tenancy/) - Understand workspace isolation
- [Configure Dashboard Authentication](/how-to/security/configure-dashboard-auth/) - Set up OIDC
- [Workspace CRD Reference](/reference/core/workspace/) - Complete field reference
