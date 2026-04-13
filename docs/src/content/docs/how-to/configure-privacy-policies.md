---
title: "Configure Privacy Policies"
description: "Create and attach SessionPrivacyPolicy resources to control recording, PII handling, encryption, and user opt-out for session data"
sidebar:
  order: 15
---

This guide walks through creating a `SessionPrivacyPolicy`, attaching it to a workspace service group, overriding it for a specific agent, and verifying the configuration is active.

`SessionPrivacyPolicy` is an Enterprise feature. You need an active Enterprise license to use it.

## Prerequisites

- Omnia operator v0.x or later with Enterprise enabled
- A Workspace with at least one service group
- `kubectl` access to the cluster

## Step 1: Create a policy in your workspace namespace

A `SessionPrivacyPolicy` must live in the same namespace that the workspace manages. Find the namespace by checking `Workspace.spec.namespace.name`.

```yaml
kubectl apply -f - <<'EOF'
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionPrivacyPolicy
metadata:
  name: default-privacy
  namespace: my-workspace-ns
spec:
  recording:
    enabled: true
    facadeData: true
    richData: true
    pii:
      redact: true
      patterns:
        - email
        - ssn
        - credit_card
      strategy: replace
  userOptOut:
    enabled: true
    honorDeleteRequests: true
    deleteWithinDays: 30
  auditLog:
    enabled: true
    retentionDays: 365
EOF
```

Confirm the policy is active:

```bash
kubectl get sessionprivacypolicy default-privacy -n my-workspace-ns
```

The `PHASE` column should show `Active`.

## Step 2: Attach the policy to a service group

Edit your Workspace to add `privacyPolicyRef` to the service group. Most workspaces have a single service group named `default`.

```yaml
kubectl apply -f - <<'EOF'
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Workspace
metadata:
  name: my-workspace
spec:
  displayName: My Workspace
  namespace:
    name: my-workspace-ns
  services:
    - name: default
      privacyPolicyRef:
        name: default-privacy
EOF
```

The operator reconciles the Workspace and sets a `PrivacyPolicyResolved` condition.

Verify:

```bash
kubectl get workspace my-workspace \
  -o jsonpath='{.status.conditions[?(@.type=="PrivacyPolicyResolved")]}'
```

Expected output (formatted for readability):

```json
{
  "type": "PrivacyPolicyResolved",
  "status": "True",
  "reason": "PolicyResolved",
  "message": "..."
}
```

## Step 3: Override the policy on a specific AgentRuntime (optional)

If one agent needs different privacy rules from the rest of the service group, set `spec.privacyPolicyRef` on that AgentRuntime. Create the stricter policy first:

```yaml
kubectl apply -f - <<'EOF'
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionPrivacyPolicy
metadata:
  name: strict-privacy
  namespace: my-workspace-ns
spec:
  recording:
    enabled: true
    facadeData: true
    richData: false
  userOptOut:
    enabled: true
    honorDeleteRequests: true
    deleteWithinDays: 7
  auditLog:
    enabled: true
    retentionDays: 365
EOF
```

Then patch the agent:

```yaml
kubectl apply -f - <<'EOF'
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: sensitive-agent
  namespace: my-workspace-ns
spec:
  privacyPolicyRef:
    name: strict-privacy
EOF
```

Verify the condition on the AgentRuntime:

```bash
kubectl get agentruntime sensitive-agent -n my-workspace-ns \
  -o jsonpath='{.status.conditions[?(@.type=="PrivacyPolicyResolved")]}'
```

## Step 4: Create a global default (optional)

A policy named `default` in the `omnia-system` namespace applies to any agent that has no service group policy and no agent override. This is useful as a baseline for all workspaces.

```yaml
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

## Troubleshooting

### `PrivacyPolicyResolved` is False with reason `PolicyNotFound`

The policy referenced by `privacyPolicyRef.name` does not exist in the expected namespace. Check:

1. The policy name is spelled correctly.
2. The policy exists in the workspace's namespace (not in `omnia-system` or another namespace):
   ```bash
   kubectl get sessionprivacypolicy -n my-workspace-ns
   ```
3. The policy was applied successfully (no admission webhook rejections).

### Seeing which policy applies to a live session

The session-api exposes the effective policy for any namespace/agent combination:

```bash
kubectl exec -n omnia-system deploy/session-api -- \
  curl -s "http://localhost:8080/api/v1/privacy-policy?namespace=my-workspace-ns&agent=my-agent"
```

A 200 response contains the effective recording config. A 204 means no policy applies and recording runs with defaults.

### Agent is not using the override

The `PolicyWatcher` in the session-api polls the Kubernetes API every 30 seconds. After applying a change to an AgentRuntime or policy, wait up to 30 seconds for the new policy to take effect on new sessions. In-flight sessions use the policy that was cached when they started.

## Rotating encryption keys

When `spec.encryption` is configured, key rotation works as follows:

1. Update `encryption.keyID` in the policy to point to the new key in your KMS.
2. Optionally set `encryption.keyRotation.reEncryptExisting: true` to re-encrypt stored data.
3. The `PolicyWatcher` fires an invalidation callback on the next poll (within 30 seconds). New session writes immediately use the new key.
4. Existing ciphertext written with the old key remains readable as long as the old key is still accessible in the KMS. Do not revoke or delete the old key until all existing data has been re-encrypted or its retention period has expired.

If `keyRotation.reEncryptExisting` is enabled, track progress via:

```bash
kubectl get sessionprivacypolicy my-policy -n my-workspace-ns \
  -o jsonpath='{.status.keyRotation}'
```

The `reEncryptionProgress.status` field transitions through `Pending` → `InProgress` → `Completed`.

## Reference

See [SessionPrivacyPolicy CRD](/reference/sessionprivacypolicy/) for the full field reference, encryption coverage per object type, and example YAML.
