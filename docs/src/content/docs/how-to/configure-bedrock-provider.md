---
title: "Configure AWS Bedrock Provider"
description: "Set up Omnia to use AWS Bedrock for LLM access via IRSA or access keys"
sidebar:
  order: 16
---

This guide covers how to configure an Omnia Provider to use AWS Bedrock for LLM access. Bedrock providers support two authentication methods: **workload identity (IRSA)** for production use, and **access keys** for simpler setups.

## Prerequisites

- An EKS cluster with the OIDC provider enabled
- AWS Bedrock model access enabled in your target region ([enable model access](https://docs.aws.amazon.com/bedrock/latest/userguide/model-access.html))
- `eksctl` and `aws` CLI installed
- Omnia operator installed in the cluster

## Option 1: Workload Identity (IRSA) — Recommended

IAM Roles for Service Accounts (IRSA) lets pods assume an IAM role without static credentials. This is the recommended approach for production.

### 1. Create an IAM policy

Create a policy that grants access to Bedrock model invocation:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "bedrock:InvokeModel",
        "bedrock:InvokeModelWithResponseStream"
      ],
      "Resource": "*"
    }
  ]
}
```

Save this as `bedrock-policy.json` and create the policy:

```bash
aws iam create-policy \
  --policy-name OmniaBedrock \
  --policy-document file://bedrock-policy.json
```

### 2. Create an IAM role with OIDC trust

Use `eksctl` to create a role bound to the Omnia service account:

```bash
eksctl create iamserviceaccount \
  --name omnia-agent \
  --namespace agents \
  --cluster my-cluster \
  --role-name omnia-bedrock-role \
  --attach-policy-arn arn:aws:iam::123456789012:policy/OmniaBedrock \
  --approve
```

Alternatively, annotate the service account via Helm values:

```yaml
# values.yaml
serviceAccount:
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/omnia-bedrock-role
```

### 3. Create the Provider

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: bedrock-claude
  namespace: agents
spec:
  type: bedrock
  model: anthropic.claude-3-5-sonnet-20241022-v2:0

  platform:
    type: aws
    region: us-east-1

  auth:
    type: workloadIdentity
    roleArn: arn:aws:iam::123456789012:role/omnia-bedrock-role

  capabilities:
    - text
    - streaming
    - vision
    - tools
```

### 4. Verify

```bash
kubectl get provider bedrock-claude -n agents -o wide
kubectl get provider bedrock-claude -n agents -o jsonpath='{.status.conditions}' | jq .
```

Both the `AuthConfigured` and `Ready` conditions should be `True`.

## Option 2: Access Key

For development or environments without IRSA, you can use static AWS credentials.

### 1. Create a Secret

```bash
kubectl create secret generic aws-credentials \
  --namespace agents \
  --from-literal=AWS_ACCESS_KEY_ID=AKIA... \
  --from-literal=AWS_SECRET_ACCESS_KEY=...
```

### 2. Create the Provider

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: bedrock-claude
  namespace: agents
spec:
  type: bedrock
  model: anthropic.claude-3-5-sonnet-20241022-v2:0

  platform:
    type: aws
    region: us-east-1

  auth:
    type: accessKey
    credentialsSecretRef:
      name: aws-credentials

  capabilities:
    - text
    - streaming
    - vision
    - tools
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
    name: bedrock-claude
  facade:
    type: websocket
    port: 8080
```

## Troubleshooting

### Model access not enabled

If the Provider shows an error, verify that model access is enabled in the target region:

```bash
aws bedrock list-foundation-models --region us-east-1 \
  --query "modelSummaries[?modelId=='anthropic.claude-3-5-sonnet-20241022-v2:0']"
```

### Region mismatch

Ensure the `platform.region` in the Provider spec matches the region where you enabled Bedrock model access. Bedrock model availability varies by region.

### IRSA annotation missing

If using workload identity, verify the service account has the correct annotation:

```bash
kubectl get sa omnia-agent -n agents -o jsonpath='{.metadata.annotations}'
```

Look for `eks.amazonaws.com/role-arn` pointing to the correct role.

### Checking Provider conditions

```bash
kubectl describe provider bedrock-claude -n agents
```

Look at the `Conditions` section for `AuthConfigured`, `CredentialConfigured`, and `Ready`.

## Related Resources

- [Provider CRD Reference](/reference/provider/)
- [Configure GCP Vertex AI Provider](/how-to/configure-vertex-provider/)
- [Configure Azure AI Provider](/how-to/configure-azure-ai-provider/)
- [Migrate Provider Credentials](/how-to/migrate-provider-credentials/)
