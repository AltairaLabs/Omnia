---
title: "Manage Workspaces"
description: "Create and configure workspaces for team isolation and access control"
sidebar:
  order: 8
---

This guide covers creating workspaces, configuring access control, and setting resource quotas for multi-tenant deployments.

## Prerequisites

- Omnia operator deployed with Workspace controller enabled
- `kubectl` access to the cluster
- (For production) Identity provider configured for OIDC

## Create a Workspace

A workspace provides an isolated environment for a team with its own namespace, RBAC, and resource quotas.

### Basic Workspace

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

### Verify Workspace Status

```bash
kubectl get workspace my-team -o yaml
```

Check the status section:

```yaml
status:
  phase: Ready
  namespace:
    name: omnia-my-team
    created: true
  serviceAccounts:
    owner: my-team-owner
    editor: my-team-editor
    viewer: my-team-viewer
```

## Configure Access Control

### Role Bindings with IdP Groups

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

### ServiceAccount Access for CI/CD

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

### Direct User Grants

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

### Anonymous Access

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

## Configure Resource Quotas

### Compute Quotas

Limit CPU and memory usage:

```yaml
spec:
  quotas:
    compute:
      requests.cpu: "50"
      requests.memory: "100Gi"
      limits.cpu: "100"
      limits.memory: "200Gi"
```

### Object Quotas

Limit the number of Kubernetes objects:

```yaml
spec:
  quotas:
    objects:
      configmaps: 100
      secrets: 50
      persistentvolumeclaims: 20
```

### Agent Quotas

Control AgentRuntime deployments:

```yaml
spec:
  quotas:
    agents:
      maxAgentRuntimes: 20
      maxReplicasPerAgent: 10
```

### Arena Quotas

Limit Arena evaluation jobs:

```yaml
spec:
  quotas:
    arena:
      maxConcurrentJobs: 10
      maxJobsPerDay: 100
      maxWorkersPerJob: 50
```

## Set Environment and Tags

### Environment Tier

Classify workspaces by environment:

```yaml
spec:
  environment: production  # development | staging | production
```

This enables environment-based policies and promotion workflows.

### Cost Attribution Tags

Add tags for cost tracking:

```yaml
spec:
  defaultTags:
    team: "customer-support"
    cost-center: "CC-1234"
    business-unit: "support-ops"
```

These tags are applied to all resources created in the workspace.

## Configure Network Isolation

Network isolation restricts traffic to and from your workspace namespace using Kubernetes NetworkPolicies. This provides defense-in-depth for multi-tenant environments.

### Enable Basic Isolation

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

### Allow Ingress from Load Balancer

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

### Allow Egress to Internal Services

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

### Restrict External API Access

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

### Allow Private Networks (Local Development)

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

### Disable Isolation

To remove network restrictions, either delete the `networkPolicy` section or set `isolate: false`:

```yaml
spec:
  networkPolicy:
    isolate: false
```

The controller will automatically delete the NetworkPolicy.

## Deploy Resources to a Workspace

Once your workspace is ready, deploy agents to its namespace:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: support-bot
  namespace: omnia-customer-support  # Workspace namespace
spec:
  promptPackRef:
    name: support-prompts
  providerRef:
    name: claude-provider
```

The dashboard automatically scopes resources to the current workspace.

## Use the Dashboard

### Switch Workspaces

The dashboard shows a workspace selector in the header. Users only see workspaces they have access to.

### View Workspace Resources

When you select a workspace, the dashboard shows:
- Agents deployed in that workspace
- PromptPacks in the workspace namespace
- Events and logs scoped to that workspace

### Access Control in Dashboard

The dashboard enforces role-based access:

| Role | Can View | Can Create/Edit | Can Delete | Can Manage Members |
|------|----------|-----------------|------------|-------------------|
| viewer | Yes | No | No | No |
| editor | Yes | Yes | Yes | No |
| owner | Yes | Yes | Yes | Yes |

## Complete Example

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

  quotas:
    compute:
      requests.cpu: "50"
      requests.memory: "100Gi"
      limits.cpu: "100"
      limits.memory: "200Gi"

    objects:
      configmaps: 100
      secrets: 50
      persistentvolumeclaims: 20

    agents:
      maxAgentRuntimes: 20
      maxReplicasPerAgent: 10

    arena:
      maxConcurrentJobs: 10
      maxJobsPerDay: 100

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

### Workspace Stuck in Pending

**Symptom:** Workspace phase remains `Pending`

**Check:**
1. Verify namespace doesn't already exist with conflicting labels
2. Check operator logs: `kubectl logs -n omnia-system deploy/omnia-controller-manager`
3. Ensure `spec.namespace.create: true` if namespace should be auto-created

### Access Denied to Workspace

**Symptom:** User can't access workspace in dashboard

**Check:**
1. Verify user's groups in JWT token (decode at jwt.io)
2. Confirm group names match exactly in `roleBindings`
3. Check if anonymous access is enabled (for development)

### ServiceAccount Token Issues

**Symptom:** API calls fail with authentication errors

**Check:**
1. Verify ServiceAccounts exist: `kubectl get sa -n omnia-customer-support`
2. Check RoleBindings: `kubectl get rolebindings -n omnia-customer-support`
3. Ensure workspace phase is `Ready`

### Quota Exceeded

**Symptom:** Cannot create new resources

**Check:**
1. View current usage: `kubectl describe resourcequota -n omnia-customer-support`
2. Review workspace quota settings
3. Clean up unused resources or increase quotas

### Network Connectivity Issues

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

### Agents Not Receiving Traffic

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

## Next Steps

- [Multi-Tenancy Architecture](/explanation/multi-tenancy/) - Understand workspace isolation
- [Configure Dashboard Authentication](/how-to/configure-dashboard-auth/) - Set up OIDC
- [Workspace CRD Reference](/reference/workspace/) - Complete field reference
