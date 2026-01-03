---
title: "Production Setup"
description: "Deploy Omnia for production with Istio, Gateway API, and observability"
sidebar:
  order: 2
---


This tutorial walks through deploying Omnia for production use with:

- **Istio service mesh** for traffic management and mTLS
- **Gateway API** for external agent access
- **Observability stack** (Prometheus, Grafana, Loki, Tempo)
- **JWT authentication** for secure agent access

## Prerequisites

- Kubernetes cluster (1.28+)
- `kubectl`, `helm`, and `istioctl` installed
- Domain name with DNS configured (for external access)

## Step 1: Install Istio

Install Istio with the Gateway API integration:

```bash
curl -L https://istio.io/downloadIstio | sh -
cd istio-*
export PATH=$PWD/bin:$PATH

istioctl install --set profile=default -y

kubectl label namespace default istio-injection=enabled
```

Verify Istio is running:

```bash
kubectl get pods -n istio-system
```

## Step 2: Install Omnia with Full Stack

Create a values file for production:

```yaml
replicaCount: 2

prometheus:
  enabled: true
  server:
    persistentVolume:
      enabled: true
      size: 50Gi

grafana:
  enabled: true
  adminPassword: "your-secure-password"  # Change this!
  persistence:
    enabled: true
    size: 10Gi

loki:
  enabled: true
  singleBinary:
    persistence:
      enabled: true
      size: 50Gi

alloy:
  enabled: true

tempo:
  enabled: true
  persistence:
    enabled: true
    size: 50Gi

istio:
  enabled: true

gateway:
  enabled: true
  className: istio
  listeners:
    https:
      enabled: true
      tlsSecretName: omnia-tls  # Create this secret with your TLS cert

internalGateway:
  enabled: true
  className: istio

keda:
  enabled: true
```

Install Omnia:

```bash
kubectl create namespace omnia-system

helm install omnia omnia/omnia \
  -n omnia-system \
  -f production-values.yaml
```

## Step 3: Configure TLS Certificate

Create a TLS secret for HTTPS:

```bash
kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: omnia-tls
  namespace: omnia-system
spec:
  secretName: omnia-tls
  issuerRef:
    name: letsencrypt-prod
    kind: ClusterIssuer
  dnsNames:
    - agents.yourdomain.com
EOF

kubectl create secret tls omnia-tls \
  -n omnia-system \
  --cert=path/to/tls.crt \
  --key=path/to/tls.key
```

## Step 4: Configure JWT Authentication

Set up JWT authentication for agent access:

```yaml
authentication:
  enabled: true
  jwt:
    issuer: "https://your-identity-provider.com"
    audiences:
      - "omnia-agents"
```

See [Configure Authentication](/how-to/configure-authentication) for detailed provider configuration.

## Step 5: Deploy a Production Agent

First, create a Provider for your LLM:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: llm-credentials
  namespace: default
stringData:
  ANTHROPIC_API_KEY: "sk-ant-..."
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: claude-production
  namespace: default
spec:
  type: claude
  model: claude-sonnet-4-20250514
  secretRef:
    name: llm-credentials
  defaults:
    temperature: "0.7"
    maxTokens: 4096
```

Then create an AgentRuntime with production settings:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: production-agent
  namespace: default
spec:
  promptPackRef:
    name: my-promptpack
  providerRef:
    name: claude-production
  facade:
    type: websocket
    port: 8080
    handler: runtime  # Production mode
  session:
    type: redis
    storeRef:
      name: redis-credentials
    ttl: "24h"
  runtime:
    replicas: 2
    resources:
      requests:
        cpu: "500m"
        memory: "256Mi"
      limits:
        cpu: "1000m"
        memory: "512Mi"
    autoscaling:
      enabled: true
      type: keda
      minReplicas: 1
      maxReplicas: 10
      keda:
        pollingInterval: 15
        cooldownPeriod: 300
        triggers:
          - type: prometheus
            metadata:
              serverAddress: "http://omnia-prometheus-server.omnia-system.svc"
              query: 'sum(omnia_facade_connections_active{agent="production-agent"})'
              threshold: "50"
```

Create an HTTPRoute to expose the agent:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: production-agent-route
  namespace: default
spec:
  parentRefs:
    - name: omnia-agents
      namespace: omnia-system
  hostnames:
    - "agents.yourdomain.com"
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /production-agent
      backendRefs:
        - name: production-agent
          port: 8080
```

## Step 6: Access Observability Tools

The internal gateway exposes Grafana and Prometheus. Port-forward to access:

```bash
# Get the internal gateway IP
kubectl get gateway omnia-internal -n omnia-system

# Or port-forward directly
kubectl port-forward svc/omnia-grafana 3000:80 -n omnia-system
```

Access Grafana at `http://localhost:3000` with:
- Username: `admin`
- Password: (from your values file)

Pre-configured dashboards include:
- **Omnia Overview**: Request rates, latency, and errors
- **Agent Metrics**: Per-agent performance and scaling
- **Cost Tracking**: Token usage and estimated costs

## Step 7: Connect to Your Agent

With the external gateway configured, connect via WebSocket:

```bash
websocat "wss://agents.yourdomain.com/production-agent/ws?agent=production-agent" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

Or using the internal service (from within the cluster):

```bash
websocat "ws://production-agent.default.svc:8080/ws?agent=production-agent"
```

## Verification Checklist

- [ ] Istio pods running in `istio-system`
- [ ] Omnia operator running in `omnia-system`
- [ ] Prometheus scraping metrics
- [ ] Grafana dashboards loading
- [ ] Gateway receiving external traffic
- [ ] Agent pods running with Istio sidecar
- [ ] WebSocket connections working through Gateway

## Next Steps

- [Set Up Observability](/how-to/setup-observability) - Configure dashboards and alerts
- [Configure Authentication](/how-to/configure-authentication) - Detailed JWT setup
- [Scale Agent Deployments](/how-to/scale-agents) - KEDA and HPA configuration
- [Autoscaling Explained](/explanation/autoscaling) - Understanding scaling strategies
