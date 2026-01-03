---
title: "Local Development Setup"
description: "Set up a local development environment for Omnia"
sidebar:
  order: 1
---


This guide walks you through setting up a local development environment for testing Omnia.

## Prerequisites

Install the required tools:

- **Go 1.25+**: [Download Go](https://golang.org/dl/)
- **Docker**: [Install Docker](https://docs.docker.com/get-docker/)
- **kubectl**: [Install kubectl](https://kubernetes.io/docs/tasks/tools/)
- **kind**: [Install kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- **Helm**: [Install Helm](https://helm.sh/docs/intro/install/)

## Create a Local Cluster

Create a kind cluster with port forwarding:

```bash
cat <<EOF | kind create cluster --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 30080
    hostPort: 8080
    protocol: TCP
EOF
```

Verify the cluster is running:

```bash
kubectl cluster-info
```

## Deploy Redis (Optional)

If you need session persistence, deploy Redis:

```bash
kubectl create namespace redis
helm repo add bitnami https://charts.bitnami.com/bitnami
helm install redis bitnami/redis -n redis \
  --set auth.enabled=false \
  --set architecture=standalone
```

## Build and Load Images

Build the operator and agent images:

```bash
make docker-build IMG=omnia-operator:dev

docker build -t omnia-agent:dev -f Dockerfile.agent .

kind load docker-image omnia-operator:dev
kind load docker-image omnia-agent:dev
```

## Install the Operator

Deploy using Helm with local images:

```bash
helm install omnia charts/omnia -n omnia-system --create-namespace \
  --set image.repository=omnia-operator \
  --set image.tag=dev \
  --set image.pullPolicy=Never
```

## Verify Installation

Check the operator is running:

```bash
kubectl get pods -n omnia-system
kubectl logs -n omnia-system -l app.kubernetes.io/name=omnia -f
```

## Deploy Test Resources

Apply sample manifests:

```bash
kubectl apply -f config/samples/
```

## Using Demo Mode for Testing

For local development without LLM costs, use the `demo` or `echo` handler modes:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: test-agent
spec:
  promptPackRef:
    name: test-prompts
  facade:
    type: websocket
    handler: demo  # Use 'echo' for simple connectivity testing
  session:
    type: memory
```

The demo handler provides:
- Streaming responses that simulate real LLM output
- Simulated tool calls for password and weather queries
- No API key required

This is useful for:
- UI/frontend development
- Integration testing
- Demos and screenshots
- Validating WebSocket connectivity

## Connect to an Agent

Forward the agent port:

```bash
kubectl port-forward svc/sample-agent 8080:8080
```

Test with websocat:

```bash
websocat ws://localhost:8080?agent=sample-agent
```

## Troubleshooting

### Operator not starting

Check logs:

```bash
kubectl logs -n omnia-system deployment/omnia-operator
```

### Agent pods failing

Check events:

```bash
kubectl describe agentruntime <name>
kubectl describe pod -l app.kubernetes.io/instance=<name>
```

### WebSocket connection refused

Ensure the service is ready:

```bash
kubectl get endpoints <agent-name>
```
