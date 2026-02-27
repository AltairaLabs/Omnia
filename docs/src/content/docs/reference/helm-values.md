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
    memory: 256Mi
  requests:
    cpu: 10m
    memory: 128Mi
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

## Enterprise Features

Enterprise features require a valid license key. Enable them with:

```yaml
enterprise:
  enabled: true  # Enable enterprise features (Arena Fleet, etc.)
```

When `enterprise.enabled` is `true`:
- Arena CRDs are installed:
  - [ArenaSource](/reference/arenasource) - PromptKit bundle sources
  - [ArenaJob](/reference/arenajob) - Test execution
  - [ArenaTemplateSource](/reference/arena-template-source) - Project templates
  - [ArenaDevSession](/reference/arena-dev-session) - Interactive testing sessions
- Arena controllers are deployed
- Workspace shared filesystem features are available
- Project Editor with LSP validation (when `promptkitLsp.enabled`)

:::note[License Required]
Enterprise features require a valid license. See [Installing a License](/how-to/install-license/) for details.
:::

## Eval Worker Configuration

The eval worker runs realtime evals for non-PromptKit agents (Pattern A). This is an **enterprise feature**.

### Basic Settings

```yaml
enterprise:
  evalWorker:
    enabled: false       # Enable eval worker deployment
    replicaCount: 1      # Number of eval worker replicas
    image:
      repository: ghcr.io/altairalabs/omnia-eval-worker
      tag: ""            # Defaults to Chart appVersion
      pullPolicy: IfNotPresent
    resources:
      limits:
        cpu: 500m
        memory: 256Mi
      requests:
        cpu: 100m
        memory: 128Mi
```

### Multi-Namespace Mode

By default, the eval worker watches only its deployment namespace (single-namespace mode with `Role`/`RoleBinding` RBAC). To watch multiple namespaces from a single worker, set the `namespaces` list:

```yaml
enterprise:
  evalWorker:
    enabled: true
    namespaces:
      - production
      - staging
      - dev
```

When `namespaces` has multiple entries:
- The worker subscribes to Redis streams for all listed namespaces
- RBAC switches from namespace-scoped `Role` to cluster-scoped `ClusterRole`
- A single consumer group is shared across all streams

When `namespaces` is empty (the default), the worker uses the deployment namespace and namespace-scoped RBAC.

### Extra Environment Variables

Pass additional environment variables (e.g., provider API keys for LLM judges):

```yaml
enterprise:
  evalWorker:
    extraEnv:
      - name: OPENAI_API_KEY
        valueFrom:
          secretKeyRef:
            name: llm-provider-keys
            key: openai-api-key
```

## Arena Fleet Configuration

Arena Fleet provides distributed testing for PromptKit bundles. This is an **enterprise feature**.

### Basic Settings

```yaml
enterprise:
  enabled: true  # Required for Arena Fleet

      image:
        repository: ghcr.io/altairalabs/arena-worker
        tag: ""  # Defaults to Chart appVersion
        pullPolicy: IfNotPresent
      resources:
        limits:
          cpu: 1000m
          memory: 512Mi
        requests:
          cpu: 100m
          memory: 128Mi
```

### Source Configuration

```yaml
enterprise:
  arena:
    source:
      defaultInterval: 5m  # Interval between source checks
      fetchTimeout: 2m     # Timeout for fetching sources
```

### Result Storage

```yaml
enterprise:
  arena:
    storage:
      type: memory  # memory (testing), s3, or pvc

      # S3 configuration (when type: s3)
      s3:
        bucket: ""
        region: ""
        endpoint: ""      # For S3-compatible storage
        secretRef: ""     # Secret with AWS credentials

      # PVC configuration (when type: pvc)
      pvc:
        claimName: ""
        storageClass: ""
        size: 10Gi
```

When Arena Fleet is enabled, the operator will watch for:
- **ArenaSource**: Git, OCI, or ConfigMap sources for PromptKit bundles
- **ArenaConfig**: Test configuration referencing sources and providers
- **ArenaJob**: Job execution with worker pods

See the [Arena Fleet documentation](/explanation/arena-fleet/) for architecture details.

### Arena Queue (Redis)

Arena Fleet uses a work queue for distributing tasks to workers. By default, an in-memory queue is used for development. For production, use Redis.

#### Queue Type

```yaml
enterprise:
  arena:
    queue:
      type: memory  # memory (dev) or redis (production)
```

#### Managed Redis (Bitnami Subchart)

Deploy a Redis instance using the Bitnami Redis subchart:

```yaml
redis:
  enabled: true
  architecture: standalone  # standalone or replication
  auth:
    enabled: false  # Enable and set password for production
  master:
    persistence:
      enabled: true
      size: 1Gi
    resources:
      limits:
        cpu: 200m
        memory: 256Mi
      requests:
        cpu: 50m
        memory: 64Mi

enterprise:
  arena:
    queue:
      type: redis
      redis:
        host: "omnia-redis-master"  # Auto-generated service name
        port: 6379
```

#### Bring Your Own Redis (BYOD)

Connect to an external Redis instance (ElastiCache, Memorystore, Azure Cache, etc.):

```yaml
enterprise:
  arena:
    queue:
      type: redis
      external:
        # Option 1: Direct URL
        url: "redis://my-redis.example.com:6379"

        # Option 2: URL from secret
        secretRef:
          name: redis-credentials
          key: redis-url

        # Option 3: Password separate from URL
        password: ""  # Or use passwordSecretRef
        passwordSecretRef:
          name: redis-credentials
          key: redis-password
```

### Arena Controller

The Arena controller manages ArenaSource, ArenaJob, ArenaTemplateSource, and ArenaDevSession resources.

```yaml
enterprise:
  arena:
    controller:
      replicaCount: 1
      image:
        repository: ghcr.io/altairalabs/omnia-arena-controller
        tag: ""  # Defaults to Chart appVersion
        pullPolicy: IfNotPresent
      resources: {}
```

### Dev Console

The dev console provides interactive agent testing. Pods are created on-demand by the ArenaDevSession controller.

```yaml
enterprise:
  arena:
    devConsole:
      image:
        repository: ghcr.io/altairalabs/omnia-arena-dev-console
        tag: ""  # Defaults to Chart appVersion
        pullPolicy: IfNotPresent
```

See [ArenaDevSession CRD](/reference/arena-dev-session) for details on interactive testing.

### Community Templates

Omnia can automatically deploy a community templates source with pre-built Arena project templates.

```yaml
enterprise:
  communityTemplates:
    enabled: true  # Deploy community templates ArenaTemplateSource
    name: community-templates
    namespace: ""  # Defaults to workspace namespace
    git:
      url: https://github.com/AltairaLabs/arena-templates
      branch: main
    syncInterval: 1h
```

See [ArenaTemplateSource CRD](/reference/arena-template-source) for details on template sources.

### PromptKit LSP

The PromptKit LSP server provides real-time YAML validation and code intelligence for the Project Editor.

```yaml
enterprise:
  promptkitLsp:
    enabled: false  # Enable for Project Editor validation
    replicaCount: 2
    image:
      repository: ghcr.io/altairalabs/omnia-promptkit-lsp
      tag: ""  # Defaults to Chart appVersion
      pullPolicy: IfNotPresent
    service:
      port: 8080
    resources: {}
```

When enabled, the LSP provides:
- Real-time YAML syntax validation
- PromptKit schema validation
- Semantic token highlighting for template variables
- Diagnostic messages in the Project Editor

See the [PromptKit documentation](https://promptkit.altairalabs.ai) for details on the configuration format.

#### Redis Subchart Configuration

See the [Bitnami Redis chart documentation](https://github.com/bitnami/charts/tree/main/bitnami/redis) for all available options.

#### Redis + Grafana Integration

When both `redis.enabled` and `grafana.enabled` are true, the chart automatically:

1. **Installs the Redis datasource plugin** in Grafana
2. **Configures a Redis datasource** pointing to `omnia-redis-master:6379`
3. **Creates a Redis Overview dashboard** with:
   - Connected clients
   - Memory usage (current and over time)
   - Total keys
   - Uptime
   - Commands per second

No additional configuration is required - the integration is automatic.

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

## Demo Mode (Separate Chart)

Demo agents are now deployed via a separate `omnia-demos` chart. This provides a cleaner separation between the core operator and demo/example resources.

### Installing Demo Agents

```bash
# First, install the main Omnia operator
helm install omnia oci://ghcr.io/altairalabs/omnia \
  --namespace omnia-system \
  --create-namespace

# Then, install the demo agents
helm install omnia-demos oci://ghcr.io/altairalabs/omnia-demos \
  --namespace omnia-demo \
  --create-namespace
```

### Demo Chart Configuration

The `omnia-demos` chart has its own values. Key options include:

```yaml
# Namespace for demo resources
namespace: omnia-demo

# Ollama configuration
ollama:
  model: llava:7b  # Vision-capable model
  additionalModels:
    - llama3.2:3b  # For tools demo
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

# Vision demo agent
agent:
  name: vision-demo
  handler: runtime

# Tools demo agent (uses llama3.2 for tool calling)
toolsDemo:
  enabled: true

# Audio demo (requires Gemini API key)
audioDemo:
  enabled: false

# OPA policy validation (prevents model switching)
opa:
  enabled: false
  mode: sidecar  # or "extauthz" with Istio
```

### Requirements

- **RAM**: Minimum 8GB, 16GB recommended
- **Disk**: ~10GB for the llava:7b model
- **CPU**: 4+ cores (GPU optional but significantly faster)

Once deployed, demo agents are accessible at:
- `vision-demo.omnia-demo.svc:8080`
- `tools-demo.omnia-demo.svc:8080` (if enabled)

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

Quick-start demo with local Ollama LLM. Deploy using two separate charts:

```bash
# Main chart with dashboard
helm install omnia oci://ghcr.io/altairalabs/omnia \
  -n omnia-system --create-namespace \
  --set dashboard.enabled=true

# Demo agents chart
helm install omnia-demos oci://ghcr.io/altairalabs/omnia-demos \
  -n omnia-demo --create-namespace \
  --set ollama.persistence.enabled=true
```
