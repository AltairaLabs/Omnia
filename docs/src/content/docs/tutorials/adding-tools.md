---
title: "Adding Tools to Agents"
description: "Extend your agent's capabilities with tool integrations"
sidebar:
  order: 2
---


This tutorial shows you how to give your agents additional capabilities using the ToolRegistry CRD.

## Overview

Tools allow agents to perform actions beyond generating text. With Omnia's ToolRegistry, you can:

- Define HTTP and gRPC tools with explicit schemas
- Connect to self-describing MCP servers
- Integrate with OpenAPI-documented services
- Mix multiple handler types in a single registry

## Handler Types

Omnia supports four types of tool handlers:

| Type | Category | Description |
|------|----------|-------------|
| `http` | Explicit | HTTP REST endpoints with defined schema |
| `grpc` | Explicit | gRPC services using Tool protocol |
| `mcp` | Self-describing | Model Context Protocol servers |
| `openapi` | Self-describing | OpenAPI/Swagger services |

**Self-describing** handlers (MCP, OpenAPI) automatically discover available tools at runtime. **Explicit** handlers (HTTP, gRPC) require you to define the tool name, description, and input schema.

## Step 1: Create a Tool Service

First, deploy a simple tool service. This example provides a calculator tool:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: calculator-service
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: calculator
  template:
    metadata:
      labels:
        app: calculator
    spec:
      containers:
        - name: calculator
          image: your-calculator-service:latest
          ports:
            - containerPort: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: calculator
  namespace: default
spec:
  selector:
    app: calculator
  ports:
    - port: 80
      targetPort: 8080
```

## Step 2: Create a ToolRegistry

Create a ToolRegistry with an HTTP handler pointing to your service:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ToolRegistry
metadata:
  name: agent-tools
  namespace: default
spec:
  handlers:
    - name: calculator
      type: http
      endpoint:
        serviceRef:
          name: calculator
          port: 80
      tool:
        name: calculate
        description: "Perform mathematical calculations"
        inputSchema:
          type: object
          properties:
            expression:
              type: string
              description: "Mathematical expression to evaluate"
          required: [expression]
      httpConfig:
        method: POST
        contentType: application/json
      timeout: "10s"
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
  discoveredToolsCount: 1
  availableToolsCount: 1
  discoveredTools:
    - handlerName: calculator
      name: calculate
      status: Available
  conditions:
    - type: HandlersAvailable
      status: "True"
    - type: AllHandlersReady
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
  promptPackRef:
    name: assistant-pack
  toolRegistryRef:
    name: agent-tools
  facade:
    type: websocket
    port: 8080
  session:
    type: memory
    ttl: "1h"
  runtime:
    replicas: 1
  provider:
    secretRef:
      name: llm-credentials
```

Apply the update:

```bash
kubectl apply -f agentruntime.yaml
```

## Step 5: Test Tool Invocation

Connect to your agent and ask it to use a tool:

```bash
websocat ws://localhost:8080/ws?agent=my-assistant
```

```json
{"type": "message", "content": "What is 25 * 4?"}
```

You'll see tool call and result messages in the response stream:

```json
{"type": "connected", "session_id": "abc123"}
{"type": "tool_call", "tool_call": {"id": "tc-1", "name": "calculate", "arguments": {"expression": "25 * 4"}}}
{"type": "tool_result", "tool_result": {"id": "tc-1", "result": {"answer": 100}}}
{"type": "chunk", "content": "25 multiplied by 4 equals 100."}
{"type": "done", "content": "25 multiplied by 4 equals 100."}
```

## Adding Self-Describing Tools

### MCP Server

Connect to an MCP server that automatically exposes its tools:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ToolRegistry
metadata:
  name: mcp-tools
spec:
  handlers:
    - name: filesystem
      type: mcp
      mcpConfig:
        transport: sse
        endpoint: http://mcp-filesystem.tools.svc.cluster.local:8080/sse
```

The MCP server announces its tools, and Omnia automatically makes them available to agents.

### OpenAPI Service

Connect to any service with an OpenAPI specification:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ToolRegistry
metadata:
  name: api-tools
spec:
  handlers:
    - name: petstore
      type: openapi
      openAPIConfig:
        specURL: https://petstore.swagger.io/v2/swagger.json
        operationFilter:
          - getPetById
          - findPetsByStatus
```

Each OpenAPI operation becomes a tool. Use `operationFilter` to limit which operations are exposed.

## Combining Multiple Handlers

A single ToolRegistry can contain multiple handlers of different types:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ToolRegistry
metadata:
  name: all-tools
spec:
  handlers:
    # Explicit HTTP tool
    - name: search
      type: http
      endpoint:
        url: https://api.search.com/query
      tool:
        name: web_search
        description: "Search the web"
        inputSchema:
          type: object
          properties:
            query:
              type: string
          required: [query]
      httpConfig:
        method: POST

    # Self-describing MCP server
    - name: code-assistant
      type: mcp
      mcpConfig:
        transport: sse
        endpoint: http://mcp-code.tools.svc.cluster.local/sse

    # Self-describing OpenAPI service
    - name: weather-api
      type: openapi
      openAPIConfig:
        specURL: https://api.weather.com/openapi.yaml
```

## Tool Discovery via Labels

You can also discover tool services via Kubernetes labels:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ToolRegistry
metadata:
  name: discovered-tools
spec:
  handlers:
    - name: platform-tools
      selector:
        matchLabels:
          omnia.altairalabs.ai/tool: "true"
          team: platform
```

Services matching the selector are automatically added. Annotate your services to customize behavior:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-tool-service
  labels:
    omnia.altairalabs.ai/tool: "true"
    team: platform
  annotations:
    omnia.altairalabs.ai/tool-path: "/api/action"
    omnia.altairalabs.ai/tool-description: "Perform custom action"
spec:
  selector:
    app: my-tool
  ports:
    - port: 80
```

## Next Steps

- Read the [ToolRegistry Reference](/reference/toolregistry/) for all configuration options
- Learn about [configuring authentication](/how-to/configure-authentication/) for tool access
- Explore [observability](/how-to/setup-observability/) to monitor tool calls
