---
title: "Adding Tools to Agents"
description: "Extend your agent's capabilities with tool integrations"
order: 2
---

# Adding Tools to Agents

This tutorial shows you how to give your agents additional capabilities using the ToolRegistry CRD.

## Overview

Tools allow agents to perform actions beyond generating text. With Omnia's ToolRegistry, you can:

- Define inline tools with direct HTTP endpoints
- Discover tools via Kubernetes label selectors
- Mix multiple tool sources in a single registry

## Step 1: Create a Simple Tool Service

First, let's deploy a simple tool service. This example provides a weather lookup tool:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: weather-tool
  namespace: default
  labels:
    omnia.altairalabs.ai/tool: "true"
  annotations:
    omnia.altairalabs.ai/tool-path: "/weather"
    omnia.altairalabs.ai/tool-description: "Get current weather for a location"
spec:
  selector:
    app: weather-service
  ports:
    - port: 80
      targetPort: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: weather-service
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: weather-service
  template:
    metadata:
      labels:
        app: weather-service
    spec:
      containers:
        - name: weather
          image: your-weather-service:latest
          ports:
            - containerPort: 8080
```

## Step 2: Create a ToolRegistry

Create a ToolRegistry that discovers tools via labels:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ToolRegistry
metadata:
  name: agent-tools
  namespace: default
spec:
  tools:
    # Inline tool definition
    - name: calculator
      type: http
      url: https://api.example.com/calculate
      description: "Perform mathematical calculations"

    # Service discovery via selector
    - name: discovered
      selector:
        matchLabels:
          omnia.altairalabs.ai/tool: "true"
```

Apply it:

```bash
kubectl apply -f toolregistry.yaml
```

## Step 3: Check Tool Discovery

Verify the tools were discovered:

```bash
kubectl get toolregistry agent-tools -o yaml
```

You should see the status showing discovered tools:

```yaml
status:
  phase: Ready
  discoveredTools: 2
  availableTools: 2
  conditions:
    - type: ToolsAvailable
      status: "True"
```

## Step 4: Connect Tools to Your Agent

Update your AgentRuntime to reference the ToolRegistry:

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
  toolRegistryRef:
    name: agent-tools
  facade:
    type: websocket
    port: 8080
```

Apply the update:

```bash
kubectl apply -f agentruntime.yaml
```

## Step 5: Test Tool Invocation

Connect to your agent and ask it to use a tool:

```bash
websocat ws://localhost:8080?agent=my-assistant
```

```json
{"type": "message", "content": "What's the weather in San Francisco?"}
```

You'll see tool call and result messages in the response stream:

```json
{"type": "tool_call", "tool_call": {"id": "tc-1", "name": "weather", "arguments": {"location": "San Francisco"}}}
{"type": "tool_result", "tool_result": {"id": "tc-1", "result": "72°F, Sunny"}}
{"type": "done", "content": "The weather in San Francisco is currently 72°F and sunny."}
```

## Tool Types

Omnia supports several tool types:

| Type | Description |
|------|-------------|
| `http` | RESTful HTTP endpoints |
| `grpc` | gRPC service endpoints |

## Next Steps

- Read the [ToolRegistry Reference](/reference/toolregistry/) for all configuration options
- Learn about [tool security](/how-to/secure-tools/) best practices
- Explore [custom tool development](/how-to/develop-tools/)
