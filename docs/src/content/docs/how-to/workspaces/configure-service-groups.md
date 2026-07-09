---
title: "Configure workspace service groups"
description: "Provision the session-api and memory-api backends that agents in a workspace use"
sidebar:
  order: 9
---

A **service group** is the set of backend services — session-api and memory-api — that the
agents in a workspace use to persist conversations and cross-session memory. Every
AgentRuntime selects a group by name (`spec.serviceGroup`, defaulting to `default`); the
group's endpoints are resolved from the workspace's `status.services[]`.

:::caution[Agents need a service group to persist anything]
There is **no cluster-wide default session/memory endpoint**. If an agent's `serviceGroup`
doesn't resolve to a ready group, the agent still starts but **silently falls back to an
in-memory session store** — conversations, token usage, and cost are **not persisted**, and
memory is unavailable. A workspace intended to run real agents needs at least one service
group (conventionally named `default`).
:::

## Prerequisites

- A [workspace](/how-to/workspaces/manage-workspaces/) (the group's services are deployed
  into the workspace namespace).
- A reachable PostgreSQL instance and **two databases** — one for session data, one for
  memory. (You can point both at the same server; session-api and memory-api key all rows
  by workspace, so one server can back many workspaces.)

## Provision a managed service group

In `managed` mode (the default) the operator deploys and manages session-api and memory-api
for you.

### 1. Create the database secrets

Each service references a Kubernetes Secret containing a single **`POSTGRES_CONN`** key with
a PostgreSQL connection string. Create one for session and one for memory, **in the
workspace namespace**:

```bash
kubectl create secret generic customer-support-session-db \
  -n omnia-customer-support \
  --from-literal=POSTGRES_CONN='postgres://USER:PASSWORD@HOST:5432/sessions?sslmode=require'

kubectl create secret generic customer-support-memory-db \
  -n omnia-customer-support \
  --from-literal=POSTGRES_CONN='postgres://USER:PASSWORD@HOST:5432/memory?sslmode=require'
```

:::tip
Store real connection strings in your secret manager (e.g. External Secrets, CSI driver)
rather than creating them with `kubectl`. The only contract is a Secret in the workspace
namespace with a `POSTGRES_CONN` key.
:::

### 2. Add the service group to the workspace

:::caution[Add this to your *existing* workspace — don't create a new one]
If you already have a workspace (the one your agents deploy into), add the `services:` block
to **it** — `kubectl edit workspace <your-workspace>` — rather than applying this as a new
`Workspace`. A namespace can be owned by **only one** workspace: pointing a second workspace
at an existing namespace collides with the first, producing duplicate session/memory
deployments and agents that resolve the wrong backend. The manifest below shows a full
from-scratch workspace; **if yours already exists, copy only the `services:` block into it.**
:::

A `managed` group **must** define both `session` and `memory` (the CRD rejects a managed
group missing either):

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
  services:
    - name: default          # AgentRuntimes reference this via spec.serviceGroup
      mode: managed
      session:
        database:
          secretRef:
            name: customer-support-session-db
      memory:
        database:
          secretRef:
            name: customer-support-memory-db
```

Apply it:

```bash
kubectl apply -f workspace.yaml
```

For each managed group the operator creates, in the workspace namespace:

- a **session-api** Deployment + Service named `session-<workspace>-<group>` (port 8080)
- a **memory-api** Deployment + Service named `memory-<workspace>-<group>` (port 8080)
- the ServiceAccounts and RBAC those pods need (including read access to the DB secret)

### 3. Verify

The group is `ready` only once **both** the session and memory Deployments have a ready
replica:

```bash
kubectl get deploy -n omnia-customer-support
# session-customer-support-default   1/1
# memory-customer-support-default    1/1

kubectl get workspace customer-support -o jsonpath='{.status.services}' | jq
```

```json
[
  {
    "name": "default",
    "ready": true,
    "sessionURL": "http://session-customer-support-default.omnia-customer-support:8080",
    "memoryURL": "http://memory-customer-support-default.omnia-customer-support:8080"
  }
]
```

The workspace also surfaces a `ServicesReady` condition (`True` only when every group is
ready). If a group is stuck, check the session/memory pod logs — a bad `POSTGRES_CONN` (wrong
host, database doesn't exist, auth failure) is the usual cause of a Deployment that never
becomes ready.

## Point agents at the group

An AgentRuntime selects its service group by name; omit it to use `default`:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: support-bot
  namespace: omnia-customer-support
spec:
  serviceGroup: default   # must match a spec.services[].name on the workspace
  # ...
```

If `serviceGroup` names a group that isn't in the workspace's `status.services[]` (or the
group isn't ready), the agent logs the error and degrades to a non-persistent in-memory
session store — see the caution above.

## Optional configuration

Each managed group accepts several optional fields (see the
[Workspace CRD reference](/reference/core/workspace/#services)):

- **`memory.providerRef`** — reference a [Provider](/reference/core/provider/) to enable
  embedding-based semantic memory. **Without it, memory is lexical-only** (keyword recall,
  no vector search).
- **`session.policyRef` / `memory.policyRef`** — attach a
  [SessionRetentionPolicy](/reference/policies/sessionretentionpolicy/) or
  [MemoryPolicy](/reference/policies/memorypolicy/) to the group.
- **`redis`** (group-level, or per `session`/`memory`) — pin the group's cache to a specific
  existing Redis. This is a **reference** to a Redis you already run — the operator does not
  provision one. Unset means the operator-wide default.
- **`privacyPolicyRef`** — apply a
  [SessionPrivacyPolicy](/reference/policies/sessionprivacypolicy/) to every agent in the
  group.
- **`autoscaling`** — a default autoscaling policy inherited by every AgentRuntime in the
  group (see [autoscaling](/explanation/agents/autoscaling/)).
- **`podOverrides`** on `session`/`memory` — customize the managed pods (ServiceAccount,
  scheduling, CSI secret stores, workload-identity labels).

## Use external services instead

Set `mode: external` to point a group at session-api / memory-api you already run elsewhere
(a shared/central instance, another cluster, or a hosted service). The operator creates **no**
Deployments — it just publishes the URLs you supply:

```yaml
spec:
  services:
    - name: default
      mode: external
      external:
        sessionURL: https://session.internal.example.com
        memoryURL: https://memory.internal.example.com
```

Both URLs are required and must be `http(s)://`. An external group reports `ready: true` as
soon as it's applied (the operator does not health-check the endpoints).

## Per-workspace privacy service (Enterprise)

Setting `spec.privacy` provisions a per-workspace **privacy-api** (consent, opt-out, the
compliance audit hub, and DSAR erasure). It needs its own consent database secret
(`POSTGRES_CONN`) and requires an Enterprise license. See
[Configure privacy policies](/how-to/privacy/configure-privacy-policies/) and
[Manage user consent](/how-to/privacy/manage-user-consent/).

```yaml
spec:
  privacy:
    database:
      secretRef:
        name: customer-support-privacy-db
```

:::note
session-api and memory-api are deployed for every managed group regardless of edition, but
their **audit, privacy, and advanced-memory** behavior is gated by the operator's
`ENTERPRISE_ENABLED` license mode. The privacy-api component itself is Enterprise-only.
:::

## Next steps

- [Manage workspaces](/how-to/workspaces/manage-workspaces/) — namespace, RBAC, access control
- [Workspace CRD reference](/reference/core/workspace/#services) — every `services[]` field
- [Isolate workspace content](/how-to/workspaces/isolate-workspace-content/) — shared storage
