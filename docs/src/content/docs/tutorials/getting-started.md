---
title: "Getting Started"
description: "Deploy your first AI agent with Omnia in 10 minutes"
sidebar:
  order: 1
---


This tutorial walks you through deploying your first AI agent using Omnia. By the end, you'll have a working agent accessible via WebSocket.

## Prerequisites

Before you begin, ensure you have:

- A Kubernetes cluster (kind, minikube, or a cloud provider)
- `kubectl` configured to access your cluster
- `helm` v3 installed
- An LLM provider API key (OpenAI, Anthropic, etc.) — *or use Demo Mode below*

:::tip[No API Key? Try Demo Mode]
If you don't have an API key yet, you can try Omnia with the demo charts that use Ollama (a local LLM):

```bash
# Install the Omnia operator with dashboard
helm install omnia oci://ghcr.io/altairalabs/charts/omnia \
  -n omnia-system --create-namespace \
  --set dashboard.enabled=true

# Install the demo agents (separate chart)
helm install omnia-demos oci://ghcr.io/altairalabs/charts/omnia-demos \
  -n omnia-demo --create-namespace
```

This deploys a vision-capable agent using the llava:7b model running locally. No API keys required!

**Requirements**: 8GB+ RAM, 10GB disk for the model.

Once deployed, port-forward the dashboard and open it in your browser:

```bash
kubectl port-forward -n omnia-system svc/omnia-dashboard 3000:3000
```

Visit `http://localhost:3000` and connect to the `vision-demo` agent.
:::

## Step 1: Install the Operator

Add the Omnia Helm repository and install the operator:

```bash
helm repo add altaira https://charts.altairalabs.ai
helm repo update

kubectl create namespace omnia-system
helm install omnia altaira/omnia -n omnia-system
```

Or install directly from the OCI registry:

```bash
helm install omnia oci://ghcr.io/altairalabs/charts/omnia -n omnia-system --create-namespace
```

Verify the operator is running:

```bash
kubectl get pods -n omnia-system
```

You should see the operator pod in a `Running` state.

## Step 2: Create a PromptPack

A PromptPack defines the prompts your agent will use. PromptPacks follow the [PromptPack specification](https://promptpack.org/docs/spec/schema-reference) - a structured JSON format for packaging multi-prompt conversational systems.

First, create a ConfigMap containing your compiled PromptPack JSON:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: assistant-prompts
  namespace: default
data:
  # Compiled PromptPack JSON (use `packc` to compile from YAML source)
  pack.json: |
    {
      "$schema": "https://promptpack.org/schema/latest/promptpack.schema.json",
      "id": "assistant",
      "name": "Assistant",
      "version": "1.0.0",
      "template_engine": {
        "version": "v1",
        "syntax": "{{variable}}"
      },
      "prompts": {
        "main": {
          "id": "main",
          "name": "Main Assistant",
          "version": "1.0.0",
          "system_template": "You are a helpful AI assistant. Be concise and accurate in your responses. Always be polite and professional.",
          "parameters": {
            "temperature": 0.7,
            "max_tokens": 4096
          }
        }
      }
    }
```

Then create the PromptPack resource that references the ConfigMap:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: PromptPack
metadata:
  name: assistant-pack
  namespace: default
spec:
  version: "1.0.0"
  source:
    type: configmap
    configMapRef:
      name: assistant-prompts
```

Apply both:

```bash
kubectl apply -f configmap.yaml
kubectl apply -f promptpack.yaml
```

Verify the PromptPack is ready:

```bash
kubectl get promptpack assistant-pack
```

> **Tip**: Author PromptPacks in YAML and compile them to JSON using [packc](https://promptkit.altairalabs.ai/packc/reference/) for validation and optimization:
> ```bash
> packc compile --config arena.yaml --output pack.json --id assistant
> kubectl create configmap assistant-prompts --from-file=pack.json
> ```

## Step 3: Configure the LLM Provider

Create a Secret with your LLM provider API key, then create a Provider resource:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: llm-credentials
  namespace: default
type: Opaque
stringData:
  ANTHROPIC_API_KEY: "sk-ant-..."  # Or OPENAI_API_KEY / GEMINI_API_KEY
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: my-provider
  namespace: default
spec:
  type: claude  # One of: claude, openai, gemini, ollama, mock
  model: claude-sonnet-4-20250514
  credential:
    secretRef:
      name: llm-credentials
      # key is inferred from provider type:
      #   claude  → ANTHROPIC_API_KEY
      #   openai  → OPENAI_API_KEY
      #   gemini  → GEMINI_API_KEY
```

```bash
kubectl apply -f provider.yaml
```

Verify the Provider is ready:

```bash
kubectl get provider my-provider
# Should show: my-provider   claude   claude-sonnet-4-20250514   Ready   ...
```

> **Tip**: Don't have an API key yet? Use `handler: demo` in your AgentRuntime to test with simulated responses, or set `type: mock` on the Provider for a no-network testing provider. See [Handler Modes](/reference/agentruntime/#handler-modes) for details.

## Step 4: Deploy the Agent

Now create an AgentRuntime to deploy your agent:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: my-assistant
  namespace: default
spec:
  promptPackRef:
    name: assistant-pack
  providerRef:
    name: my-provider
  facade:
    type: websocket
    port: 8080
    handler: demo  # Use "demo" for testing without an API key
  session:
    type: memory
    ttl: "1h"
```

> **Note**: Handler modes are:
> - `runtime` *(default)* — uses the runtime framework in the container for real LLM responses.
> - `demo` — simulated streaming responses for demos without an API key.
> - `echo` — echoes the input back; useful for testing connectivity.
>
> Session store types are `memory` (single-pod dev only), `redis`, and `postgres`. Redis and Postgres require a `storeRef` pointing at a Secret with connection details.

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
# Interactive mode - type messages directly
websocat "ws://localhost:8080/ws?agent=my-assistant"
```

Send a JSON message (the `?agent=` parameter is required):

```json
{"type": "message", "content": "Hello, who are you?"}
```

You should see responses like:

```json
{"type":"connected","session_id":"abc123...","timestamp":"..."}
{"type":"chunk","session_id":"abc123...","content":"Hello","timestamp":"..."}
{"type":"chunk","session_id":"abc123...","content":"!","timestamp":"..."}
{"type":"done","session_id":"abc123...","content":"","timestamp":"..."}
```

> **Tip**: To send a single test message programmatically:
> ```bash
> echo '{"type":"message","content":"Hello!"}' | websocat "ws://localhost:8080/ws?agent=my-assistant"
> ```

## Next Steps

- Learn about [Provider configuration](/reference/provider/) for LLM settings
- Explore [ToolRegistry](/tutorials/adding-tools/) to give your agent capabilities
- Read about [session management](/explanation/sessions/) for stateful conversations
- Set up [observability](/how-to/setup-observability/) for monitoring
- Configure [autoscaling](/how-to/scale-agents/) for production workloads

Congratulations! You've deployed your first AI agent with Omnia.
