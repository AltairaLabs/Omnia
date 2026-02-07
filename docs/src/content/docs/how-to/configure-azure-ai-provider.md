---
title: "Configure Azure AI Provider"
description: "Set up Omnia to use Azure AI (Azure OpenAI) via Azure AD Workload Identity or service principal"
sidebar:
  order: 18
---

This guide covers how to configure an Omnia Provider to use Azure AI Services (Azure OpenAI) for LLM access. Azure AI providers support two authentication methods: **Azure AD Workload Identity** for production use, and **service principals** for simpler setups.

## Prerequisites

- An AKS cluster with OIDC issuer enabled
- An Azure AI (Azure OpenAI) resource deployed
- `az` CLI installed and authenticated
- Omnia operator installed in the cluster

## Option 1: Workload Identity — Recommended

Azure AD Workload Identity lets Kubernetes pods authenticate as a managed identity without storing credentials. This is the recommended approach for production.

### 1. Create a managed identity

```bash
az identity create \
  --name omnia-azure-ai \
  --resource-group my-resource-group \
  --location eastus
```

Note the `clientId` from the output — you'll need it later.

### 2. Assign the Cognitive Services role

```bash
az role assignment create \
  --assignee <managed-identity-client-id> \
  --role "Cognitive Services OpenAI User" \
  --scope /subscriptions/<subscription-id>/resourceGroups/<resource-group>/providers/Microsoft.CognitiveServices/accounts/<resource-name>
```

### 3. Establish a federated credential

Get the OIDC issuer URL from your AKS cluster:

```bash
az aks show \
  --name my-cluster \
  --resource-group my-resource-group \
  --query "oidcIssuerProfile.issuerUrl" -o tsv
```

Create the federated credential:

```bash
az identity federated-credential create \
  --name omnia-federated \
  --identity-name omnia-azure-ai \
  --resource-group my-resource-group \
  --issuer <oidc-issuer-url> \
  --subject system:serviceaccount:agents:omnia-agent \
  --audiences api://AzureADTokenExchange
```

### 4. Annotate the service account

Configure the service account via Helm values:

```yaml
# values.yaml
serviceAccount:
  labels:
    azure.workload.identity/use: "true"
  annotations:
    azure.workload.identity/client-id: <managed-identity-client-id>
```

### 5. Create the Provider

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: azure-openai
  namespace: agents
spec:
  type: azure-ai
  model: gpt-4o

  platform:
    type: azure
    region: eastus
    endpoint: https://my-resource.openai.azure.com

  auth:
    type: workloadIdentity

  capabilities:
    - text
    - streaming
    - tools
    - json
```

### 6. Verify

```bash
kubectl get provider azure-openai -n agents -o wide
kubectl get provider azure-openai -n agents -o jsonpath='{.status.conditions}' | jq .
```

Both the `AuthConfigured` and `Ready` conditions should be `True`.

## Option 2: Service Principal

For development or environments without Workload Identity, you can use service principal credentials.

### 1. Create a service principal

```bash
az ad sp create-for-rbac \
  --name omnia-azure-ai-sp \
  --role "Cognitive Services OpenAI User" \
  --scopes /subscriptions/<subscription-id>/resourceGroups/<resource-group>/providers/Microsoft.CognitiveServices/accounts/<resource-name>
```

### 2. Create a Secret

```bash
kubectl create secret generic azure-credentials \
  --namespace agents \
  --from-literal=AZURE_CLIENT_ID=<app-id> \
  --from-literal=AZURE_CLIENT_SECRET=<password> \
  --from-literal=AZURE_TENANT_ID=<tenant-id>
```

### 3. Create the Provider

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: azure-openai
  namespace: agents
spec:
  type: azure-ai
  model: gpt-4o

  platform:
    type: azure
    region: eastus
    endpoint: https://my-resource.openai.azure.com

  auth:
    type: servicePrincipal
    credentialsSecretRef:
      name: azure-credentials

  capabilities:
    - text
    - streaming
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
    name: azure-openai
  facade:
    type: websocket
    port: 8080
```

## Troubleshooting

### Endpoint URL format

The `platform.endpoint` must be the full Azure OpenAI resource URL, including `https://` and the `.openai.azure.com` suffix:

```
https://my-resource.openai.azure.com
```

Do not include a trailing slash or API version path.

### Identity not federated

If using workload identity and the Provider shows `AuthConfigured: False`, verify the federated credential exists:

```bash
az identity federated-credential list \
  --identity-name omnia-azure-ai \
  --resource-group my-resource-group
```

Ensure the `subject` matches `system:serviceaccount:<namespace>:<service-account-name>`.

### Role assignment missing

Verify the managed identity or service principal has the correct role:

```bash
az role assignment list \
  --assignee <client-id> \
  --scope /subscriptions/<subscription-id>/resourceGroups/<resource-group>/providers/Microsoft.CognitiveServices/accounts/<resource-name>
```

### Checking Provider conditions

```bash
kubectl describe provider azure-openai -n agents
```

Look at the `Conditions` section for `AuthConfigured`, `CredentialConfigured`, and `Ready`.

## Related Resources

- [Provider CRD Reference](/reference/provider/)
- [Configure AWS Bedrock Provider](/how-to/configure-bedrock-provider/)
- [Configure GCP Vertex AI Provider](/how-to/configure-vertex-provider/)
- [Migrate Provider Credentials](/how-to/migrate-provider-credentials/)
