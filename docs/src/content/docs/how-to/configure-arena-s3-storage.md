---
title: "Configure Arena S3 Result Storage"
description: "Store Arena Fleet evaluation results in S3 or S3-compatible storage"
sidebar:
  order: 10
  badge:
    text: Arena
    variant: note
---

This guide shows how to configure Arena Fleet to store evaluation results in Amazon S3 or S3-compatible storage (MinIO, DigitalOcean Spaces, etc.).

## Prerequisites

- Arena Fleet enabled in your Omnia installation
- An S3 bucket or S3-compatible storage endpoint
- AWS credentials or IAM role with write access to the bucket

## Option 1: AWS S3 with Credentials

### Create a Credentials Secret

Create a Kubernetes secret with your AWS credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: arena-s3-credentials
  namespace: default
type: Opaque
stringData:
  AWS_ACCESS_KEY_ID: "AKIAIOSFODNN7EXAMPLE"
  AWS_SECRET_ACCESS_KEY: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
```

```bash
kubectl apply -f s3-credentials.yaml
```

### Configure the ArenaJob

Reference the credentials in your ArenaJob output configuration:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: nightly-eval
  namespace: default
spec:
  configRef:
    name: my-eval-config
  type: evaluation
  evaluation:
    outputFormats:
      - json
      - junit
  workers:
    replicas: 2
  output:
    type: s3
    s3:
      bucket: my-arena-results
      prefix: "evaluations/nightly/"
      region: us-west-2
      secretRef:
        name: arena-s3-credentials
```

Results will be stored at:
```
s3://my-arena-results/evaluations/nightly/<job-name>/<timestamp>/
```

## Option 2: AWS S3 with IAM Roles (IRSA)

For production deployments on EKS, use IAM Roles for Service Accounts (IRSA) instead of static credentials.

### Create an IAM Policy

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:PutObject",
        "s3:GetObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::my-arena-results",
        "arn:aws:s3:::my-arena-results/*"
      ]
    }
  ]
}
```

### Annotate the Service Account

Configure the arena worker service account with the IAM role:

```yaml
# In your Helm values
arena:
  worker:
    serviceAccount:
      annotations:
        eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/arena-s3-role
```

### Configure the ArenaJob

When using IRSA, omit the `secretRef`:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: nightly-eval
spec:
  configRef:
    name: my-eval-config
  output:
    type: s3
    s3:
      bucket: my-arena-results
      prefix: "evaluations/"
      region: us-west-2
      # No secretRef needed - uses IRSA
```

## Option 3: S3-Compatible Storage (MinIO)

For MinIO or other S3-compatible storage, specify the custom endpoint:

### Create Credentials

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: minio-credentials
  namespace: default
type: Opaque
stringData:
  AWS_ACCESS_KEY_ID: "minioadmin"
  AWS_SECRET_ACCESS_KEY: "minioadmin"
```

### Configure with Custom Endpoint

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: local-eval
spec:
  configRef:
    name: my-eval-config
  output:
    type: s3
    s3:
      bucket: arena-results
      prefix: "evals/"
      endpoint: http://minio.minio-system.svc:9000
      secretRef:
        name: minio-credentials
```

:::note[Region for S3-Compatible Storage]
When using S3-compatible storage, the `region` field is typically ignored but some implementations may require it. Set it to any valid value like `us-east-1`.
:::

## Output Structure

Arena Fleet organizes results in S3 with the following structure:

```
s3://bucket/prefix/
└── <job-name>/
    └── <timestamp>/
        ├── results.json      # Full evaluation results
        ├── results.junit.xml # JUnit format (if requested)
        ├── summary.json      # Aggregated metrics
        └── scenarios/
            ├── scenario-1.json
            └── scenario-2.json
```

## Accessing Results

### Using AWS CLI

```bash
# List results
aws s3 ls s3://my-arena-results/evaluations/nightly/

# Download results
aws s3 cp s3://my-arena-results/evaluations/nightly/eval-001/results.json .
```

### From ArenaJob Status

The job status includes the result URL:

```bash
kubectl get arenajob nightly-eval -o jsonpath='{.status.result.url}'
```

## Global Default Storage

Configure default S3 storage for all Arena jobs in your Helm values:

```yaml
arena:
  storage:
    type: s3
    s3:
      bucket: arena-results
      region: us-west-2
      prefix: "omnia/"
      secretRef: arena-s3-credentials
```

Jobs can override this default or omit the `output` section to use the global configuration.

## Troubleshooting

### Access Denied Errors

Verify your credentials have the required permissions:

```bash
# Test with AWS CLI
aws s3 ls s3://my-arena-results/
aws s3 cp test.txt s3://my-arena-results/test.txt
```

### Endpoint Connection Issues

For S3-compatible storage, ensure:
- The endpoint URL is reachable from within the cluster
- Use `http://` for non-TLS endpoints
- The bucket exists (some implementations require pre-created buckets)

### Check Worker Logs

```bash
kubectl logs -l arena.omnia.altairalabs.ai/job=<job-name> | grep -i s3
```

## Related Resources

- **[ArenaJob Reference](/reference/arenajob/#output)**: Complete output configuration options
- **[Helm Values: Arena Storage](/reference/helm-values/#result-storage)**: Global storage configuration
- **[Monitor Arena Jobs](/how-to/monitor-arena-jobs/)**: Track job progress and results
