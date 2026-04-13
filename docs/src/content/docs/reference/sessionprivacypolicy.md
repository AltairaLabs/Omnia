---
title: "SessionPrivacyPolicy CRD"
description: "Complete reference for the SessionPrivacyPolicy custom resource"
sidebar:
  order: 7
---

:::note[Enterprise]
SessionPrivacyPolicy is an Enterprise feature. See [Licensing](/explanation/licensing/) for details.
:::

The SessionPrivacyPolicy custom resource defines privacy rules for session data. It controls what is recorded, how PII is handled, how long data is retained, user opt-out and GDPR delete behavior, at-rest encryption via KMS, and audit logging.

Policies are cluster-scoped and layered — a `global` policy applies everywhere, a `workspace` policy narrows to a namespace, and an `agent` policy narrows further to a single AgentRuntime. The session-api computes the effective policy for each `(namespace, agent)` tuple and uses it to drive enforcement.

## API Version

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionPrivacyPolicy
```

The resource is cluster-scoped; `metadata.namespace` is not used.

## Scope and Inheritance

The `spec.level` field selects where a policy applies:

| Level | Scope | Required refs |
|-------|-------|---------------|
| `global` | Entire cluster | None (and `workspaceRef`/`agentRef` must be absent) |
| `workspace` | A single Workspace | `workspaceRef` |
| `agent` | A single AgentRuntime | `agentRef` |

When multiple policies match the same session, the session-api merges them **stricter-wins**:

| Field | Merge rule |
|-------|-----------|
| `recording.enabled` | `false` wins — if any level disables recording, it is off |
| `recording.richData` | `false` wins |
| `recording.facadeData` | `false` wins |
| `recording.pii.redact` | `true` wins |
| `recording.pii.encrypt` | `true` wins |
| `userOptOut.enabled` | `true` wins |
| `retention.*Days` | Minimum value wins |
| `encryption.enabled` | `true` wins |
| `encryption.kmsProvider` / `keyID` / `secretRef` | Child overrides parent |

An agent-level policy can therefore **further restrict** a workspace or global policy, but cannot relax it.

## Spec Fields

### `level` (required)

One of `global`, `workspace`, or `agent`. See [Scope and Inheritance](#scope-and-inheritance) above.

### `workspaceRef`

Required when `level: workspace`. References the Workspace this policy applies to.

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Name of the Workspace resource |

### `agentRef`

Required when `level: agent`. References the AgentRuntime this policy applies to.

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Name of the AgentRuntime |
| `namespace` | string | Namespace of the AgentRuntime |

### `recording` (required)

Controls what session data is recorded.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `enabled` | bool | Yes | Master switch. When `false` the facade skips recording entirely and the session-api rejects recording writes with 204. |
| `facadeData` | bool | No | Enables recording of facade-layer summary data (session metadata, summaries). |
| `richData` | bool | No | Enables recording of full session content: assistant messages, tool calls, tool results, provider calls, and runtime events. When `false`, user messages, session status updates, and TTL refreshes are still recorded, but assistant/tool content is skipped. |
| `pii` | PIIConfig | No | PII detection and handling. |

`PIIConfig`:

| Field | Type | Description |
|-------|------|-------------|
| `redact` | bool | Enable automatic PII redaction in recorded data. |
| `encrypt` | bool | Encrypt PII instead of or in addition to redacting it. |
| `patterns` | []string | Which PII patterns to detect. Built-ins: `ssn`, `credit_card`, `phone_number`, `email`, `ip_address`. Custom regex patterns use the `custom:` prefix, e.g. `custom:^[A-Z]{2}\d{6}$`. |
| `strategy` | string | How matched PII is rewritten. One of `replace` (default; swaps for a token like `[REDACTED_SSN]`), `hash` (deterministic truncated SHA-256), or `mask` (preserves the last 4 characters, masking the rest with `*`). |

### `retention`

Privacy-specific retention overrides on top of the tenant's SessionRetentionPolicy. Split into two data tiers.

| Field | Type | Description |
|-------|------|-------------|
| `facade` | PrivacyRetentionTierConfig | Retention for facade-layer data. |
| `richData` | PrivacyRetentionTierConfig | Retention for full session content. |

`PrivacyRetentionTierConfig`:

| Field | Type | Description |
|-------|------|-------------|
| `warmDays` | int32 | Days to keep data in the warm (Postgres) tier. Minimum 0. |
| `coldDays` | int32 | Days to keep data in the cold archive. Minimum 0. |

### `userOptOut`

User opt-out and data-subject deletion behavior.

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | bool | Whether users can opt out of recording. |
| `honorDeleteRequests` | bool | Whether user deletion requests (DSAR) are honored and cascaded. |
| `deleteWithinDays` | int32 | Maximum number of days to fulfill a deletion request. Minimum 1. |

### `encryption`

Configures envelope encryption of session data at rest. When enabled on the effective global policy, the session-api builds a KMS provider at startup and transparently encrypts message content, tool-call arguments/results, runtime event payloads, and error messages. Plaintext columns used for analytics (message role, tool name/status/duration, event type, token counts, costs) remain in the clear so dashboards work without decryption. Legacy plaintext data written before encryption was enabled is read back unchanged — migration is gradual.

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | bool | Master switch. |
| `kmsProvider` | string | One of `aws-kms`, `azure-keyvault`, `gcp-kms`, `vault`. Required when `enabled: true`. |
| `keyID` | string | KMS key identifier. ARN for AWS, key URI for Azure/GCP, key name for Vault Transit. Required when `enabled: true`. |
| `secretRef` | LocalObjectReference | Reference to a Secret in the `omnia-system` namespace holding provider credentials. See [Secret structure by provider](#secret-structure-by-kms-provider). |
| `keyRotation` | KeyRotationConfig | Optional automatic key rotation. |

`KeyRotationConfig`:

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | bool | Whether scheduled rotation runs. |
| `schedule` | string | Cron expression (e.g. `0 0 1 * *` for monthly). |
| `reEncryptExisting` | bool | When `true`, after rotation a background job walks existing rows and re-encrypts them with the new key version. Requires the arena-controller to be started with `--session-postgres-conn`; otherwise re-encryption is silently skipped and old rows stay on the old key version. |
| `batchSize` | int32 | Rows re-encrypted per batch. Default 100, maximum 1000. |

### `auditLog`

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | bool | Emit audit events for privacy-related operations (opt-outs, deletions, key rotations). |
| `retentionDays` | int32 | Days to keep audit log entries. Minimum 1. |

## Secret Structure by KMS Provider

The Secret referenced by `encryption.secretRef` lives in the `omnia-system` namespace and uses the following keys depending on provider. Any key may be omitted if the cluster provides the equivalent via workload identity or the environment.

### `aws-kms`

| Key | Description |
|-----|-------------|
| `region` | AWS region (required). |
| `access-key-id` | Static access key (optional — omit to use IRSA or the instance role). |
| `secret-access-key` | Static secret key (optional). |

### `azure-keyvault`

| Key | Description |
|-----|-------------|
| `tenant-id` | Azure AD tenant ID. |
| `client-id` | Service principal client ID. |
| `client-secret` | Service principal client secret. |

### `gcp-kms`

| Key | Description |
|-----|-------------|
| `credentials-json` | Full service account JSON (optional — omit to use Workload Identity). |

### `vault`

| Key | Description |
|-----|-------------|
| `vault-url` | Address of the Vault server, e.g. `https://vault.example.com`. |
| `token` | Vault token with access to the Transit mount. |
| `mount-path` | Transit mount path (optional; defaults to `transit`). |

## Status Fields

### `phase`

| Value | Description |
|-------|-------------|
| `Active` | Policy is valid and in effect. |
| `Error` | Policy has a configuration error. See conditions for details. |

### `conditions`

Standard Kubernetes conditions. Common types:

| Type | Meaning |
|------|---------|
| `Ready` | The policy is valid and the controller has published it. |
| `ParentFound` | For `workspace`/`agent` policies: the referenced Workspace or AgentRuntime exists. |
| `EffectivePolicyStored` | The merged effective policy has been stored for the session-api to consume. |

### `keyRotation`

Reports on key rotation activity.

| Field | Type | Description |
|-------|------|-------------|
| `lastRotatedAt` | Time | Timestamp of the last successful rotation. |
| `currentKeyVersion` | string | Version currently used for new encryptions. |
| `reEncryptionProgress.status` | string | One of `Pending`, `InProgress`, `Completed`, `Failed`. |
| `reEncryptionProgress.messagesProcessed` | int64 | Rows re-encrypted so far. |
| `reEncryptionProgress.startedAt` / `completedAt` | Time | Re-encryption run timestamps. |

### `observedGeneration`

The most recent `.metadata.generation` processed by the controller.

## Print Columns

When using `kubectl get sessionprivacypolicies`, the following columns are displayed:

| Column | Source |
|--------|--------|
| Level | `.spec.level` |
| Recording | `.spec.recording.enabled` |
| PII Redact | `.spec.recording.pii.redact` |
| Encryption | `.spec.encryption.enabled` |
| Phase | `.status.phase` |
| Age | `.metadata.creationTimestamp` |

## Examples

### Privacy-conservative global default

Record only facade-layer metadata — skip assistant messages, tool calls, and tool results. User prompts and session lifecycle events are still captured.

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionPrivacyPolicy
metadata:
  name: global-default
spec:
  level: global
  recording:
    enabled: true
    facadeData: true
    richData: false
    pii:
      redact: true
      patterns: [ssn, credit_card, email]
      strategy: replace
  userOptOut:
    enabled: true
    honorDeleteRequests: true
    deleteWithinDays: 30
  auditLog:
    enabled: true
    retentionDays: 365
```

### Workspace policy with AWS KMS encryption

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionPrivacyPolicy
metadata:
  name: finance-encryption
spec:
  level: workspace
  workspaceRef:
    name: finance
  recording:
    enabled: true
    facadeData: true
    richData: true
  encryption:
    enabled: true
    kmsProvider: aws-kms
    keyID: arn:aws:kms:us-east-1:111122223333:key/abcd1234-ab12-cd34-ef56-abcdef123456
    secretRef:
      name: omnia-encryption-aws
    keyRotation:
      enabled: true
      schedule: "0 0 1 * *"
      reEncryptExisting: true
      batchSize: 250
```

### Agent-level strict override

An agent that must never persist rich content, regardless of what the parent workspace or global policy allows. Stricter-wins merging ensures this takes effect.

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionPrivacyPolicy
metadata:
  name: pii-agent-strict
spec:
  level: agent
  agentRef:
    name: pii-reviewer
    namespace: compliance
  recording:
    enabled: true
    facadeData: true
    richData: false
    pii:
      redact: true
      encrypt: true
      patterns: [ssn, credit_card, phone_number, email, ip_address]
      strategy: hash
  retention:
    facade:
      warmDays: 7
      coldDays: 30
    richData:
      warmDays: 0
      coldDays: 0
```

## Related Resources

- [Configure Privacy Policies](/how-to/configure-privacy-policies/) — operational guide
- [Workspace CRD Reference](/reference/workspace/)
- [AgentRuntime CRD Reference](/reference/agentruntime/)
