---
title: "Configure Privacy Policies"
description: "Control session recording, PII handling, and at-rest encryption with SessionPrivacyPolicy"
sidebar:
  order: 23
---

:::note[Enterprise]
SessionPrivacyPolicy is an Enterprise feature. See [Licensing](/explanation/licensing/) for details.
:::

This guide walks through the common tasks for configuring SessionPrivacyPolicy resources: restricting recording, enabling at-rest encryption with each supported KMS, rotating keys, and layering policies. For the full field reference, see the [SessionPrivacyPolicy CRD Reference](/reference/sessionprivacypolicy/).

## Prerequisites

- Omnia Enterprise license activated
- Operator and session-api installed and running
- `kubectl` access with permissions to create `SessionPrivacyPolicy` (cluster-scoped) and Secrets in `omnia-system`
- For encryption: a KMS key already provisioned in your cloud account

## Disable Session Recording Globally

Apply a global policy with `recording.enabled: false` — the facade skips recording entirely and the session-api returns `204 No Content` for recording writes.

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionPrivacyPolicy
metadata:
  name: recording-off
spec:
  level: global
  recording:
    enabled: false
```

```bash
kubectl apply -f recording-off.yaml
```

Verify:

```bash
kubectl get sessionprivacypolicy recording-off
```

```
NAME            LEVEL    RECORDING   PII REDACT   ENCRYPTION   PHASE    AGE
recording-off   global   false                                 Active   10s
```

## Restrict Rich Content Recording

To keep basic session metadata and user prompts but avoid persisting assistant messages, tool calls, and tool results, set `recording.richData: false`:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionPrivacyPolicy
metadata:
  name: conservative-recording
spec:
  level: global
  recording:
    enabled: true
    facadeData: true
    richData: false
```

:::tip
With `richData: false`, the facade still records user messages, session status updates, and TTL refreshes. Only assistant content, tool calls, tool results, provider calls, and runtime events are skipped. The session-api middleware rejects any write the facade should have skipped as a safety net.
:::

## Enable Encryption at Rest with AWS KMS

Encryption is driven by the effective **global** policy. The session-api loads the referenced Secret at startup, builds a KMS client, and transparently envelope-encrypts message content, tool-call arguments/results, runtime event payloads, and error messages. Analytics-friendly columns (role, status, durations, token counts, costs) remain plaintext.

### 1. Create the credentials Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: omnia-encryption-aws
  namespace: omnia-system
type: Opaque
stringData:
  region: us-east-1
  access-key-id: AKIA...
  secret-access-key: wJalrXUtn...
```

:::tip
If the cluster uses IAM Roles for Service Accounts (IRSA) or the instance role has KMS permissions, omit `access-key-id` / `secret-access-key` and only set `region`.
:::

### 2. Create the SessionPrivacyPolicy

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionPrivacyPolicy
metadata:
  name: encryption-global
spec:
  level: global
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
```

```bash
kubectl apply -f encryption-global.yaml
```

### 3. Verify

```bash
kubectl get --raw /api/v1/namespaces/omnia-system/services/omnia-session-api:http/proxy/api/v1/encryption-status
```

Expected response:

```json
{"enabled":true}
```

You can also check the session-api logs for the encryption provider being wired up at startup.

:::caution
Data written before encryption was enabled remains readable — the reader detects plaintext rows and passes them through unchanged. Migration is gradual; use key rotation with `reEncryptExisting: true` (below) if you need every row converted.
:::

## Configure Other KMS Providers

### Azure Key Vault

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: omnia-encryption-azure
  namespace: omnia-system
type: Opaque
stringData:
  tenant-id: 00000000-0000-0000-0000-000000000000
  client-id: 11111111-1111-1111-1111-111111111111
  client-secret: ...
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionPrivacyPolicy
metadata:
  name: encryption-global
spec:
  level: global
  recording:
    enabled: true
  encryption:
    enabled: true
    kmsProvider: azure-keyvault
    keyID: https://my-vault.vault.azure.net/keys/omnia-session/abcdef
    secretRef:
      name: omnia-encryption-azure
```

### Google Cloud KMS

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: omnia-encryption-gcp
  namespace: omnia-system
type: Opaque
stringData:
  credentials-json: |
    {
      "type": "service_account",
      "project_id": "...",
      "private_key_id": "...",
      "private_key": "...",
      "client_email": "...",
      "client_id": "..."
    }
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionPrivacyPolicy
metadata:
  name: encryption-global
spec:
  level: global
  recording:
    enabled: true
  encryption:
    enabled: true
    kmsProvider: gcp-kms
    keyID: projects/my-project/locations/us/keyRings/omnia/cryptoKeys/session
    secretRef:
      name: omnia-encryption-gcp
```

:::tip
If the cluster uses GKE Workload Identity, omit `credentials-json` — the provider will fall back to Application Default Credentials.
:::

### HashiCorp Vault Transit

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: omnia-encryption-vault
  namespace: omnia-system
type: Opaque
stringData:
  vault-url: https://vault.example.com
  token: hvs.CAES...
  mount-path: transit   # optional; defaults to "transit"
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionPrivacyPolicy
metadata:
  name: encryption-global
spec:
  level: global
  recording:
    enabled: true
  encryption:
    enabled: true
    kmsProvider: vault
    keyID: omnia-session
    secretRef:
      name: omnia-encryption-vault
```

## Enable Automated Key Rotation

Add `keyRotation` to the encryption block:

```yaml
spec:
  encryption:
    enabled: true
    kmsProvider: aws-kms
    keyID: arn:aws:kms:us-east-1:111122223333:key/abcd1234-...
    secretRef:
      name: omnia-encryption-aws
    keyRotation:
      enabled: true
      schedule: "0 0 1 * *"     # monthly
      reEncryptExisting: true
      batchSize: 250
```

The `KeyRotationReconciler` in the arena-controller picks up the schedule and rotates on its cadence.

:::caution
When `reEncryptExisting: true`, the arena-controller walks the `messages` table cursor-based and re-encrypts rows with the new key version. This requires the arena-controller to be started with `--session-postgres-conn=<session-postgres-url>`. Without the flag, key rotation still rotates the active key — but existing rows stay on the old version (and the job is silently skipped).
:::

Progress is reflected in `.status.keyRotation`:

```bash
kubectl get sessionprivacypolicy encryption-global -o jsonpath='{.status.keyRotation}'
```

## Trigger a Manual Key Rotation

Add the `omnia.altairalabs.ai/rotate-key` annotation to force an out-of-band rotation:

```bash
kubectl annotate sessionprivacypolicy encryption-global \
  omnia.altairalabs.ai/rotate-key="$(date +%s)" --overwrite
```

The controller removes the annotation after processing. `.status.keyRotation.lastRotatedAt` updates on success.

## Inheritance: Layered Policies

Policies merge **stricter-wins** across levels. A common pattern is a permissive global policy with agent-level overrides for sensitive workloads.

```yaml
# Global: recording on, rich data on
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionPrivacyPolicy
metadata:
  name: global-default
spec:
  level: global
  recording:
    enabled: true
    facadeData: true
    richData: true
---
# Workspace: require PII redaction in finance namespace
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionPrivacyPolicy
metadata:
  name: finance-pii
spec:
  level: workspace
  workspaceRef:
    name: finance
  recording:
    enabled: true
    pii:
      redact: true
      patterns: [credit_card, ssn]
      strategy: mask
---
# Agent: disable rich recording for one PII-sensitive agent
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionPrivacyPolicy
metadata:
  name: pii-reviewer-strict
spec:
  level: agent
  agentRef:
    name: pii-reviewer
    namespace: finance
  recording:
    enabled: true
    richData: false
```

For a session running the `pii-reviewer` agent in the `finance` workspace, the effective policy is: recording enabled, PII redaction on (from workspace), rich data off (from agent).

## Verify

List all policies and their effective settings:

```bash
kubectl get sessionprivacypolicies
```

```
NAME                  LEVEL       RECORDING   PII REDACT   ENCRYPTION   PHASE    AGE
global-default        global      true                                  Active   5m
finance-pii           workspace   true        true                      Active   4m
pii-reviewer-strict   agent       true                                  Active   1m
encryption-global     global      true                     true         Active   10m
```

Inspect the resolved policy seen by the facade for a given `(namespace, agent)`:

```bash
kubectl get --raw "/api/v1/namespaces/omnia-system/services/omnia-session-api:http/proxy/api/v1/privacy-policy?namespace=finance&agent=pii-reviewer"
```

Response shape:

```json
{"recording":{"enabled":true,"facadeData":true,"richData":false}}
```

A `204 No Content` means no policy applies and default behavior (record everything) is used.

Check encryption wiring:

```bash
kubectl get --raw /api/v1/namespaces/omnia-system/services/omnia-session-api:http/proxy/api/v1/encryption-status
```

## Troubleshooting

**Encryption not active**: `GET /api/v1/encryption-status` returns `{"enabled":false}` even though the policy is `Active`.

- The session-api only picks up encryption config at startup. Roll the session-api pods after creating or updating the encryption policy.
- Check session-api logs for `encryption provider built` at startup, or for errors loading the Secret.
- Confirm the Secret lives in `omnia-system` (that is the namespace the session-api reads from).
- Verify the Secret keys match the [provider-specific keys](/reference/sessionprivacypolicy/#secret-structure-by-kms-provider) — typos fail silently in some providers.

**Re-encryption is skipped after rotation**: `.status.keyRotation.reEncryptionProgress` never populates.

- The arena-controller must be started with `--session-postgres-conn=<url>`. Without that flag, rotation happens but re-encryption is a no-op.
- Check arena-controller logs for `re-encryption skipped` with `reason: no session postgres connection`.

**Facade still recording after disabling**: Users report the UI still shows messages being stored.

- The facade caches the privacy policy response for 60 seconds. Wait a minute, or restart the facade pods.
- Confirm `GET /api/v1/privacy-policy?namespace=X&agent=Y` returns the expected JSON — a `204` means no policy matches, and the facade defaults to recording enabled.
- Check that your policy's `level` and refs actually select the agent. An agent-level policy with the wrong `agentRef.namespace` will simply not apply.

**Policy stuck in `Error` phase**:

```bash
kubectl describe sessionprivacypolicy <name>
```

Look at `conditions` — `ParentFound: False` means the referenced Workspace or AgentRuntime does not exist; `Ready: False` with a reason explains validation failures (for example, `encryption.enabled: true` without `keyID` or `kmsProvider`).

## Related Resources

- [SessionPrivacyPolicy CRD Reference](/reference/sessionprivacypolicy/) — full field specification
- [Workspace CRD Reference](/reference/workspace/)
- [Configure Sessions](/how-to/configure-sessions/) — retention and storage configuration
