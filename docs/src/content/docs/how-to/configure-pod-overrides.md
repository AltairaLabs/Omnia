---
title: "Configure Pod Overrides"
description: "Inject scheduling, workload identity, and CSI secret-store hooks into operator-generated Pods"
sidebar:
  order: 30
---

`podOverrides` is a shared, optional block present on every Omnia CRD that the operator uses to generate Pods. It unblocks CSI secret-store drivers (Azure Key Vault, AWS Secrets Manager, GCP Secret Manager, Vault), workload identity (IRSA, AAD Workload Identity, GKE WLI), GPU scheduling, Istio injection, and image-pull secrets without requiring primer pods or fork-and-patch workarounds.

## Supported placements

| CRD | Field | Affected Pod |
|---|---|---|
| `AgentRuntime` | `spec.podOverrides` | facade + runtime |
| `AgentRuntime` | `spec.evals.podOverrides` | namespace-level eval-worker |
| `Workspace` | `spec.services[].session.podOverrides` | managed session-api |
| `Workspace` | `spec.services[].memory.podOverrides` | managed memory-api |
| `ArenaJob` | `spec.workers.podOverrides` | worker Jobs |
| `ArenaDevSession` | `spec.podOverrides` | dev-console |

## Merge semantics

- **Labels:** operator-set labels always win on key collision (Service selectors depend on them).
- **Annotations:** your values win on key collision.
- **NodeSelector:** merged by key; your values win on collision.
- **Tolerations / topologySpreadConstraints / imagePullSecrets / extraVolumes / extraVolumeMounts / extraEnv / extraEnvFrom:** appended.
- **Affinity / priorityClassName / serviceAccountName:** your value replaces the operator default when non-empty.

For multi-container pods (`AgentRuntime` facade + runtime), container-scoped overrides (`extraEnv`, `extraEnvFrom`, `extraVolumeMounts`) apply to both user containers but skip operator-injected sidecars such as the enterprise `policy-proxy`. Per-container env remains available via `spec.facade.extraEnv` and `spec.runtime.extraEnv`.

## Example — Azure Key Vault via CSI (Workspace session-api)

Sync database credentials from Azure Key Vault into the session-api Pod without a primer-Pod workaround:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Workspace
metadata:
  name: prod
spec:
  services:
    - name: default
      mode: managed
      session:
        database:
          secretRef:
            name: session-db
        podOverrides:
          serviceAccountName: workload-identity-sa
          annotations:
            azure.workload.identity/use: "true"
          extraVolumes:
            - name: kv-secrets
              csi:
                driver: secrets-store.csi.k8s.io
                readOnly: true
                volumeAttributes:
                  secretProviderClass: session-db-spc
          extraVolumeMounts:
            - name: kv-secrets
              mountPath: /mnt/secrets-store
              readOnly: true
          extraEnvFrom:
            - secretRef:
                name: session-db   # synced by SPC.secretObjects
```

## Example — GPU runtime + IRSA (AgentRuntime)

Run a vision agent on GPU nodes with AWS IRSA-federated S3 access:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: vision-agent
spec:
  promptPackRef:
    name: vision
  facade:
    type: websocket
  podOverrides:
    serviceAccountName: vision-irsa
    nodeSelector:
      nvidia.com/gpu.product: A100
    tolerations:
      - key: nvidia.com/gpu
        operator: Exists
        effect: NoSchedule
    imagePullSecrets:
      - name: ecr-pull
```

## Example — Arena worker with shared CSI credentials

Give every worker in an ArenaJob pool access to a CSI-synced provider secret:

```yaml
apiVersion: arena.omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: eval-run-1
spec:
  sourceRef:
    name: my-arena-source
  workers:
    replicas: 5
    podOverrides:
      extraVolumes:
        - name: provider-keys
          csi:
            driver: secrets-store.csi.k8s.io
            readOnly: true
            volumeAttributes:
              secretProviderClass: arena-providers
      extraEnvFrom:
        - secretRef:
            name: arena-provider-keys
```

## Interaction with existing fields

- `AgentRuntime.spec.runtime.nodeSelector` / `.tolerations` / `.affinity` are still honored. `spec.podOverrides.*` values are merged **after** them (append for slices, key-wise merge for maps, replacement for scalars).
- `AgentRuntime.spec.facade.extraEnv` and `AgentRuntime.spec.runtime.extraEnv` continue to target each container specifically; prefer them for per-container env.
- `AgentRuntime.spec.extraPodAnnotations` is preserved; user annotations from `podOverrides.annotations` override on key collision.

## Out of scope

These fields are **not** included in `PodOverrides` by design:

- `resources` — already per-workload where relevant.
- `securityContext` — the operator enforces distroless / non-root; override via CRD-specific resource fields only.
- `hostNetwork`, `hostPID`, `hostIPC`, `runtimeClassName` — security footguns; open an issue if you need them.
