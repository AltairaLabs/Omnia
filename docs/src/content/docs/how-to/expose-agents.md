---
title: "Expose Agents"
description: "Expose agents externally using Kubernetes Gateway API"
sidebar:
  order: 6
---


Omnia uses the Kubernetes Gateway API to expose agents externally. This provides a standard, portable way to manage ingress traffic with support for WebSocket connections.

## Prerequisites

- Kubernetes cluster with Gateway API CRDs installed
- A Gateway controller (Istio, Envoy Gateway, etc.)

### Install Gateway API CRDs

```bash
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.0/standard-install.yaml
```

### Install Istio (recommended)

Istio provides a production-ready Gateway controller:

```bash
helm repo add istio https://istio-release.storage.googleapis.com/charts
helm repo update

helm install istio-base istio/base -n istio-system --create-namespace
helm install istiod istio/istiod -n istio-system --wait
```

## Enable the Gateway

Configure the gateway in your Helm values:

```yaml
gateway:
  enabled: true
  name: agents
  className: istio
  listeners:
    http:
      port: 80
      protocol: HTTP
```

## Create an HTTPRoute for Your Agent

After deploying an AgentRuntime, create an HTTPRoute to expose it:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-agent
  namespace: default
spec:
  parentRefs:
    - name: omnia-agents
      namespace: omnia-system
  hostnames:
    - "agents.example.com"
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /my-agent
      backendRefs:
        - name: my-agent
          port: 8080
```

## Finding Your Agent's URL

After creating an HTTPRoute, the resulting external URL is published to the agent's status:

### View URLs via kubectl

```bash
kubectl get agentruntime <name> -o jsonpath='{.status.facade.endpoints}'
```

This displays all discovered external endpoints derived from HTTPRoutes targeting the agent's facade Service.

### View URLs in the Dashboard

The agent's detail page displays the external URL in the **Connect** card (under the "External" tab). This mirrors the same `status.facade.endpoints` data.

### URL Validity

If an endpoint shows `valid: false`, the URL is advertised but will not connect. This typically occurs with path-prefix routes that lack a `URLRewrite` filter:

```yaml
# This route will be marked valid: false
rules:
  - matches:
      - path:
          type: PathPrefix
          value: /my-agent
    backendRefs:
      - name: my-agent
        port: 8080
```

**Solution**: Either use host-based routing (recommended), or add a `URLRewrite` filter with `ReplacePrefixMatch`:

```yaml
rules:
  - matches:
      - path:
          type: PathPrefix
          value: /my-agent
    filters:
      - type: URLRewrite
        urlRewrite:
          replacePrefixMatch: /
    backendRefs:
      - name: my-agent
        port: 8080
```

### Authentication

External authentication is governed by the agent's `spec.externalAuth` setting (sharedToken, apiKeys, OIDC, or edge trust), not the dashboard's management-plane token. Configure external auth on the AgentRuntime to control how clients authenticate to the external endpoint.

## Access Your Agent

### Get the Gateway IP

```bash
kubectl get gateway omnia-agents -n omnia-system \
  -o jsonpath='{.status.addresses[0].value}'
```

### Connect via WebSocket

```bash
websocat ws://<gateway-ip>/my-agent/ws

wscat -c ws://<gateway-ip>/my-agent/ws
```

## Enable HTTPS

For production, enable TLS termination:

### Create a TLS Secret

```bash
kubectl create secret tls agents-tls \
  --cert=path/to/cert.pem \
  --key=path/to/key.pem \
  -n omnia-system
```

### Configure HTTPS Listener

```yaml
gateway:
  enabled: true
  listeners:
    http:
      port: 80
      protocol: HTTP
    https:
      enabled: true
      port: 443
      protocol: HTTPS
      tlsSecretName: agents-tls
```

### Update HTTPRoute for HTTPS

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-agent
spec:
  parentRefs:
    - name: omnia-agents
      namespace: omnia-system
      sectionName: https  # Use the HTTPS listener
  hostnames:
    - "agents.example.com"
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /my-agent
      backendRefs:
        - name: my-agent
          port: 8080
```

## Multiple Agents

Expose multiple agents through the same gateway:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: all-agents
spec:
  parentRefs:
    - name: omnia-agents
      namespace: omnia-system
  hostnames:
    - "agents.example.com"
  rules:
    # Customer service agent
    - matches:
        - path:
            type: PathPrefix
            value: /customer-service
      backendRefs:
        - name: customer-service-agent
          port: 8080
    # Sales agent
    - matches:
        - path:
            type: PathPrefix
            value: /sales
      backendRefs:
        - name: sales-agent
          port: 8080
    # Support agent
    - matches:
        - path:
            type: PathPrefix
            value: /support
      backendRefs:
        - name: support-agent
          port: 8080
```

## Host-Based Routing

Route to different agents based on hostname:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: customer-agent
spec:
  parentRefs:
    - name: omnia-agents
      namespace: omnia-system
  hostnames:
    - "customer.agents.example.com"
  rules:
    - backendRefs:
        - name: customer-service-agent
          port: 8080
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: sales-agent
spec:
  parentRefs:
    - name: omnia-agents
      namespace: omnia-system
  hostnames:
    - "sales.agents.example.com"
  rules:
    - backendRefs:
        - name: sales-agent
          port: 8080
```

## Internal Gateway

Omnia also creates an internal gateway for observability tools:

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

Access internal tools:

```bash
# Get internal gateway IP
kubectl get gateway omnia-internal -n omnia-system \
  -o jsonpath='{.status.addresses[0].value}'

# Access Grafana
curl http://<internal-ip>:8080/grafana/

# Access Prometheus
curl http://<internal-ip>:8080/prometheus/
```

## Troubleshooting

### Check Gateway Status

```bash
kubectl get gateway -n omnia-system
kubectl describe gateway omnia-agents -n omnia-system
```

### Check HTTPRoute Status

```bash
kubectl get httproute
kubectl describe httproute my-agent
```

### Verify Route is Attached

The HTTPRoute status should show it's accepted:

```bash
kubectl get httproute my-agent -o jsonpath='{.status.parents[0].conditions}'
```

### Check Istio Proxy Logs

```bash
kubectl logs -l istio=ingressgateway -n istio-system
```

## Without Istio

If using a different Gateway controller (e.g., Envoy Gateway, Contour):

```yaml
gateway:
  enabled: true
  className: envoy  # or your controller's class name
```

Ensure your controller supports WebSocket connections for agent communication.
