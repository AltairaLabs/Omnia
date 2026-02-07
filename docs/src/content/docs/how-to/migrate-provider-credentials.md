---
title: "Migrate Provider Credentials"
description: "Migrate from legacy secretRef to the unified credential configuration"
sidebar:
  order: 19
---

This guide walks you through migrating Provider resources from the legacy top-level `secretRef` field to the newer `credential` configuration. The `credential` field provides a unified API with support for Kubernetes Secrets, environment variables, and file-based credentials.

## Why Migrate

- **Unified API** — The `credential` field supports multiple credential strategies (`secretRef`, `envVar`, `filePath`) under a single field, replacing the top-level `secretRef` which only supports Kubernetes Secrets.
- **Future-proof** — The top-level `secretRef` is deprecated and may be removed in a future release.
- **Consistency** — New features like hyperscaler authentication (`platform` + `auth`) are built around the modern credential model.

Both `secretRef` and `credential.secretRef` continue to work, so migration can be done at your own pace.

## Before You Begin

List all Providers in your cluster:

```bash
kubectl get providers -A
```

## Step 1: Identify Providers Using Legacy secretRef

Find Providers that use the top-level `secretRef`:

```bash
kubectl get providers -A -o json | \
  jq -r '.items[] | select(.spec.secretRef != null) | "\(.metadata.namespace)/\(.metadata.name)"'
```

## Step 2: Update the Provider Spec

Move `secretRef` under the `credential` field. The structure is identical — only the nesting changes.

**Before:**

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: claude-production
  namespace: agents
spec:
  type: claude
  model: claude-sonnet-4-20250514
  secretRef:
    name: anthropic-credentials
  defaults:
    temperature: "0.7"
```

**After:**

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: claude-production
  namespace: agents
spec:
  type: claude
  model: claude-sonnet-4-20250514
  credential:
    secretRef:
      name: anthropic-credentials
  defaults:
    temperature: "0.7"
```

The only change is replacing the top-level `secretRef` with `credential.secretRef`. The Secret itself does not need to change.

> **Important**: Do not set both `secretRef` and `credential` on the same Provider. CEL validation will reject the resource.

## Step 3: Apply and Verify

Apply the updated Provider:

```bash
kubectl apply -f provider.yaml
```

Verify the Provider is healthy:

```bash
kubectl get provider claude-production -n agents -o wide
```

Check conditions:

```bash
kubectl get provider claude-production -n agents \
  -o jsonpath='{.status.conditions}' | jq .
```

The `CredentialConfigured` condition should be `True` with reason `SecretRef`.

## Batch Migration

For clusters with many Providers, you can use a script to update them all:

```bash
#!/bin/bash
# migrate-providers.sh — Migrate all Providers from secretRef to credential.secretRef

for provider in $(kubectl get providers -A -o json | \
  jq -r '.items[] | select(.spec.secretRef != null) | "\(.metadata.namespace)/\(.metadata.name)"'); do

  NAMESPACE=$(echo "$provider" | cut -d/ -f1)
  NAME=$(echo "$provider" | cut -d/ -f2)

  echo "Migrating $NAMESPACE/$NAME..."

  # Get current secretRef values
  SECRET_NAME=$(kubectl get provider "$NAME" -n "$NAMESPACE" -o jsonpath='{.spec.secretRef.name}')
  SECRET_KEY=$(kubectl get provider "$NAME" -n "$NAMESPACE" -o jsonpath='{.spec.secretRef.key}')

  # Build the patch
  if [ -n "$SECRET_KEY" ]; then
    PATCH="{\"spec\":{\"secretRef\":null,\"credential\":{\"secretRef\":{\"name\":\"$SECRET_NAME\",\"key\":\"$SECRET_KEY\"}}}}"
  else
    PATCH="{\"spec\":{\"secretRef\":null,\"credential\":{\"secretRef\":{\"name\":\"$SECRET_NAME\"}}}}"
  fi

  kubectl patch provider "$NAME" -n "$NAMESPACE" --type=merge -p "$PATCH"
done

echo "Migration complete. Verifying..."
kubectl get providers -A -o wide
```

## Rollback

If you need to revert, restore the original `secretRef` field and remove `credential`:

```yaml
spec:
  secretRef:
    name: anthropic-credentials
  # Remove the credential field entirely
```

Both formats are supported, so rollback is safe. The only constraint is that you cannot have both `secretRef` and `credential` set at the same time.

## Related Resources

- [Provider CRD Reference](/reference/provider/)
- [Configure AWS Bedrock Provider](/how-to/configure-bedrock-provider/)
- [Configure GCP Vertex AI Provider](/how-to/configure-vertex-provider/)
- [Configure Azure AI Provider](/how-to/configure-azure-ai-provider/)
