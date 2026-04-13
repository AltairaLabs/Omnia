---
title: "SessionPrivacyPolicy CRD"
description: "Complete reference for the SessionPrivacyPolicy custom resource (Enterprise)"
sidebar:
  order: 12
---

`SessionPrivacyPolicy` is a namespaced enterprise CRD that captures privacy rules for session data: what gets recorded, how long it is kept, whether users can opt out, how data is encrypted at rest, and whether privacy operations are audit-logged. Policies are **reusable documents** — a single policy can be referenced by multiple service groups or agent overrides. The policy itself carries no binding information; binding happens at the consumer side.

## API Version

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionPrivacyPolicy
```

## Resource Scope

`SessionPrivacyPolicy` is **namespace-scoped**. Policies must live in the same namespace as the Workspace or AgentRuntime that references them, except for the global default, which lives in `omnia-system`.

## How Policies Are Bound

Policies are attached to consumers, not embedded in the policy itself.

### Service group binding (Workspace)

Each entry in `Workspace.spec.services[]` has an optional `privacyPolicyRef` field that selects a policy in that workspace's namespace:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Workspace
metadata:
  name: my-workspace
spec:
  services:
    - name: default
      privacyPolicyRef:
        name: my-policy
```

### Agent override (AgentRuntime)

An `AgentRuntime` can override its service group's policy via `spec.privacyPolicyRef`. The policy must exist in the same namespace as the AgentRuntime:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: my-agent
  namespace: my-workspace-ns
spec:
  privacyPolicyRef:
    name: strict-policy
```

## Resolution Order

When the session-api resolves the effective policy for a session, it uses the first matching rule in this chain:

1. `AgentRuntime.spec.privacyPolicyRef` — per-agent override in the agent's own namespace.
2. The service group's `privacyPolicyRef` on the Workspace whose `spec.namespace.name` matches the agent's namespace. The service group is determined by `AgentRuntime.spec.serviceGroup` (defaults to `default`).
3. The global default `SessionPrivacyPolicy` named `default` in the `omnia-system` namespace.
4. No policy applies — all session data is recorded without restriction.

There is no merge semantics. The first matching policy is used in its entirety.

## Namespacing Rules

| Binding location | Policy must live in |
|---|---|
| `Workspace.spec.services[].privacyPolicyRef` | The workspace's own namespace (`spec.namespace.name`) |
| `AgentRuntime.spec.privacyPolicyRef` | The AgentRuntime's namespace |
| Global default | `omnia-system` namespace, named `default` |

## Spec Fields

### `recording` (required)

Controls what session data is recorded.

| Field | Type | Default | Required |
|---|---|---|---|
| `recording.enabled` | bool | — | Yes |
| `recording.facadeData` | bool | false | No |
| `recording.richData` | bool | false | No |
| `recording.pii` | PIIConfig | — | No |

When `recording.enabled` is `false`, all write endpoints on the session-api return 204 No Content and drop the data.

When `recording.richData` is `false`, the middleware blocks assistant messages, tool calls, runtime events, and provider calls. User messages, status updates, and TTL refreshes continue to be accepted.

`recording.facadeData` controls whether facade-layer summary metadata (session open/close timestamps, user IDs, counts) is recorded.

#### `recording.pii`

Configures automatic PII detection and handling.

| Field | Type | Description |
|---|---|---|
| `pii.redact` | bool | Enable PII redaction |
| `pii.encrypt` | bool | Encrypt detected PII instead of (or in addition to) redaction |
| `pii.patterns` | string[] | PII patterns to detect. Built-in: `ssn`, `credit_card`, `phone_number`, `email`, `ip_address`. Custom regex with `custom:` prefix, e.g. `custom:^[A-Z]{2}\d{6}$` |
| `pii.strategy` | string | Redaction method: `replace` (default, e.g. `[REDACTED_SSN]`), `hash` (deterministic SHA-256), `mask` (preserve last 4 chars) |

```yaml
spec:
  recording:
    enabled: true
    richData: true
    pii:
      redact: true
      patterns:
        - email
        - ssn
        - credit_card
      strategy: replace
```

### `retention`

Privacy-specific retention overrides. These are additive constraints on top of any `SessionRetentionPolicy` that governs the workspace.

| Field | Type | Description |
|---|---|---|
| `retention.facade.warmDays` | int32 | Days to keep facade data in warm store |
| `retention.facade.coldDays` | int32 | Days to keep facade data in cold archive |
| `retention.richData.warmDays` | int32 | Days to keep rich session content in warm store |
| `retention.richData.coldDays` | int32 | Days to keep rich session content in cold archive |

### `userOptOut`

Enables end-user control over session recording.

| Field | Type | Description |
|---|---|---|
| `userOptOut.enabled` | bool | Allow users to opt out of recording |
| `userOptOut.honorDeleteRequests` | bool | Process user data deletion requests (GDPR/CCPA) |
| `userOptOut.deleteWithinDays` | int32 | Maximum days to fulfill a deletion request (minimum: 1) |

When `userOptOut.enabled` is true and a user has opted out, all session-api write endpoints return 204 No Content and silently drop the request. The `X-Omnia-User-ID` header propagated by the facade and runtime enables per-user enforcement.

### `encryption`

Configures encryption at rest for session data using envelope encryption (AES-256-GCM with a KMS-managed data key).

| Field | Type | Description |
|---|---|---|
| `encryption.enabled` | bool | Enable encryption |
| `encryption.kmsProvider` | string | KMS provider. One of: `aws-kms`, `azure-keyvault`, `gcp-kms`, `vault` |
| `encryption.keyID` | string | Key identifier within the KMS provider |
| `encryption.secretRef.name` | string | Name of a Secret containing provider credentials |
| `encryption.keyRotation` | KeyRotationConfig | Key rotation settings |

Both `kmsProvider` and `keyID` are required when `enabled` is `true` (enforced by CEL validation).

#### Per-field encryption coverage

Not all fields are encrypted. Fields that are useful for analytics or operational queries remain plaintext:

| Object | Encrypted | Plaintext |
|---|---|---|
| Message | `content`, all `metadata` values | `role`, `type`, `sessionID`, timestamps |
| ToolCall | `arguments`, `result`, `errorMessage` | `name`, `status`, `sessionID`, timestamps |
| RuntimeEvent | `data`, `errorMessage` | `eventType`, `sessionID`, timestamps |
| ProviderCall | (none — entirely plaintext) | all fields |

The `enc:v1:` prefix identifies encrypted string fields (such as `errorMessage`). JSON envelope fields are identified by an `_encryption` metadata key within the stored JSON object.

#### `encryption.keyRotation`

| Field | Type | Description |
|---|---|---|
| `keyRotation.enabled` | bool | Enable automatic rotation |
| `keyRotation.schedule` | string | Cron expression, e.g. `0 0 1 * *` for monthly |
| `keyRotation.reEncryptExisting` | bool | Re-encrypt existing data after rotation |
| `keyRotation.batchSize` | int32 | Messages per re-encryption batch (1–1000, default 100) |

Key rotation updates `encryption.keyID`. New writes immediately use the new key. Existing ciphertext remains readable as long as the old key is still accessible in the KMS.

```yaml
spec:
  encryption:
    enabled: true
    kmsProvider: aws-kms
    keyID: arn:aws:kms:us-east-1:123456789012:key/mrk-abc123
    secretRef:
      name: aws-kms-credentials
    keyRotation:
      enabled: true
      schedule: "0 0 1 * *"
      reEncryptExisting: true
      batchSize: 100
```

### `auditLog`

Logs privacy-related operations (opt-out changes, deletion requests, policy applications) for compliance purposes.

| Field | Type | Description |
|---|---|---|
| `auditLog.enabled` | bool | Enable audit logging |
| `auditLog.retentionDays` | int32 | Days to retain audit entries (minimum: 1) |

## Status Fields

| Field | Type | Description |
|---|---|---|
| `status.phase` | string | `Active` or `Error` |
| `status.observedGeneration` | int64 | Last generation reconciled |
| `status.conditions` | Condition[] | Standard Kubernetes conditions |
| `status.keyRotation` | KeyRotationStatus | Key rotation progress (when configured) |

### Conditions

| Type | Meaning |
|---|---|
| `Ready` | Policy is valid and can be applied |

### `status.keyRotation`

| Field | Type | Description |
|---|---|---|
| `keyRotation.lastRotatedAt` | time | Timestamp of the last successful rotation |
| `keyRotation.currentKeyVersion` | string | Version of the key currently in use |
| `keyRotation.reEncryptionProgress.status` | string | `Pending`, `InProgress`, `Completed`, or `Failed` |
| `keyRotation.reEncryptionProgress.messagesProcessed` | int64 | Messages re-encrypted so far |
| `keyRotation.reEncryptionProgress.startedAt` | time | When re-encryption began |
| `keyRotation.reEncryptionProgress.completedAt` | time | When re-encryption finished |

## Examples

### Minimal policy — recording only

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionPrivacyPolicy
metadata:
  name: record-only
  namespace: my-workspace-ns
spec:
  recording:
    enabled: true
    facadeData: true
    richData: true
```

### Comprehensive policy — PII redaction, encryption, opt-out, audit

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionPrivacyPolicy
metadata:
  name: gdpr-compliant
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
        - phone_number
      strategy: replace
  retention:
    facade:
      warmDays: 90
    richData:
      warmDays: 30
      coldDays: 365
  userOptOut:
    enabled: true
    honorDeleteRequests: true
    deleteWithinDays: 30
  encryption:
    enabled: true
    kmsProvider: aws-kms
    keyID: arn:aws:kms:us-east-1:123456789012:key/mrk-abc123
    secretRef:
      name: aws-kms-credentials
    keyRotation:
      enabled: true
      schedule: "0 0 1 * *"
      reEncryptExisting: false
  auditLog:
    enabled: true
    retentionDays: 365
```

### Global default in omnia-system

```yaml
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
```

## Related Resources

- [Configure Privacy Policies](/how-to/configure-privacy-policies/) — step-by-step setup guide
- [Workspace CRD](/reference/workspace/) — `spec.services[].privacyPolicyRef`
- [AgentRuntime CRD](/reference/agentruntime/) — `spec.privacyPolicyRef`
