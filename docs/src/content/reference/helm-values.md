---
title: "Helm Values Reference"
description: "Complete reference for Omnia Helm chart configuration"
order: 5
---

# Helm Values Reference

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
# Number of operator replicas
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

## Agent Configuration

```yaml
agent:
  image:
    repository: ghcr.io/altairalabs/omnia-agent
    tag: ""  # Defaults to Chart appVersion
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
```

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

Advanced autoscaling with scale-to-zero support:

```yaml
keda:
  enabled: false
  operator:
    watchNamespace: ""  # Empty = all namespaces
  prometheus:
    serverAddress: "http://omnia-prometheus-server.omnia-system.svc.cluster.local"
```

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
