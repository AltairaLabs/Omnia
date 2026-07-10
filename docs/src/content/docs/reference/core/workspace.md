---
title: "Workspace CRD"
description: "Complete reference for the Workspace custom resource"
sidebar:
  order: 10
---

The Workspace custom resource defines a multi-tenant workspace with isolated namespace, RBAC, and network isolation in Kubernetes.

## API version

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Workspace
```

**Short name:** `ws` (e.g. `kubectl get ws`).

## Resource scope

Workspace is a **cluster-scoped** resource. It creates and manages resources in its associated namespace.

## Spec fields

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

### `runtime`

Workspace-wide pod defaults applied to **every AgentRuntime** in the workspace. Its purpose is hyperscaler-agnostic **cloud workload-identity**: the ServiceAccount and pod labels needed to bind a runtime pod to a cloud workload identity (Azure Workload Identity, AWS IRSA, GKE Workload Identity) so keyless providers (`Provider.auth.type: workloadIdentity`) authenticate without a stored secret.

| Field | Type | Required |
|-------|------|----------|
| `runtime.serviceAccountName` | string | No |
| `runtime.podLabels` | map[string]string | No |
| `runtime.podAnnotations` | map[string]string | No |

| Field | Description |
|-------|-------------|
| `serviceAccountName` | The ServiceAccount every agent runtime pod in this workspace runs as. Provisioned out of band (IaC) with the cloud identity annotations (e.g. `azure.workload.identity/client-id`, `eks.amazonaws.com/role-arn`, `iam.gke.io/gcp-service-account`). Empty = no default; agents fall back to the operator-created per-agent SA. |
| `podLabels` | Labels added to every agent runtime pod, e.g. `azure.workload.identity/use: "true"` to opt into the Azure webhook. AWS IRSA and GKE WLI need none. |
| `podAnnotations` | Annotations added to every agent runtime pod. Reserved for parity with `podLabels`; rarely needed. |

```yaml
spec:
  runtime:
    serviceAccountName: ws-runtime-sa
    podLabels:
      azure.workload.identity/use: "true"
```

**Precedence.** An AgentRuntime that sets its own `spec.podOverrides.serviceAccountName` is bringing its own identity (its own annotated SA), so it opts **out** of these defaults as a unit — neither the workspace SA nor the workspace pod labels are applied to it. Agents that set no SA inherit these defaults. This lets agents provisioned via the deploy API (which can't carry cloud-specific SA names) still authenticate to keyless providers.

#### Workload-identity federation is provisioned out of band (IaC)

Omnia treats `runtime` as **opaque passthrough** — it never interprets the values or branches on cloud provider. The operator references the ServiceAccount by name and applies the pod labels, but it does **not** create the cloud-side federated trust (Azure FIC, AWS IRSA role/trust policy, or GKE WLI binding).

Each cloud's trust statement is **per-exact-subject** — `system:serviceaccount:<namespace>:<sa>` on Azure, the role trust policy on AWS, a specific KSA↔GSA binding on GKE — and there are no wildcard subjects. Because Omnia isolates each workspace in its own namespace, **every workspace needs its own federated credential**, even when sharing one managed identity and one SA name.

:::caution[Keyless providers require an IaC-provisioned workspace]
The annotated ServiceAccount **and** the cloud-side federated credential must be provisioned out of band by the same IaC (Terraform/Bicep/Helm/GitOps) that creates the `Workspace`. For a **dynamically created** workspace (dashboard UI / API → operator creates the namespace at runtime) there is no IaC step in that path, so nothing provisions the cloud-side credential for the new workspace's runtime SA.

Consequence: agents in a UI-created workspace **cannot** use `auth.type: workloadIdentity` until the FIC / IRSA role / WLI binding is created out of band. For dynamic workspaces, use **secret-based provider auth** (`servicePrincipal`, `accessKey`, or `serviceAccount`) instead, or pre-provision a shared federated ServiceAccount that the workspace references.
:::

### `services`

Named service groups for the workspace. Each group bundles the session-api and
memory-api endpoints (and related defaults) that its agents use; an AgentRuntime
selects a group via `spec.serviceGroup` (defaulting to `default`). A group also
carries **agent-runtime defaults** that its agents inherit.

#### `services[].autoscaling`

Default autoscaling policy for **every AgentRuntime in this service group**. An
agent that omits `spec.runtime.autoscaling` inherits this policy whole; an agent
that sets its own block fully owns autoscaling and this default is ignored
(explicit agent spec wins as a unit). This lets a workspace owner turn on
"autoscaling by default" once — including for agents created via the dashboard or
a deploy tool, which can't easily express per-agent scaling.

The value is an [`AutoscalingConfig`](/reference/core/agentruntime/#runtimeautoscaling)
— the same shape as `AgentRuntime.spec.runtime.autoscaling`.

```yaml
spec:
  services:
    - name: default
      autoscaling:
        enabled: true
        type: hpa
        minReplicas: 1
        maxReplicas: 10
        targetMemoryUtilizationPercentage: 70
```

An AgentRuntime in this group with no `spec.runtime.autoscaling` inherits the
above; one that sets its own keeps it.

:::caution[KEDA must be installed for `type: keda`]
KEDA is not installed by the chart by default. When a resolved default requests
`type: keda` but the KEDA CRDs are absent, the agent surfaces an
`AutoscalingReady=False` condition with reason `KEDANotInstalled` and stays at
static replicas — the reconcile does not fail. Use `type: hpa` (the default) on
clusters without KEDA.
:::

#### `services[].privacyPolicyRef`

References a `SessionPrivacyPolicy` in this workspace's namespace that applies to
**all agents in this service group**. An individual AgentRuntime can override it
with its own `spec.privacyPolicyRef`.

| Field | Type | Required |
|-------|------|----------|
| `services[].privacyPolicyRef.name` | string | No |

```yaml
spec:
  services:
    - name: default
      privacyPolicyRef:
        name: gdpr-compliant
```

See the [SessionPrivacyPolicy CRD](/reference/policies/sessionprivacypolicy/) reference and
[Configure Privacy Policies](/how-to/privacy/configure-privacy-policies/) for policy
resolution order and per-agent overrides.

### `privacy`

:::note[Enterprise Feature]
The per-workspace privacy-api service is an Enterprise feature. It requires an
active Enterprise license — see [Install an Enterprise License](/how-to/operations/install-license/).
:::

Configures the per-workspace **privacy-api** service, which owns per-user consent
grants and opt-out preferences for the workspace. **Setting `spec.privacy` is the
trigger that provisions the privacy-api** for this workspace: when it is present,
the operator deploys a privacy-api instance backed by the consent database you
reference here; when it is omitted, no privacy-api is provisioned and the
workspace's session/memory services run without centralized preference
enforcement, consent tracking, the compliance audit hub, or DSAR erasure.

| Field | Type | Required |
|-------|------|----------|
| `privacy.database.secretRef.name` | string | Yes |

`privacy.database` points at the PostgreSQL **consent database** for this
workspace (one database per workspace). The referenced Secret must contain a
`POSTGRES_CONN` key holding a valid PostgreSQL connection string.

```yaml
spec:
  privacy:
    database:
      secretRef:
        name: my-workspace-privacy-db
```

Once provisioned, the privacy-api's resolved URL is published on
[`status.privacyURL`](#privacyurl). To manage consent and opt-out preferences
against it, see [Manage User Consent](/how-to/privacy/manage-user-consent/); to submit
right-to-erasure requests, see [Handle Data Subject Erasure](/how-to/privacy/handle-data-subject-erasure/).

### `quotas` — not implemented

:::caution
`spec.quotas` is **not a field on the Workspace CRD** and the controller enforces no
resource quotas. Applying a `quotas` block is rejected by the API server with
`strict decoding error: unknown field "spec.quotas"`. To limit resource usage, apply a
native Kubernetes [`ResourceQuota`](https://kubernetes.io/docs/concepts/policy/resource-quotas/)
to the workspace namespace directly — see
[Limit resource usage](/how-to/workspaces/manage-workspaces/#limit-resource-usage).
Workspace-native quotas are tracked in
[issue #1781](https://github.com/AltairaLabs/Omnia/issues/1781).
:::

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

#### Default behavior (when `isolate: true`)

- **DNS**: Always allows egress to `kube-system` on port 53 (UDP/TCP)
- **Same namespace**: Allows all ingress/egress within the workspace namespace
- **Shared namespaces**: Allows ingress/egress to namespaces labeled `omnia.altairalabs.ai/shared: true`
- **External APIs**: Allows egress to `0.0.0.0/0` excluding [RFC 1918](https://datatracker.ietf.org/doc/html/rfc1918) private IP ranges:
  - `10.0.0.0/8` - Class A private network
  - `172.16.0.0/12` - Class B private networks
  - `192.168.0.0/16` - Class C private networks

This allows agents to reach external LLM APIs while blocking access to other tenants' pods and internal cluster services.

#### Basic isolation

```yaml
spec:
  networkPolicy:
    isolate: true
```

This creates a NetworkPolicy named `workspace-{name}-isolation` with default rules.

#### Restrict external access

Disable external API access (blocks internet egress except DNS):

```yaml
spec:
  networkPolicy:
    isolate: true
    allowExternalAPIs: false
```

#### Allow private networks (local development)

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

#### Custom ingress rules

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

#### Custom egress rules

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

#### NetworkPolicyRule structure

| Field | Type | Description |
|-------|------|-------------|
| `peers` | []NetworkPolicyPeer | Sources (ingress) or destinations (egress) |
| `ports` | []NetworkPolicyPort | Ports to allow (optional, all ports if omitted) |

#### NetworkPolicyPeer structure

| Field | Type | Description |
|-------|------|-------------|
| `namespaceSelector.matchLabels` | map[string]string | Select namespaces by label |
| `podSelector.matchLabels` | map[string]string | Select pods by label |
| `ipBlock.cidr` | string | IP block in CIDR notation |
| `ipBlock.except` | []string | CIDRs to exclude from the block |

#### NetworkPolicyPort structure

| Field | Type | Description |
|-------|------|-------------|
| `protocol` | string | TCP, UDP, or SCTP (default: TCP) |
| `port` | integer | Port number |

### `costControls`

Budget and cost control settings for the workspace.

:::note
`costControls` is accepted and stored on the CRD, but the Workspace controller does **not
yet enforce it** — it does not populate [`status.costUsage`](#costusage) or apply
`budgetExceededAction`. Treat this as declarative intent until enforcement lands
([issue #1781](https://github.com/AltairaLabs/Omnia/issues/1781)).
:::

| Field | Type | Default | Required |
|-------|------|---------|----------|
| `costControls.dailyBudget` | string | - | No |
| `costControls.monthlyBudget` | string | - | No |
| `costControls.budgetExceededAction` | string | warn | No |
| `costControls.alertThresholds` | []CostAlertThreshold | [] | No |

Budget values are in USD (e.g., "100.00", "2000.00").

#### Budget exceeded actions

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

## Status fields

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

:::note
Not currently populated — the controller does not yet compute cost usage (see
[`costControls`](#costcontrols) and [issue #1781](https://github.com/AltairaLabs/Omnia/issues/1781)).
:::

| Field | Description |
|-------|-------------|
| `status.costUsage.dailySpend` | Current day's spending in USD |
| `status.costUsage.dailyBudget` | Configured daily budget in USD |
| `status.costUsage.monthlySpend` | Current month's spending in USD |
| `status.costUsage.monthlyBudget` | Configured monthly budget in USD |
| `status.costUsage.lastUpdated` | Timestamp of last cost calculation |

### `privacyURL`

Resolved URL of the per-workspace privacy-api. Populated only when
[`spec.privacy`](#privacy) is set; empty otherwise.

| Field | Description |
|-------|-------------|
| `status.privacyURL` | In-cluster base URL of the provisioned privacy-api service |

### `conditions`

| Type | Description |
|------|-------------|
| `Ready` | Overall workspace readiness |
| `NamespaceReady` | Namespace is created and configured |
| `ServiceAccountsReady` | ServiceAccounts are created |
| `RoleBindingsReady` | RBAC resources are configured |
| `NetworkPolicyReady` | NetworkPolicy is configured (if enabled) |

## Complete example

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

  # Workspace-wide cloud workload identity (keyless providers).
  # The SA + cloud-side federated credential must be provisioned by IaC.
  runtime:
    serviceAccountName: ws-runtime-sa
    podLabels:
      azure.workload.identity/use: "true"

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

## Dashboard integration

The dashboard provides a workspace switcher for managing multiple workspaces. Users see only the workspaces they have access to, based on their IdP group membership.

### Workspace API endpoints

The dashboard uses workspace-scoped API endpoints:

| Endpoint | Description |
|----------|-------------|
| `GET /api/workspaces` | List accessible workspaces |
| `GET /api/workspaces/{name}` | Get workspace details |
| `GET /api/workspaces/{name}/agents` | List agents in workspace |
| `GET /api/workspaces/{name}/promptpacks` | List prompt packs in workspace |
| `GET /api/workspaces/{name}/agents/{agentName}/logs` | Get agent logs |
| `GET /api/workspaces/{name}/agents/{agentName}/events` | Get agent events |

### ServiceAccount token management

The dashboard manages ServiceAccount tokens for workspace-scoped K8s API access. Each workspace has three ServiceAccounts (owner, editor, viewer) with corresponding RBAC permissions. The dashboard fetches short-lived tokens to make API calls with the appropriate permission level.

## Authorization flow

1. User authenticates via OIDC (Okta, Azure AD, Google)
2. JWT contains claims: `{ email, groups: ["group1", "group2", ...] }`
3. Dashboard checks which groups are in workspace `roleBindings`
4. Grants highest privilege role found
5. Makes K8s API calls using workspace ServiceAccount token

This design keeps the Workspace CRD small (10-20 groups) even with 10,000+ users. User management happens in your IdP, not in Kubernetes.
