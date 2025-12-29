---
title: "Getting Started"
description: "Deploy your first AI agent with Omnia in 10 minutes"
order: 1
---

# Getting Started with Omnia

This tutorial walks you through deploying your first AI agent using Omnia. By the end, you'll have a working agent accessible via WebSocket.

## Prerequisites

Before you begin, ensure you have:

- A Kubernetes cluster (kind, minikube, or a cloud provider)
- `kubectl` configured to access your cluster
- `helm` v3 installed
- An LLM provider API key (OpenAI, Anthropic, etc.)

## Step 1: Install the Operator

Add the Omnia Helm repository and install the operator:

```bash
# Add the Helm repository
helm repo add omnia https://altairalabs.github.io/omnia/charts
helm repo update

# Create namespace and install
kubectl create namespace omnia-system
helm install omnia omnia/omnia -n omnia-system
```

Verify the operator is running:

```bash
kubectl get pods -n omnia-system
```

You should see the operator pod in a `Running` state.

## Step 2: Create a PromptPack

A PromptPack defines the prompts your agent will use. First, create a ConfigMap with your prompt content:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: assistant-prompts
  namespace: default
data:
  system.txt: |
    You are a helpful AI assistant. Be concise and accurate in your responses.
  greeting.txt: |
    Hello! I'm your AI assistant. How can I help you today?
```

Then create the PromptPack resource:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: PromptPack
metadata:
  name: assistant-pack
  namespace: default
spec:
  source:
    configMapRef:
      name: assistant-prompts
```

Apply both:

```bash
kubectl apply -f configmap.yaml
kubectl apply -f promptpack.yaml
```

## Step 3: Configure Provider Credentials

Create a Secret with your LLM provider API key:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: llm-credentials
  namespace: default
type: Opaque
stringData:
  api-key: "your-api-key-here"
```

```bash
kubectl apply -f secret.yaml
```

## Step 4: Deploy the Agent

Now create an AgentRuntime to deploy your agent:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: my-assistant
  namespace: default
spec:
  replicas: 1
  provider:
    name: openai
    model: gpt-4
    apiKeySecretRef:
      name: llm-credentials
      key: api-key
  promptPackRef:
    name: assistant-pack
  facade:
    type: websocket
    port: 8080
```

```bash
kubectl apply -f agentruntime.yaml
```

## Step 5: Verify the Deployment

Check that all resources are ready:

```bash
# Check the AgentRuntime status
kubectl get agentruntime my-assistant

# Check the pods
kubectl get pods -l app.kubernetes.io/instance=my-assistant

# Check the service
kubectl get svc my-assistant
```

## Step 6: Connect to the Agent

Port-forward to access the agent:

```bash
kubectl port-forward svc/my-assistant 8080:8080
```

Now you can connect using any WebSocket client. Using `websocat`:

```bash
websocat ws://localhost:8080?agent=my-assistant
```

Send a message:

```json
{"type": "message", "content": "Hello, who are you?"}
```

You'll receive a response with the agent's reply.

## Next Steps

- Learn about [PromptPack configuration](/how-to/configure-promptpacks/)
- Explore [ToolRegistry](/tutorials/adding-tools/) to give your agent capabilities
- Read about [session management](/explanation/sessions/) for stateful conversations

Congratulations! You've deployed your first AI agent with Omnia.
