# Omnia Helm chart

Deploys [Omnia](https://omnia.altairalabs.ai) — a Kubernetes operator for AI agent orchestration — to a Kubernetes cluster.

This chart installs:

- **Operator** (`cmd/main.go`) — controller-manager reconciling AgentRuntime, PromptPack, ToolRegistry, Provider, Workspace, and SessionRetentionPolicy CRDs. Also serves the dashboard and REST API.
- **Dashboard** (`dashboard/`) — Next.js frontend for agent consoles, session browsing, and admin.
- **Eval worker** (per-namespace) — executes realtime evals against active agent sessions.
- **Session retention** — default `SessionRetentionPolicy` (cluster default).
- **Optional: Arena / Enterprise** — prompt testing, evaluation, dev-console, policy proxy (`enterprise.enabled: true`).
- **Optional: observability subcharts** — Prometheus, Grafana, Loki, Tempo, Alloy (all off by default).
- **Optional: workload plumbing** — NFS server/CSI for workspace content, KEDA for autoscaling, Redis for Arena queue.

The chart **does not** include sample AgentRuntime/Workspace CRs — you create those.

---

## Minimum install

Every install **must** set these two values. The chart refuses to render without them.

| Value | Why |
|---|---|
| `dashboard.auth.mode` | One of `oauth`, `builtin`, `proxy`, `anonymous`. No default — prevents accidental unauthenticated deploys. |
| `dashboard.auth.allowAnonymous: true` | Only required when `mode: anonymous`. Explicit opt-in. |

Smallest possible install (local kind/k3d, no auth):

```bash
helm install omnia oci://ghcr.io/altairalabs/charts/omnia \
  --set dashboard.auth.mode=anonymous \
  --set dashboard.auth.allowAnonymous=true
```

For a real cluster you also need Postgres access for the session store and usually a storage class for workspace content. See the three deployment profiles below.

---

## Deployment profiles

Three starting points, from least to most featureful. Each points at a `values-*.yaml` file in this directory (examples coming in a follow-up PR — tracked in [#895](https://github.com/AltairaLabs/Omnia/issues/895)).

### 1. Development (single machine, kind/k3d/Docker Desktop)

Goal: install Omnia with zero external dependencies. Dev Postgres + NFS server both deployed inline.

```bash
helm install omnia oci://ghcr.io/altairalabs/charts/omnia \
  --values values-dev.yaml \
  --set dashboard.auth.mode=anonymous \
  --set dashboard.auth.allowAnonymous=true
```

What you get:
- Operator (1 replica), Dashboard (1 replica, anonymous auth).
- Dev Postgres (in-cluster, NOT production-safe — see `postgres.dev`).
- NFS server + CSI driver for workspace content (dev-only).
- No enterprise, no observability, no license.

### 2. Production (OSS, no Arena)

Goal: multi-replica operator/dashboard on real infra, OAuth, external Postgres, external storage class for workspace content.

Required values file (create as `values-prod.yaml`):

```yaml
# Auth
dashboard:
  auth:
    mode: oauth
  oauth:
    provider: github        # or google, okta, azure, generic
    clientId: <redacted>
    clientIdExistingSecret: omnia-oauth
    clientSecretExistingSecret: omnia-oauth

# Session store
postgres:
  dev:
    enabled: false    # use external Postgres via Workspace CRs instead

# Workspace content (RWX required — use EFS / Azure Files / Filestore)
workspaceContent:
  enabled: true
  persistence:
    storageClassName: efs-sc    # AWS example
    size: 100Gi

# High availability
replicaCount: 3
dashboard:
  replicaCount: 2
  podDisruptionBudget:
    enabled: true
```

Install:

```bash
helm install omnia oci://ghcr.io/altairalabs/charts/omnia --values values-prod.yaml
```

### 3. Enterprise (Arena + advanced features)

Requires an Omnia enterprise license. Add on top of the production profile:

```yaml
license:
  existingSecret: omnia-license

enterprise:
  enabled: true

  # Community templates source reaches out to github.com on each sync.
  # Disabled by default (0.9.0-beta.7+) for air-gap safety. Enable if you
  # want the gallery:
  communityTemplates:
    enabled: true

  arena:
    queue:
      type: redis
      redis:
        host: "omnia-redis-master"
        port: 6379

redis:
  enabled: true
```

Install:

```bash
helm install omnia oci://ghcr.io/altairalabs/charts/omnia \
  --values values-prod.yaml \
  --values values-enterprise.yaml
```

---

## Cloud-native add-ons

Plug Omnia into your cloud's native identity and secret-store features via `PodOverrides` on per-workspace and per-agent CRs. See the [Configure Pod Overrides](https://omnia.altairalabs.ai/docs/how-to/configure-pod-overrides) how-to for the full reference.

Short summary of placements:

| CRD | Field | Common use |
|---|---|---|
| `AgentRuntime.spec.podOverrides` | facade + runtime pod | workload identity SA, CSI-mounted provider keys, GPU nodeSelector |
| `AgentRuntime.spec.evals.podOverrides` | eval-worker (per ns) | batch-node affinity |
| `Workspace.spec.services[].session.podOverrides` | session-api | CSI-synced DB password via envFrom |
| `Workspace.spec.services[].memory.podOverrides` | memory-api | CSI-synced embedding-provider key |
| `ArenaJob.spec.workers.podOverrides` | worker Jobs | per-job provider credential CSI mount |
| `ArenaDevSession.spec.podOverrides` | dev console | GPU scheduling |

For chart-owned deployments (operator, dashboard, arena-controller) use the inline `extraEnv` / `extraEnvFrom` / `extraVolumes` / `extraVolumeMounts` values. A unified `podOverrides:` block for these is planned in the chart overhaul ([#895 W4](https://github.com/AltairaLabs/Omnia/issues/895)).

---

## Upgrading

Before each upgrade:

1. Re-read the [CHANGELOG](https://github.com/AltairaLabs/Omnia/releases) for the target version.
2. Run `helm diff upgrade omnia <chart> --values values-*.yaml` (install the `helm-diff` plugin).
3. Back up the session-api database.

CRD changes ship with new chart versions. Since Helm does **not** upgrade CRDs in `crds/` automatically, apply them separately:

```bash
kubectl apply --server-side --force-conflicts -f charts/omnia/crds/
# Enterprise CRDs (if enterprise.enabled):
helm template omnia <chart> -s templates/enterprise/omnia.altairalabs.ai_arenajobs.yaml | kubectl apply --server-side -f -
# ...repeat for arenadevsessions, arenasources, arenatemplatesources, rolloutanalyses, sessionprivacypolicies
```

`--server-side` is required because the CRDs embed `corev1.Volume`/`Affinity`/`Toleration` (via `PodOverrides`) and exceed the 262144-byte client-side `last-applied-configuration` annotation limit.

---

## Uninstalling

```bash
helm uninstall omnia
```

This removes all resources **except**:
- CRDs (by Helm default; if you want them gone: `kubectl delete -f charts/omnia/crds/`)
- PersistentVolumeClaims (keep data; delete manually if you're sure)
- `omnia-system` namespace (if the chart created it)

---

## Gotchas

A selection of things that will bite you if you don't know about them:

- **`dashboard.auth.mode` is required.** The chart intentionally has no default — avoids accidentally deploying with no auth.
- **`workspaceContent.enabled: true` requires ReadWriteMany storage.** Azure Disk, AWS EBS will fail. Use Azure Files, EFS, Filestore, NFS, or an RWX CSI driver. Set `workspaces.storage.storageClass` accordingly.
- **Single-replica PDBs are refused.** `podDisruptionBudget.enabled=true` with `replicaCount: 1` renders a template `fail` — otherwise every drain/rollout would be permanently blocked. Either raise replicas or set `podDisruptionBudget.maxUnavailable: 1`.
- **Dev Postgres (`postgres.dev.enabled`) is dev-only.** No volume persistence by default; restart loses data. Production installs MUST use external Postgres via a Secret referenced by the `Workspace` CR.
- **Community templates (`enterprise.communityTemplates.enabled`) reach out to GitHub.** Disabled by default (0.9.0-beta.7+) for air-gap safety. Dev-enterprise values file turns it on explicitly.
- **Enterprise CRDs ship in `templates/enterprise/` (not `crds/`) so they're gated by `enterprise.enabled`.** Users who later flip `enterprise.enabled: true` need a fresh `helm upgrade` to get the CRDs — or apply them directly.
- **Large CRDs overflow client-side `kubectl apply`.** Always use `kubectl apply --server-side --force-conflicts -f charts/omnia/crds/` when installing CRDs manually. `make install` does this correctly.
- **Istio + Prometheus metric merging** requires both `prometheus.io/*` pod annotations (for direct scrape) AND `prometheus.istio.io/merge-metrics: "true"` (for sidecar merging). The operator deployment sets both.
- **Grafana dashboard ConfigMaps render only when `grafana.enabled`.** They're labeled for discovery by Grafana's sidecar — harmless if missing, but not orphaned.

---

## Values reference

A full values table is generated from `values.yaml` comments. Browse:

- [`values.yaml`](./values.yaml) — canonical source with inline documentation for every knob
- [`values.schema.json`](./values.schema.json) — JSON Schema validation (completion in progress — [#895 W2](https://github.com/AltairaLabs/Omnia/issues/895))
- [`values-dev.yaml`](./values-dev.yaml) — local development
- [`values-dev-enterprise.yaml`](./values-dev-enterprise.yaml) — local development + enterprise
- [`values-demo-observability.yaml`](./values-demo-observability.yaml) — all observability subcharts on

More worked examples (Azure KV CSI, AWS IRSA, GKE Workload Identity) coming in [#895 W5](https://github.com/AltairaLabs/Omnia/issues/895).

---

## Contributing

See the [chart overhaul proposal](https://github.com/AltairaLabs/Omnia/issues/895) for the active improvement plan. File issues at https://github.com/AltairaLabs/Omnia/issues.

Pre-commit checks for chart changes:

- `helm lint --strict charts/omnia`
- `hack/validate-helm.sh` (helm template renders with default and enterprise values)
- `helm unittest charts/omnia` (local only; CI coverage coming in [#895 W6](https://github.com/AltairaLabs/Omnia/issues/895))
