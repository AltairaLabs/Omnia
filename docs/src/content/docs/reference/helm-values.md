---
title: "Helm Values Reference"
description: "Complete reference for Omnia Helm chart configuration"
sidebar:
  order: 5
---


This document covers all configuration options for the Omnia Helm chart.

## Installation

```bash
helm install omnia oci://ghcr.io/altairalabs/omnia \
  --namespace omnia-system \
  --create-namespace \
  -f values.yaml
```

## Operator Configuration

### Basic Settings

```yaml
replicaCount: 1

image:
  repository: ghcr.io/altairalabs/omnia
  pullPolicy: IfNotPresent
  tag: ""  # Defaults to Chart appVersion

imagePullSecrets: []
nameOverride: ""
fullnameOverride: ""
```

### Service Account

```yaml
serviceAccount:
  create: true
  annotations: {}
  name: ""  # Generated if not set
```

### Pod Configuration

```yaml
podAnnotations: {}

podSecurityContext:
  runAsNonRoot: true
  seccompProfile:
    type: RuntimeDefault

securityContext:
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  capabilities:
    drop:
      - ALL
```

### Resources

```yaml
resources:
  limits:
    cpu: 500m
    memory: 128Mi
  requests:
    cpu: 10m
    memory: 64Mi
```

### Scheduling

```yaml
nodeSelector: {}
tolerations: []
affinity: {}
```

### Leader Election

```yaml
leaderElection:
  enabled: true  # Enable for HA deployments
```

### Health Probes

```yaml
probes:
  port: 8081
  liveness:
    initialDelaySeconds: 15
    periodSeconds: 20
  readiness:
    initialDelaySeconds: 5
    periodSeconds: 10
```

### Metrics

```yaml
metrics:
  enabled: false
  port: 8443
  secure: true
```

### Webhooks

```yaml
webhook:
  enabled: false
  port: 9443
```

### RBAC

```yaml
rbac:
  create: true
```

### CRDs

```yaml
crds:
  install: true
```

## Facade Configuration

```yaml
facade:
  image:
    repository: ghcr.io/altairalabs/omnia-facade
    tag: ""  # Defaults to Chart appVersion
```

### Media Storage

The facade can optionally provide media storage for file uploads. Configure via environment variables in your AgentRuntime.

#### Local Storage (Development)

```yaml
# AgentRuntime spec.facade.env
- name: OMNIA_MEDIA_STORAGE_TYPE
  value: local
- name: OMNIA_MEDIA_STORAGE_PATH
  value: /var/lib/omnia/media
- name: OMNIA_MEDIA_MAX_FILE_SIZE
  value: "104857600"  # 100MB
- name: OMNIA_MEDIA_DEFAULT_TTL
  value: "24h"
```

#### S3 Storage (AWS / MinIO)

```yaml
- name: OMNIA_MEDIA_STORAGE_TYPE
  value: s3
- name: OMNIA_MEDIA_S3_BUCKET
  value: my-media-bucket
- name: OMNIA_MEDIA_S3_REGION
  value: us-west-2
- name: OMNIA_MEDIA_S3_PREFIX
  value: omnia/media/
# Optional: for MinIO or S3-compatible services
- name: OMNIA_MEDIA_S3_ENDPOINT
  value: http://minio:9000
```

#### GCS Storage (Google Cloud)

```yaml
- name: OMNIA_MEDIA_STORAGE_TYPE
  value: gcs
- name: OMNIA_MEDIA_GCS_BUCKET
  value: my-media-bucket
- name: OMNIA_MEDIA_GCS_PREFIX
  value: omnia/media/
```

#### Azure Blob Storage

```yaml
- name: OMNIA_MEDIA_STORAGE_TYPE
  value: azure
- name: OMNIA_MEDIA_AZURE_ACCOUNT
  value: mystorageaccount
- name: OMNIA_MEDIA_AZURE_CONTAINER
  value: media
- name: OMNIA_MEDIA_AZURE_PREFIX
  value: omnia/media/
# Optional: for cross-cloud or explicit credentials
- name: OMNIA_MEDIA_AZURE_KEY
  valueFrom:
    secretKeyRef:
      name: azure-storage-key
      key: account-key
```

See [Configure Media Storage](/how-to/configure-media-storage/) for detailed setup instructions.

## Framework Configuration

The framework image is used for the agent runtime container. This naming aligns with the CRD's `spec.framework` field.

```yaml
framework:
  image:
    repository: ghcr.io/altairalabs/omnia-runtime
    tag: ""  # Defaults to Chart appVersion
```

## Dashboard Configuration

The Omnia Dashboard provides a web UI for monitoring and managing agents.

### Basic Settings

```yaml
dashboard:
  enabled: true

  image:
    repository: ghcr.io/altairalabs/omnia-dashboard
    pullPolicy: IfNotPresent
    tag: ""  # Defaults to Chart appVersion

  replicaCount: 1

  service:
    type: ClusterIP
    port: 3000

  resources:
    limits:
      cpu: 500m
      memory: 256Mi
    requests:
      cpu: 100m
      memory: 128Mi
```

### Ingress

```yaml
dashboard:
  ingress:
    enabled: true
    className: nginx
    annotations: {}
    host: dashboard.example.com
    tls:
      - secretName: dashboard-tls
        hosts:
          - dashboard.example.com
```

### Authentication

```yaml
dashboard:
  auth:
    # Mode: anonymous, proxy, oauth, or builtin
    mode: anonymous

    # Session configuration (required for non-anonymous modes)
    sessionSecret: ""  # Generate with: openssl rand -base64 32
    existingSessionSecret: ""  # Use existing secret
    cookieName: omnia_session
    ttl: 86400  # 24 hours

    # Role mapping
    anonymousRole: viewer
    roleMapping:
      adminGroups: []
      editorGroups: []
```

#### Proxy Authentication

For use with external authentication proxies (e.g., oauth2-proxy):

```yaml
dashboard:
  auth:
    mode: proxy
  proxy:
    headerUser: X-Forwarded-User
    headerEmail: X-Forwarded-Email
    headerGroups: X-Forwarded-Groups
```

#### OAuth Authentication

For direct OAuth/OIDC integration:

```yaml
dashboard:
  auth:
    mode: oauth
  oauth:
    issuer: https://your-tenant.auth0.com/
    clientId: your-client-id
    clientSecret: your-client-secret  # Or use existingSecret
    scopes:
      - openid
      - email
      - profile
    groupsClaim: groups
```

## Observability Stack

All observability components are optional and disabled by default.

### Prometheus

```yaml
prometheus:
  enabled: true
  server:
    persistentVolume:
      enabled: false  # Enable for production
      size: 50Gi
    prefixURL: /prometheus
    baseURL: /prometheus
    resources:
      requests:
        cpu: 500m
        memory: 512Mi
  alertmanager:
    enabled: false
  prometheus-pushgateway:
    enabled: false
  prometheus-node-exporter:
    enabled: false
  kube-state-metrics:
    enabled: false
```

### Grafana

```yaml
grafana:
  enabled: true
  adminPassword: admin  # Change in production!
  grafana.ini:
    server:
      root_url: "%(protocol)s://%(domain)s:%(http_port)s/grafana/"
      serve_from_sub_path: true
  sidecar:
    dashboards:
      enabled: true
      label: grafana_dashboard
      searchNamespace: ALL
    datasources:
      enabled: true
      label: grafana_datasource
      searchNamespace: ALL
  service:
    type: ClusterIP
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
```

### Loki

:::note[Docker Desktop]
Loki 3.0's ruler component can fail with "no space left on device" errors on Docker Desktop with local-path provisioner. The ruler is disabled by default in Omnia's configuration to avoid this issue. See below for a workaround if you need log-based alerting.
:::

```yaml
loki:
  enabled: true
  deploymentMode: SingleBinary
  loki:
    auth_enabled: false
    useTestSchema: true
    storage:
      type: filesystem
    commonConfig:
      replication_factor: 1
  singleBinary:
    replicas: 1
    persistence:
      enabled: true
      size: 10Gi
  backend:
    replicas: 0
  read:
    replicas: 0
  write:
    replicas: 0
  gateway:
    enabled: false
  chunksCache:
    enabled: false
  resultsCache:
    enabled: false
  ruler:
    enabled: false  # Prevents mkdir errors on Docker Desktop
```

#### Enabling the Ruler (Log-based Alerting)

The ruler is disabled by default because it fails on Docker Desktop. If you need log-based alerting rules, enable the ruler with a writable emptyDir volume:

```yaml
loki:
  ruler:
    enabled: true
  singleBinary:
    extraVolumes:
      - name: rules
        emptyDir: {}
    extraVolumeMounts:
      - name: rules
        mountPath: /var/loki/rules
```

**Note:** The ruler allows you to:
- Define alerting rules that fire based on LogQL queries
- Create recording rules to pre-aggregate expensive queries

For most dev/test environments, the ruler is not needed—log ingestion and querying work without it.

### Alloy

```yaml
alloy:
  enabled: true
  alloy:
    configMap:
      content: |
        // Kubernetes pod discovery and log collection
        // See values.yaml for full configuration
```

### Tempo

```yaml
tempo:
  enabled: true
  tempo:
    reportingEnabled: false
  tempoQuery:
    enabled: false
  persistence:
    enabled: false  # Enable for production
    size: 10Gi
```

## Gateway API

### External Gateway

For exposing agents externally:

```yaml
gateway:
  enabled: true
  name: agents
  className: istio
  listeners:
    http:
      port: 80
      protocol: HTTP
    https:
      enabled: false
      port: 443
      protocol: HTTPS
      tlsSecretName: ""
```

### Internal Gateway

For observability tools:

```yaml
internalGateway:
  enabled: true
  name: internal
  className: istio
  port: 8080
  grafana:
    enabled: true
    path: /grafana
  prometheus:
    enabled: true
    path: /prometheus
```

## Istio Integration

```yaml
istio:
  enabled: false
  tempoService: omnia-tempo.omnia-system.svc.cluster.local
  tempoPort: 4317
```

## Authentication

JWT-based authentication using Istio RequestAuthentication:

```yaml
authentication:
  enabled: false
  jwt:
    issuer: ""  # e.g., https://your-tenant.auth0.com/
    jwksUri: ""  # Defaults to {issuer}/.well-known/jwks.json
    audiences: []
    forwardOriginalToken: true
    outputClaimToHeaders: []
    # - header: x-user-id
    #   claim: sub
  authorization:
    requiredClaims: []
    # - claim: "scope"
    #   values: ["agents:access"]
    excludePaths:
      - /healthz
      - /readyz
```

## KEDA

Advanced autoscaling with scale-to-zero support.

:::caution[Existing KEDA Installation]
If KEDA is **already installed** in your cluster (e.g., via `helm install keda kedacore/keda`), you **must** keep `keda.enabled=false` to avoid CRD ownership conflicts. The chart automatically detects existing KEDA installations and will fail with a helpful error message if you try to enable the subchart.

Your existing KEDA installation will work seamlessly with Omnia's ScaledObject resources.
:::

```yaml
keda:
  enabled: false  # Set to true ONLY if KEDA is not already installed
  operator:
    watchNamespace: ""  # Empty = all namespaces
  prometheus:
    serverAddress: "http://omnia-prometheus-server.omnia-system.svc.cluster.local"
```

## Demo Mode

Demo mode deploys Ollama (local LLM) with a vision-capable agent, allowing users to try Omnia without external API keys:

```yaml
demo:
  enabled: false
  namespace: omnia-demo

  ollama:
    image:
      repository: ollama/ollama
      tag: latest
      pullPolicy: IfNotPresent
    model: llava:7b  # Vision-capable model
    keepAlive: "24h"
    resources:
      requests:
        memory: "4Gi"
        cpu: "2"
      limits:
        memory: "16Gi"
        cpu: "8"
    persistence:
      enabled: true
      size: 20Gi
      storageClass: ""

  agent:
    name: vision-demo
    handler: runtime
    replicas: 1
    resources:
      requests:
        cpu: "100m"
        memory: "128Mi"
      limits:
        cpu: "500m"
        memory: "256Mi"

  promptPack:
    systemPrompt: |
      You are a helpful vision-capable AI assistant...
```

### Requirements

- **RAM**: Minimum 8GB, 16GB recommended
- **Disk**: ~10GB for the llava:7b model
- **CPU**: 4+ cores (GPU optional but significantly faster)

### Usage

```bash
helm install omnia oci://ghcr.io/altairalabs/omnia \
  --namespace omnia-system \
  --create-namespace \
  --set demo.enabled=true
```

Once deployed, the demo agent is accessible at `vision-demo.omnia-demo.svc:8080`.

## Example Configurations

### Minimal (Development)

```yaml
prometheus:
  enabled: true
grafana:
  enabled: true
```

### Production

```yaml
replicaCount: 2

resources:
  limits:
    cpu: 1000m
    memory: 256Mi
  requests:
    cpu: 100m
    memory: 128Mi

leaderElection:
  enabled: true

prometheus:
  enabled: true
  server:
    persistentVolume:
      enabled: true
      size: 100Gi

grafana:
  enabled: true
  adminPassword: ""  # Use existingSecret instead
  admin:
    existingSecret: grafana-admin
    userKey: admin-user
    passwordKey: admin-password

loki:
  enabled: true
  singleBinary:
    persistence:
      enabled: true
      size: 50Gi

tempo:
  enabled: true
  persistence:
    enabled: true
    size: 20Gi

gateway:
  enabled: true
  listeners:
    https:
      enabled: true
      tlsSecretName: agents-tls

istio:
  enabled: true

authentication:
  enabled: true
  jwt:
    issuer: "https://auth.example.com"
    audiences:
      - "agents-api"

keda:
  enabled: true
```

### Observability Only

Use existing agent deployments with just observability:

```yaml
prometheus:
  enabled: true
grafana:
  enabled: true
loki:
  enabled: true
alloy:
  enabled: true
tempo:
  enabled: true

gateway:
  enabled: false

authentication:
  enabled: false

keda:
  enabled: false
```

### Demo Mode (Try Without API Keys)

Quick-start demo with local Ollama LLM:

```yaml
demo:
  enabled: true
  ollama:
    persistence:
      enabled: true
      size: 20Gi

dashboard:
  enabled: true
```
