# Migration: SessionPrivacyPolicy Redesign (#801)

**Date:** 2026-04-13
**Issue:** https://github.com/AltairaLabs/Omnia/issues/801
**Audience:** Pre-release deployments only (dev / staging). This feature was never shipped to production, so this playbook is documentation rather than a production runbook.

## What Changed

`SessionPrivacyPolicy` was cluster-scoped and embedded its binding target in `spec.level` / `spec.workspaceRef` / `spec.agentRef`. The redesign:

- Makes `SessionPrivacyPolicy` **namespace-scoped** (policies now live in the same namespace as the resources that use them, or in `omnia-system` for the global default).
- Removes `spec.level`, `spec.workspaceRef`, and `spec.agentRef` from the CRD spec entirely.
- Moves binding to the consumer side: `Workspace.spec.services[].privacyPolicyRef` and `AgentRuntime.spec.privacyPolicyRef`.

## Before You Begin

List all existing `SessionPrivacyPolicy` resources so you know what needs to be migrated:

```bash
kubectl get sessionprivacypolicies -A \
  -o jsonpath='{range .items[*]}{.metadata.namespace}{"\t"}{.metadata.name}{"\t"}{.spec.level}{"\t"}{.spec.workspaceRef.name}{"\t"}{.spec.agentRef.name}{"\n"}{end}'
```

Take note of:
- The policy name and namespace
- `spec.level` (`workspace` or `agent`)
- `spec.workspaceRef.name` (for workspace-level policies)
- `spec.agentRef.name` (for agent-level policies)

## Step 1: Upgrade the Helm chart

```bash
helm upgrade omnia charts/omnia --reuse-values
```

After upgrade, the old CRD schema is replaced. Existing objects remain in etcd with their old spec, but the API server will reject any attempt to UPDATE them with the old fields. New objects must use the new schema.

## Step 2: Migrate `level: workspace` policies

For each policy that targeted a workspace:

1. Identify the workspace it targeted (`spec.workspaceRef.name`).
2. Identify the workspace's namespace (`kubectl get workspace <name> -o jsonpath='{.spec.namespace.name}'`).
3. Re-apply the policy in that namespace (drop `level`, `workspaceRef`, `agentRef`):

```bash
kubectl apply -f - <<'EOF'
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionPrivacyPolicy
metadata:
  name: <policy-name>
  namespace: <workspace-namespace>
spec:
  recording:
    enabled: true
    # ... copy remaining fields from old policy spec ...
EOF
```

4. Attach the policy to the workspace service group:

```bash
kubectl patch workspace <workspace-name> --type=merge \
  --patch '{"spec":{"services":[{"name":"default","privacyPolicyRef":{"name":"<policy-name>"}}]}}'
```

If the workspace has multiple service groups, add a `privacyPolicyRef` entry for each relevant group.

Verify the condition:

```bash
kubectl get workspace <workspace-name> \
  -o jsonpath='{.status.conditions[?(@.type=="PrivacyPolicyResolved")]}'
```

Expected: `"status":"True","reason":"PolicyResolved"`.

## Step 3: Migrate `level: agent` policies

For each policy that targeted a specific AgentRuntime:

1. Identify the agent it targeted (`spec.agentRef.name`) and its namespace.
2. Re-apply the policy in the agent's namespace:

```bash
kubectl apply -f - <<'EOF'
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionPrivacyPolicy
metadata:
  name: <policy-name>
  namespace: <agent-namespace>
spec:
  recording:
    enabled: true
    # ... copy remaining fields from old policy spec ...
EOF
```

3. Attach the policy to the AgentRuntime:

```bash
kubectl patch agentruntime <agent-name> -n <agent-namespace> --type=merge \
  --patch '{"spec":{"privacyPolicyRef":{"name":"<policy-name>"}}}'
```

Verify:

```bash
kubectl get agentruntime <agent-name> -n <agent-namespace> \
  -o jsonpath='{.status.conditions[?(@.type=="PrivacyPolicyResolved")]}'
```

Expected: `"status":"True","reason":"PolicyResolved"`.

## Step 4: Create a global default (optional)

If you want all workspaces that have no explicit policy to fall back to a baseline, create a policy named `default` in `omnia-system`:

```bash
kubectl apply -f - <<'EOF'
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionPrivacyPolicy
metadata:
  name: default
  namespace: omnia-system
spec:
  recording:
    enabled: true
    facadeData: true
    richData: false
  auditLog:
    enabled: true
    retentionDays: 90
EOF
```

## Step 5: Verify all workspaces and agents

```bash
# Check all workspaces
kubectl get workspaces -o jsonpath=\
'{range .items[*]}{.metadata.name}{"\t"}{range .status.conditions[?(@.type=="PrivacyPolicyResolved")]}{.status}{"\t"}{.reason}{"\n"}{end}{end}'

# Check all agentruntimes across all namespaces
kubectl get agentruntimes -A -o jsonpath=\
'{range .items[*]}{.metadata.namespace}{"\t"}{.metadata.name}{"\t"}{range .status.conditions[?(@.type=="PrivacyPolicyResolved")]}{.status}{"\t"}{.reason}{"\n"}{end}{end}'
```

Any entry with `status=False` and `reason=PolicyNotFound` indicates a `privacyPolicyRef` that points to a missing policy. Fix by creating the policy in the correct namespace or correcting the name.

## Step 6: Remove old cluster-scoped policy objects (if any remain)

If the old cluster-scoped CRD objects were not automatically removed during chart upgrade, delete them manually:

```bash
kubectl delete sessionprivacypolicies --all -A
```

Note: after the CRD scope change, a cluster-scoped resource with the same group/kind is a different CRD. Confirm you are not deleting the new namespace-scoped objects by checking the CRD definition first:

```bash
kubectl get crd sessionprivacypolicies.omnia.altairalabs.ai \
  -o jsonpath='{.spec.scope}'
```

This should return `Namespaced`. If it still returns `Cluster`, the chart upgrade did not apply — check Helm release status and re-upgrade.
