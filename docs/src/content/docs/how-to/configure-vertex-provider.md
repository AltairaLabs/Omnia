---
title: "Configure GCP Vertex AI Provider"
description: "Set up Omnia to use Google Vertex AI via GKE Workload Identity or service account key"
sidebar:
  order: 17
---

This guide covers how to configure an Omnia Provider to use Google Vertex AI for LLM access. Vertex AI providers support two authentication methods: **GKE Workload Identity** for production use, and **service account keys** for simpler setups.

## Prerequisites

- A GKE cluster with Workload Identity enabled
- Vertex AI API enabled in your GCP project (`gcloud services enable aiplatform.googleapis.com`)
- `gcloud` CLI installed and authenticated
- Omnia operator installed in the cluster

## Option 1: Workload Identity — Recommended

GKE Workload Identity lets Kubernetes service accounts act as GCP service accounts without exporting keys. This is the recommended approach for production.

### 1. Create a GCP service account

```bash
gcloud iam service-accounts create omnia-vertex \
  --display-name="Omnia Vertex AI" \
  --project=my-gcp-project
```

### 2. Grant the Vertex AI user role

```bash
gcloud projects add-iam-policy-binding my-gcp-project \
  --member="serviceAccount:omnia-vertex@my-gcp-project.iam.gserviceaccount.com" \
  --role="roles/aiplatform.user"
```

### 3. Bind the Kubernetes service account to the GCP service account

```bash
gcloud iam service-accounts add-iam-policy-binding \
  omnia-vertex@my-gcp-project.iam.gserviceaccount.com \
  --role="roles/iam.workloadIdentityUser" \
  --member="serviceAccount:my-gcp-project.svc.id.goog[agents/omnia-agent]"
```

Annotate the Kubernetes service account via Helm values:

```yaml
# values.yaml
serviceAccount:
  annotations:
    iam.gke.io/gcp-service-account: omnia-vertex@my-gcp-project.iam.gserviceaccount.com
```

### 4. Create the Provider

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: vertex-gemini
  namespace: agents
spec:
  type: vertex
  model: gemini-1.5-pro

  platform:
    type: gcp
    region: us-central1
    project: my-gcp-project

  auth:
    type: workloadIdentity
    serviceAccountEmail: omnia-vertex@my-gcp-project.iam.gserviceaccount.com

  capabilities:
    - text
    - streaming
    - vision
    - tools
    - json
```

### 5. Verify

```bash
kubectl get provider vertex-gemini -n agents -o wide
kubectl get provider vertex-gemini -n agents -o jsonpath='{.status.conditions}' | jq .
```

Both the `AuthConfigured` and `Ready` conditions should be `True`.

## Option 2: Service Account Key

For development or environments without GKE Workload Identity, you can use a service account JSON key.

### 1. Create and download a key

```bash
gcloud iam service-accounts keys create key.json \
  --iam-account=omnia-vertex@my-gcp-project.iam.gserviceaccount.com
```

### 2. Create a Secret

```bash
kubectl create secret generic gcp-credentials \
  --namespace agents \
  --from-file=credentials.json=key.json
```

### 3. Create the Provider

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: vertex-gemini
  namespace: agents
spec:
  type: vertex
  model: gemini-1.5-pro

  platform:
    type: gcp
    region: us-central1
    project: my-gcp-project

  auth:
    type: serviceAccount
    credentialsSecretRef:
      name: gcp-credentials

  capabilities:
    - text
    - streaming
    - vision
    - tools
    - json
```

## Using with AgentRuntime

Reference the Provider from an AgentRuntime:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: my-agent
  namespace: agents
spec:
  promptPackRef:
    name: my-prompts
  providerRef:
    name: vertex-gemini
  facade:
    type: websocket
    port: 8080
```

## Troubleshooting

### Vertex AI API not enabled

Ensure the API is enabled in your project:

```bash
gcloud services list --enabled --project=my-gcp-project \
  --filter="config.name:aiplatform.googleapis.com"
```

If missing, enable it:

```bash
gcloud services enable aiplatform.googleapis.com --project=my-gcp-project
```

### Project mismatch

The `platform.project` field must match the GCP project where Vertex AI is enabled. Verify the project ID:

```bash
gcloud config get-value project
```

### IAM binding not propagated

Workload Identity bindings can take a few minutes to propagate. If the Provider shows `AuthConfigured: False`, wait 2-3 minutes and check again. You can also verify the binding:

```bash
gcloud iam service-accounts get-iam-policy \
  omnia-vertex@my-gcp-project.iam.gserviceaccount.com
```

### Checking Provider conditions

```bash
kubectl describe provider vertex-gemini -n agents
```

Look at the `Conditions` section for `AuthConfigured`, `CredentialConfigured`, and `Ready`.

## Related Resources

- [Provider CRD Reference](/reference/provider/)
- [Configure AWS Bedrock Provider](/how-to/configure-bedrock-provider/)
- [Configure Azure AI Provider](/how-to/configure-azure-ai-provider/)
- [Migrate Provider Credentials](/how-to/migrate-provider-credentials/)
