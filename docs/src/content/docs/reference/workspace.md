---
title: "Workspace CRD"
description: "Complete reference for the Workspace custom resource"
sidebar:
  order: 10
---

The Workspace custom resource defines a multi-tenant workspace with isolated namespace, RBAC, and resource quotas in Kubernetes.

## API Version

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Workspace
```

## Resource Scope

Workspace is a **cluster-scoped** resource. It creates and manages resources in its associated namespace.

## Spec Fields

### `displayName`

Human-readable name for the workspace shown in the dashboard.

| Field | Type | Required |
|-------|------|----------|
| `displayName` | string | Yes |

```yaml
spec:
  displayName: "Customer Support Team"
```

### `description`

Optional description of the workspace.

| Field | Type | Required |
|-------|------|----------|
| `description` | string | No |

```yaml
spec:
  description: "Team responsible for customer support AI agents"
```

### `environment`

Environment tier for the workspace. Enables environment-based workflows and policies.

| Field | Type | Default | Required |
|-------|------|---------|----------|
| `environment` | string | development | No |

Environment types:

| Value | Description |
|-------|-------------|
| `development` | Development workspaces for testing and iteration |
| `staging` | Staging environment for pre-production testing |
| `production` | Production workspaces with stricter controls |

```yaml
spec:
  environment: production
```

### `defaultTags`

Labels applied to all resources created in this workspace. Used for cost attribution and resource organization.

| Field | Type | Required |
|-------|------|----------|
| `defaultTags` | map[string]string | No |

```yaml
spec:
  defaultTags:
    team: "customer-support"
    cost-center: "CC-1234"
    business-unit: "support-ops"
```

### `namespace`

Kubernetes namespace configuration for the workspace.

| Field | Type | Required |
|-------|------|----------|
| `namespace.name` | string | Yes |
| `namespace.create` | boolean | No (default: false) |
| `namespace.labels` | map[string]string | No |
| `namespace.annotations` | map[string]string | No |

```yaml
spec:
  namespace:
    name: omnia-customer-support
    create: true
    labels:
      environment: production
    annotations:
      cost-center: "cc-12345"
```

### `roleBindings`

Maps IdP groups and ServiceAccounts to workspace roles. This is the primary mechanism for access control.

| Field | Type | Required |
|-------|------|----------|
| `roleBindings[].groups` | []string | No |
| `roleBindings[].serviceAccounts` | []ServiceAccountRef | No |
| `roleBindings[].role` | string | Yes |

ServiceAccountRef:

| Field | Type | Required |
|-------|------|----------|
| `name` | string | Yes |
| `namespace` | string | Yes |

Available roles:

| Role | Description |
|------|-------------|
| `owner` | Full workspace control including member management |
| `editor` | Create/modify resources but cannot manage members |
| `viewer` | Read-only access to resources |

```yaml
spec:
  roleBindings:
    # Map IdP groups to roles
    - groups:
        - "omnia-admins@acme.com"
      role: owner

    - groups:
        - "omnia-engineers@acme.com"
        - "engineering-team"
      role: editor

    - groups:
        - "contractors@acme.com"
      role: viewer

    # Grant access to ServiceAccounts for CI/CD
    - serviceAccounts:
        - name: github-actions
          namespace: ci-system
        - name: argocd-application-controller
          namespace: argocd
      role: editor
```

### `directGrants`

Direct user grants for exceptions. Use sparingly - prefer groups for scalability.

| Field | Type | Required |
|-------|------|----------|
| `directGrants[].user` | string | Yes |
| `directGrants[].role` | string | Yes |
| `directGrants[].expires` | string (RFC3339) | No |

```yaml
spec:
  directGrants:
    - user: emergency-admin@acme.com
      role: owner
      expires: "2026-02-01T00:00:00Z"  # Temporary access
```

### `anonymousAccess`

Configures access for unauthenticated users.

| Field | Type | Required |
|-------|------|----------|
| `anonymousAccess.enabled` | boolean | Yes |
| `anonymousAccess.role` | string | No (default: viewer) |

> **Warning**: Granting editor or owner access allows anonymous users to modify resources. Only use in isolated development environments.

```yaml
spec:
  anonymousAccess:
    enabled: true
    role: viewer  # Read-only for anonymous users
```

### `quotas`

Resource quotas for the workspace.

#### `quotas.compute`

Standard Kubernetes compute resource quotas.

| Field | Type | Description |
|-------|------|-------------|
| `requests.cpu` | string | Total CPU requests (e.g., "50") |
| `requests.memory` | string | Total memory requests (e.g., "100Gi") |
| `limits.cpu` | string | Total CPU limits (e.g., "100") |
| `limits.memory` | string | Total memory limits (e.g., "200Gi") |

```yaml
spec:
  quotas:
    compute:
      requests.cpu: "50"
      requests.memory: "100Gi"
      limits.cpu: "100"
      limits.memory: "200Gi"
```

#### `quotas.objects`

Object count quotas.

| Field | Type | Description |
|-------|------|-------------|
| `configmaps` | integer | Maximum number of ConfigMaps |
| `secrets` | integer | Maximum number of Secrets |
| `persistentvolumeclaims` | integer | Maximum number of PVCs |

```yaml
spec:
  quotas:
    objects:
      configmaps: 100
      secrets: 50
      persistentvolumeclaims: 20
```

#### `quotas.arena`

Arena-specific quotas.

| Field | Type | Description |
|-------|------|-------------|
| `maxConcurrentJobs` | integer | Maximum concurrent Arena jobs |
| `maxJobsPerDay` | integer | Maximum Arena jobs per day |
| `maxWorkersPerJob` | integer | Maximum workers per Arena job |

```yaml
spec:
  quotas:
    arena:
      maxConcurrentJobs: 10
      maxJobsPerDay: 100
      maxWorkersPerJob: 50
```

#### `quotas.agents`

AgentRuntime-specific quotas.

| Field | Type | Description |
|-------|------|-------------|
| `maxAgentRuntimes` | integer | Maximum number of AgentRuntimes |
| `maxReplicasPerAgent` | integer | Maximum replicas per AgentRuntime |

```yaml
spec:
  quotas:
    agents:
      maxAgentRuntimes: 20
      maxReplicasPerAgent: 10
```

### `networkPolicy`

Network isolation settings for the workspace. When enabled, automatically generates a Kubernetes NetworkPolicy to restrict traffic.

| Field | Type | Default | Required |
|-------|------|---------|----------|
| `networkPolicy.isolate` | boolean | false | No |
| `networkPolicy.allowExternalAPIs` | boolean | true | No |
| `networkPolicy.allowSharedNamespaces` | boolean | true | No |
| `networkPolicy.allowPrivateNetworks` | boolean | false | No |
| `networkPolicy.allowFrom` | []NetworkPolicyRule | [] | No |
| `networkPolicy.allowTo` | []NetworkPolicyRule | [] | No |

#### Default Behavior (when `isolate: true`)

- **DNS**: Always allows egress to `kube-system` on port 53 (UDP/TCP)
- **Same namespace**: Allows all ingress/egress within the workspace namespace
- **Shared namespaces**: Allows ingress/egress to namespaces labeled `omnia.altairalabs.ai/shared: true`
- **External APIs**: Allows egress to `0.0.0.0/0` excluding [RFC 1918](https://datatracker.ietf.org/doc/html/rfc1918) private IP ranges:
  - `10.0.0.0/8` - Class A private network
  - `172.16.0.0/12` - Class B private networks
  - `192.168.0.0/16` - Class C private networks

This allows agents to reach external LLM APIs while blocking access to other tenants' pods and internal cluster services.

#### Basic Isolation

```yaml
spec:
  networkPolicy:
    isolate: true
```

This creates a NetworkPolicy named `workspace-{name}-isolation` with default rules.

#### Restrict External Access

Disable external API access (blocks internet egress except DNS):

```yaml
spec:
  networkPolicy:
    isolate: true
    allowExternalAPIs: false
```

#### Allow Private Networks (Local Development)

For local development or when agents need to access services on private networks (e.g., local LLM servers), enable `allowPrivateNetworks` to remove the RFC 1918 exclusions:

```yaml
spec:
  networkPolicy:
    isolate: true
    allowPrivateNetworks: true  # Allows 10.x, 172.16.x, 192.168.x
```

:::caution
Enabling `allowPrivateNetworks` reduces tenant isolation. Only use this in development environments or when you explicitly need agents to access private network services.
:::

#### Custom Ingress Rules

Allow traffic from specific namespaces (e.g., ingress controller):

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

#### Custom Egress Rules

Allow egress to internal databases:

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

#### NetworkPolicyRule Structure

| Field | Type | Description |
|-------|------|-------------|
| `peers` | []NetworkPolicyPeer | Sources (ingress) or destinations (egress) |
| `ports` | []NetworkPolicyPort | Ports to allow (optional, all ports if omitted) |

#### NetworkPolicyPeer Structure

| Field | Type | Description |
|-------|------|-------------|
| `namespaceSelector.matchLabels` | map[string]string | Select namespaces by label |
| `podSelector.matchLabels` | map[string]string | Select pods by label |
| `ipBlock.cidr` | string | IP block in CIDR notation |
| `ipBlock.except` | []string | CIDRs to exclude from the block |

#### NetworkPolicyPort Structure

| Field | Type | Description |
|-------|------|-------------|
| `protocol` | string | TCP, UDP, or SCTP (default: TCP) |
| `port` | integer | Port number |

### `costControls`

Budget and cost control settings for the workspace.

| Field | Type | Default | Required |
|-------|------|---------|----------|
| `costControls.dailyBudget` | string | - | No |
| `costControls.monthlyBudget` | string | - | No |
| `costControls.budgetExceededAction` | string | warn | No |
| `costControls.alertThresholds` | []CostAlertThreshold | [] | No |

Budget values are in USD (e.g., "100.00", "2000.00").

#### Budget Exceeded Actions

| Value | Description |
|-------|-------------|
| `warn` | Log warnings when budget is exceeded |
| `pauseJobs` | Pause Arena jobs when budget is exceeded |
| `block` | Block new API requests when budget is exceeded |

```yaml
spec:
  costControls:
    dailyBudget: "100.00"
    monthlyBudget: "2000.00"
    budgetExceededAction: pauseJobs
    alertThresholds:
      - percent: 80
        notify:
          - "team-lead@acme.com"
      - percent: 95
        notify:
          - "team-lead@acme.com"
          - "finance@acme.com"
```

## Status Fields

### `phase`

Current lifecycle phase of the Workspace.

| Value | Description |
|-------|-------------|
| `Pending` | Workspace is being set up |
| `Ready` | Workspace is ready for use |
| `Suspended` | Workspace is suspended |
| `Error` | Workspace has an error |

### `observedGeneration`

Most recent generation observed by the controller.

### `namespace`

Namespace status information.

| Field | Description |
|-------|-------------|
| `status.namespace.name` | Name of the created namespace |
| `status.namespace.created` | Whether namespace was created by controller |

### `serviceAccounts`

ServiceAccounts created for this workspace.

| Field | Description |
|-------|-------------|
| `status.serviceAccounts.owner` | Name of the owner ServiceAccount |
| `status.serviceAccounts.editor` | Name of the editor ServiceAccount |
| `status.serviceAccounts.viewer` | Name of the viewer ServiceAccount |

### `members`

Member count by role.

| Field | Description |
|-------|-------------|
| `status.members.owners` | Count of owner members |
| `status.members.editors` | Count of editor members |
| `status.members.viewers` | Count of viewer members |

### `networkPolicy`

NetworkPolicy status information.

| Field | Description |
|-------|-------------|
| `status.networkPolicy.name` | Name of the generated NetworkPolicy |
| `status.networkPolicy.enabled` | Whether network isolation is active |
| `status.networkPolicy.rulesCount` | Total number of ingress and egress rules |

### `costUsage`

Current cost tracking information.

| Field | Description |
|-------|-------------|
| `status.costUsage.dailySpend` | Current day's spending in USD |
| `status.costUsage.dailyBudget` | Configured daily budget in USD |
| `status.costUsage.monthlySpend` | Current month's spending in USD |
| `status.costUsage.monthlyBudget` | Configured monthly budget in USD |
| `status.costUsage.lastUpdated` | Timestamp of last cost calculation |

### `conditions`

| Type | Description |
|------|-------------|
| `Ready` | Overall workspace readiness |
| `NamespaceReady` | Namespace is created and configured |
| `ServiceAccountsReady` | ServiceAccounts are created |
| `RoleBindingsReady` | RBAC resources are configured |
| `NetworkPolicyReady` | NetworkPolicy is configured (if enabled) |

## Complete Example

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

  roleBindings:
    # Owners: Full workspace control
    - groups:
        - "omnia-cs-admins@acme.com"
      role: owner

    # Editors: Create/modify resources
    - groups:
        - "omnia-cs-engineers@acme.com"
      role: editor

    # Viewers: Read-only
    - groups:
        - "omnia-cs-contractors@acme.com"
      role: viewer

    # CI/CD ServiceAccounts
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

    arena:
      maxConcurrentJobs: 10
      maxJobsPerDay: 100
      maxWorkersPerJob: 50

    agents:
      maxAgentRuntimes: 20
      maxReplicasPerAgent: 10

  # Network isolation
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

  # Cost controls
  costControls:
    monthlyBudget: "5000.00"
    budgetExceededAction: warn
    alertThresholds:
      - percent: 80
        notify:
          - "cs-admins@acme.com"
```

## Dashboard Integration

The dashboard provides a workspace switcher for managing multiple workspaces. Users see only the workspaces they have access to, based on their IdP group membership.

### Workspace API Endpoints

The dashboard uses workspace-scoped API endpoints:

| Endpoint | Description |
|----------|-------------|
| `GET /api/workspaces` | List accessible workspaces |
| `GET /api/workspaces/{name}` | Get workspace details |
| `GET /api/workspaces/{name}/agents` | List agents in workspace |
| `GET /api/workspaces/{name}/promptpacks` | List prompt packs in workspace |
| `GET /api/workspaces/{name}/agents/{agentName}/logs` | Get agent logs |
| `GET /api/workspaces/{name}/agents/{agentName}/events` | Get agent events |

### ServiceAccount Token Management

The dashboard manages ServiceAccount tokens for workspace-scoped K8s API access. Each workspace has three ServiceAccounts (owner, editor, viewer) with corresponding RBAC permissions. The dashboard fetches short-lived tokens to make API calls with the appropriate permission level.

## Authorization Flow

1. User authenticates via OIDC (Okta, Azure AD, Google)
2. JWT contains claims: `{ email, groups: ["group1", "group2", ...] }`
3. Dashboard checks which groups are in workspace `roleBindings`
4. Grants highest privilege role found
5. Makes K8s API calls using workspace ServiceAccount token

This design keeps the Workspace CRD small (10-20 groups) even with 10,000+ users. User management happens in your IdP, not in Kubernetes.
