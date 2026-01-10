---
title: "Configure Media Storage"
description: "Set up media storage backends for file uploads in Omnia agents"
sidebar:
  order: 7
---

Media storage allows agents to handle file uploads for multi-modal conversations. This guide covers configuring different storage backends.

## Overview

When enabled, the facade container provides HTTP endpoints for:

- **Upload requests** - Get presigned URLs for uploading files
- **File uploads** - Upload files directly (local storage) or to cloud (S3/GCS/Azure)
- **File downloads** - Retrieve uploaded files
- **Media info** - Get metadata about uploaded files

## Storage Backends

Omnia supports four storage backends:

| Backend | Use Case | Authentication |
|---------|----------|----------------|
| `none` | Disable media storage (default) | N/A |
| `local` | Development, single-replica deployments | N/A |
| `s3` | AWS deployments, S3-compatible services (MinIO) | IAM roles, workload identity, access keys |
| `gcs` | Google Cloud deployments | Workload identity, service account keys |
| `azure` | Azure deployments | Managed identity, workload identity, account keys |

## Local Storage

Local storage writes files to the pod's filesystem. This is suitable for development and single-replica deployments.

```yaml
# Environment variables
OMNIA_MEDIA_STORAGE_TYPE: local
OMNIA_MEDIA_STORAGE_PATH: /var/lib/omnia/media
OMNIA_MEDIA_MAX_FILE_SIZE: "104857600"  # 100MB
OMNIA_MEDIA_DEFAULT_TTL: "24h"
```

**Limitations:**
- Files are lost when the pod restarts
- Not suitable for multi-replica deployments
- No CDN integration

## S3 Storage

S3 storage supports Amazon S3 and S3-compatible services like MinIO and LocalStack.

### AWS S3

```yaml
# Environment variables
OMNIA_MEDIA_STORAGE_TYPE: s3
OMNIA_MEDIA_S3_BUCKET: my-media-bucket
OMNIA_MEDIA_S3_REGION: us-west-2
OMNIA_MEDIA_S3_PREFIX: omnia/media/  # Optional
OMNIA_MEDIA_MAX_FILE_SIZE: "104857600"
OMNIA_MEDIA_DEFAULT_TTL: "24h"
```

### MinIO / S3-Compatible

```yaml
OMNIA_MEDIA_STORAGE_TYPE: s3
OMNIA_MEDIA_S3_BUCKET: my-bucket
OMNIA_MEDIA_S3_REGION: us-east-1
OMNIA_MEDIA_S3_ENDPOINT: http://minio.minio-system:9000
OMNIA_MEDIA_S3_PREFIX: media/
```

When `OMNIA_MEDIA_S3_ENDPOINT` is set, path-style addressing is automatically enabled for compatibility with MinIO.

### Authentication

S3 storage uses the AWS SDK default credential chain:

1. **Environment variables** - `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`
2. **IAM Roles for Service Accounts (IRSA)** - Recommended for EKS
3. **EC2 instance profiles** - For self-managed Kubernetes on EC2
4. **Shared credentials file** - `~/.aws/credentials`

#### IRSA Setup (EKS)

```yaml
# ServiceAccount annotation
apiVersion: v1
kind: ServiceAccount
metadata:
  name: omnia-agent
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/omnia-media-role
```

Required IAM policy:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:PutObject",
        "s3:GetObject",
        "s3:DeleteObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::my-media-bucket",
        "arn:aws:s3:::my-media-bucket/*"
      ]
    }
  ]
}
```

## GCS Storage

Google Cloud Storage uses presigned URLs for direct uploads.

```yaml
OMNIA_MEDIA_STORAGE_TYPE: gcs
OMNIA_MEDIA_GCS_BUCKET: my-media-bucket
OMNIA_MEDIA_GCS_PREFIX: omnia/media/  # Optional
OMNIA_MEDIA_MAX_FILE_SIZE: "104857600"
OMNIA_MEDIA_DEFAULT_TTL: "24h"
```

### Authentication

GCS storage uses Google's Application Default Credentials:

1. **Workload Identity** - Recommended for GKE
2. **Service account key file** - `GOOGLE_APPLICATION_CREDENTIALS`
3. **Compute Engine default service account**

#### Workload Identity Setup (GKE)

```yaml
# ServiceAccount annotation
apiVersion: v1
kind: ServiceAccount
metadata:
  name: omnia-agent
  annotations:
    iam.gke.io/gcp-service-account: omnia-media@my-project.iam.gserviceaccount.com
```

Required IAM role: `roles/storage.objectAdmin` on the bucket.

## Azure Blob Storage

Azure Blob Storage uses SAS tokens for presigned URLs.

```yaml
OMNIA_MEDIA_STORAGE_TYPE: azure
OMNIA_MEDIA_AZURE_ACCOUNT: mystorageaccount
OMNIA_MEDIA_AZURE_CONTAINER: media
OMNIA_MEDIA_AZURE_PREFIX: omnia/media/  # Optional
OMNIA_MEDIA_MAX_FILE_SIZE: "104857600"
OMNIA_MEDIA_DEFAULT_TTL: "24h"
```

### Authentication

Azure storage supports multiple authentication methods:

1. **DefaultAzureCredential** - Managed identity, workload identity, Azure CLI
2. **Account Key** - Explicit key via `OMNIA_MEDIA_AZURE_KEY`

#### Workload Identity Setup (AKS)

```yaml
# ServiceAccount annotation
apiVersion: v1
kind: ServiceAccount
metadata:
  name: omnia-agent
  annotations:
    azure.workload.identity/client-id: <client-id>
  labels:
    azure.workload.identity/use: "true"
```

**Note:** SAS URL generation currently requires an account key. For full workload identity support without keys, User Delegation SAS would need to be implemented (requires `Storage Blob Delegator` role).

#### Cross-Cloud / Explicit Credentials

For cross-cloud scenarios or when workload identity isn't available:

```yaml
OMNIA_MEDIA_AZURE_KEY: <storage-account-key>
```

Store the key in a Kubernetes Secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: azure-storage-key
type: Opaque
stringData:
  account-key: <your-storage-account-key>
---
# Reference in AgentRuntime
spec:
  env:
    - name: OMNIA_MEDIA_AZURE_KEY
      valueFrom:
        secretKeyRef:
          name: azure-storage-key
          key: account-key
```

## Upload Flow

### Local Storage

```mermaid
sequenceDiagram
    participant C as Client
    participant F as Facade
    participant FS as Filesystem

    C->>F: POST /media/request-upload
    F-->>C: {uploadId, url: "/media/upload/{id}"}
    C->>F: PUT /media/upload/{id}
    F->>FS: Write file
    F-->>C: 204 No Content
```

### Cloud Storage (S3/GCS/Azure)

```mermaid
sequenceDiagram
    participant C as Client
    participant F as Facade
    participant CS as Cloud Storage

    C->>F: POST /media/request-upload
    F-->>C: {uploadId, url: "presigned-url", storageRef}
    C->>CS: PUT presigned-url (direct upload)
    CS-->>C: 200 OK
    C->>F: POST /media/confirm-upload/{id}
    F->>CS: Verify object exists
    F-->>C: {mediaInfo}
```

## Prometheus Metrics

Media storage exposes the following metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `omnia_facade_uploads_total` | Counter | Upload attempts by status (success/failed) |
| `omnia_facade_upload_bytes_total` | Counter | Total bytes uploaded |
| `omnia_facade_upload_duration_seconds` | Histogram | Upload duration |
| `omnia_facade_downloads_total` | Counter | Download attempts by status |
| `omnia_facade_download_bytes_total` | Counter | Total bytes downloaded |
| `omnia_facade_media_chunks_total` | Counter | Media chunks sent by type (json/binary) |
| `omnia_facade_media_chunk_bytes_total` | Counter | Total bytes sent as chunks |

All metrics include `agent` and `namespace` labels.

## Helm Configuration

Configure media storage in your values.yaml:

```yaml
# Coming in a future release - currently configured via environment variables
# facade:
#   media:
#     storage:
#       type: s3
#       s3:
#         bucket: my-bucket
#         region: us-west-2
```

For now, configure via environment variables in the AgentRuntime spec:

```yaml
apiVersion: omnia.altairalabs.dev/v1alpha1
kind: AgentRuntime
metadata:
  name: my-agent
spec:
  facade:
    env:
      - name: OMNIA_MEDIA_STORAGE_TYPE
        value: s3
      - name: OMNIA_MEDIA_S3_BUCKET
        value: my-media-bucket
      - name: OMNIA_MEDIA_S3_REGION
        value: us-west-2
```

## Troubleshooting

### Upload fails with "upload not found or expired"

The upload URL has expired. Default TTL is 15 minutes. Request a new upload URL.

### S3 "Access Denied" errors

1. Verify IAM role has required permissions
2. Check IRSA annotation on ServiceAccount
3. Verify bucket policy allows the IAM role

### GCS "Permission denied" errors

1. Verify Workload Identity is configured
2. Check service account has `storage.objectAdmin` role
3. Verify bucket IAM bindings

### Azure "SAS generation requires shared key credential"

Azure SAS URL generation requires the account key. Set `OMNIA_MEDIA_AZURE_KEY` or implement User Delegation SAS for full workload identity support.
