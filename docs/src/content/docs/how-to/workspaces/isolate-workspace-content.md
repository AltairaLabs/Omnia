---
title: Isolate workspace content
description: How Omnia isolates per-workspace content on shared NFS, and how to enable the no-root admission-policy backstop (with Gatekeeper / Kyverno equivalents).
---

Omnia stores each workspace's content (arena projects, skill sources, prompt
pack skills) under a shared content root, laid out as
`<contentRoot>/<workspace>/<namespace>/…`. Isolation is defense-in-depth and
**uses no root anywhere** — every component runs `runAsNonRoot: true`.

## Layer 1 — per-namespace subtree volumes

Each workspace namespace mounts a native-NFS PersistentVolume scoped to its own
`…/<workspace>/<namespace>` subtree, surfaced as the PVC
`workspace-<namespace>-content`. A workload only ever sees its own subtree.

## The dashboard reads/writes via the operator, not NFS

The dashboard does **not** mount the content volume. It calls an authenticated
**operator content API** instead (`OPERATOR_CONTENT_API_URL`). Per request the
dashboard mints a short-lived RS256 identity token (carrying the user's
identity + groups, audience `omnia-operator`) signed with its mgmt-plane key;
the operator verifies the signature via the dashboard JWKS endpoint and
**recomputes the workspace role from the Workspace CR** — it never trusts a
role claim. The operator writes as its own uniform UID (`65532`), so there is
no cross-UID writer and no `chown`. This is what removes the `EACCES` the
dashboard used to hit on operator-created content.

The operator confines every path within the target workspace's subtree
(`filepath.Clean` + prefix check + `O_NOFOLLOW`), rejecting `..`, absolute, and
symlink escapes.

## Layer 2 — admission-policy backstop

A `ValidatingAdmissionPolicy` denies any pod in a workspace namespace from
mounting a workspace-content PVC other than its own
`workspace-<namespace>-content` — catching both a foreign workspace's PVC and
the shared-root PVC. The control-plane namespace (the release namespace) is
exempt, since the operator legitimately mounts the share root to serve content.

It requires **Kubernetes ≥ 1.30** with the `ValidatingAdmissionPolicy` feature
and is **off by default**. Enable it:

```yaml
workspaceContent:
  admissionPolicy:
    enabled: true
    failurePolicy: Fail
    # Optional: also deny PVs that target the share root, except the provisioner.
    shareRootPath: ""          # e.g. /data/omnia (install-specific)
    provisionerUsername: ""    # e.g. system:serviceaccount:kube-system:nfs-provisioner
```

The rendered policy's core check (CEL):

```cel
object.spec.volumes.all(v,
  !has(v.persistentVolumeClaim) ||
  !(v.persistentVolumeClaim.claimName.contains('workspace') &&
    v.persistentVolumeClaim.claimName.endsWith('-content')) ||
  v.persistentVolumeClaim.claimName == 'workspace-' + request.namespace + '-content'
)
```

### Clusters without ValidatingAdmissionPolicy

On Kubernetes < 1.30 or where the feature is disabled, enforce the same rule
with a policy controller.

**Gatekeeper** — a `ConstraintTemplate` with Rego over `input.review.object`:

```rego
package workspacecontent
violation[{"msg": msg}] {
  vol := input.review.object.spec.volumes[_]
  claim := vol.persistentVolumeClaim.claimName
  contains(claim, "workspace")
  endswith(claim, "-content")
  claim != sprintf("workspace-%s-content", [input.review.namespace])
  msg := sprintf("pod may only mount workspace-%s-content", [input.review.namespace])
}
```

**Kyverno** — a `ClusterPolicy` `validate.deny` with the equivalent JMESPath /
CEL condition over `request.object.spec.volumes[].persistentVolumeClaim.claimName`,
excluding the control-plane namespace.

Scope either to exclude the Omnia release namespace so the operator can keep
mounting the share root.
