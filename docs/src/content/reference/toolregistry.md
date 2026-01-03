---
title: "ToolRegistry CRD"
description: "Complete reference for the ToolRegistry custom resource"
order: 3
---

# ToolRegistry CRD Reference

The ToolRegistry custom resource defines tools available to AI agents.

## API Version

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ToolRegistry
```

## Spec Fields

### `tools`

List of tool definitions.

```yaml
spec:
  tools:
    - name: search
      type: http
      url: https://api.example.com/search
      description: "Search the knowledge base"
```

### Tool Definition

Each tool can be defined inline or discovered via selector.

#### Inline Tool

| Field | Type | Required |
|-------|------|----------|
| `name` | string | Yes |
| `type` | string | Yes |
| `url` | string | Yes (for inline) |
| `description` | string | No |

```yaml
- name: calculator
  type: http
  url: https://api.example.com/calculate
  description: "Perform mathematical calculations"
```

#### Discovered Tool

| Field | Type | Required |
|-------|------|----------|
| `name` | string | Yes |
| `selector.matchLabels` | map | Yes |
| `port` | string | No |

```yaml
- name: kubernetes-tools
  selector:
    matchLabels:
      omnia.altairalabs.ai/tool: "true"
  port: http  # Use specific port name
```

### Tool Types

| Type | Description | Protocol |
|------|-------------|----------|
| `http` | HTTP REST endpoint | HTTP/HTTPS |
| `grpc` | gRPC service | gRPC |
| `mcp` | Model Context Protocol server | SSE or stdio |

### HTTP Tool Configuration

Configure HTTP-specific options:

```yaml
- name: search-api
  type: http
  description: "Search the knowledge base"
  httpConfig:
    url: https://api.example.com/search
    method: POST
    headers:
      Authorization: "Bearer ${API_KEY}"
    timeout: 30s
    retries: 3
```

### gRPC Tool Configuration

Configure gRPC-specific options:

```yaml
- name: grpc-service
  type: grpc
  description: "gRPC tool service"
  grpcConfig:
    address: tool-service.tools.svc.cluster.local:50051
    useTLS: false
    timeout: 30s
```

### MCP Tool Configuration

Configure MCP (Model Context Protocol) tools:

**SSE Transport** (connect to MCP server via Server-Sent Events):

```yaml
- name: mcp-tools
  type: mcp
  description: "MCP tool server"
  mcpConfig:
    transport: sse
    endpoint: http://mcp-server.tools.svc.cluster.local:8080/sse
```

**Stdio Transport** (spawn MCP server as subprocess):

```yaml
- name: filesystem-tools
  type: mcp
  description: "Filesystem access via MCP"
  mcpConfig:
    transport: stdio
    command: /usr/local/bin/mcp-filesystem
    args:
      - "--root=/data"
    workDir: /app
    env:
      LOG_LEVEL: info
```

## Service Discovery

Tools can be automatically discovered from Kubernetes Services.

### Service Labels

Services must have the tool label:

```yaml
metadata:
  labels:
    omnia.altairalabs.ai/tool: "true"
```

### Service Annotations

Customize discovered tool behavior:

| Annotation | Description | Default |
|------------|-------------|---------|
| `omnia.altairalabs.ai/tool-path` | API path | `/` |
| `omnia.altairalabs.ai/tool-description` | Tool description | Service name |
| `omnia.altairalabs.ai/tool-type` | Tool type | `http` |

Example Service:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: weather-api
  labels:
    omnia.altairalabs.ai/tool: "true"
  annotations:
    omnia.altairalabs.ai/tool-path: "/v1/weather"
    omnia.altairalabs.ai/tool-description: "Get weather forecasts"
spec:
  selector:
    app: weather-service
  ports:
    - name: http
      port: 80
      targetPort: 8080
```

## Status Fields

### `phase`

Current phase of the ToolRegistry.

| Value | Description |
|-------|-------------|
| `Pending` | Discovering tools |
| `Ready` | All tools available |
| `Degraded` | Some tools unavailable |
| `Failed` | No tools available |

### `discoveredTools`

Number of tools discovered.

### `availableTools`

Number of tools currently available.

### `conditions`

| Type | Description |
|------|-------------|
| `ToolsAvailable` | At least one tool is available |
| `AllToolsReady` | All configured tools are ready |

## Example

Complete ToolRegistry example with multiple tool types:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ToolRegistry
metadata:
  name: agent-tools
  namespace: agents
spec:
  tools:
    # HTTP tool for web search
    - name: web-search
      type: http
      description: "Search the web for information"
      httpConfig:
        url: https://api.search.com/query
        method: POST
        timeout: 30s

    # gRPC tool for internal service
    - name: user-service
      type: grpc
      description: "User management operations"
      grpcConfig:
        address: user-service.internal.svc.cluster.local:50051

    # MCP tool via SSE
    - name: code-tools
      type: mcp
      description: "Code analysis and manipulation"
      mcpConfig:
        transport: sse
        endpoint: http://mcp-code.tools.svc.cluster.local:8080/sse

    # Discover tools from labeled services
    - name: internal-tools
      selector:
        matchLabels:
          team: platform
          omnia.altairalabs.ai/tool: "true"
```

Status after discovery:

```yaml
status:
  phase: Ready
  discoveredTools: 5
  availableTools: 5
  conditions:
    - type: ToolsAvailable
      status: "True"
    - type: AllToolsReady
      status: "True"
```

## Multi-Port Services

For services with multiple ports, specify which port to use:

```yaml
- name: multi-port-service
  selector:
    matchLabels:
      app: my-service
  port: api  # Use the port named "api"
```
