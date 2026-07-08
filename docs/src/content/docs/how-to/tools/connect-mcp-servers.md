---
title: "Connect MCP servers"
description: "Attach Model Context Protocol servers as self-describing tool sources, filter their tools, and configure transports and retries"
sidebar:
  order: 4
---

An `mcp` handler connects a [Model Context Protocol](https://modelcontextprotocol.io)
server as a **self-describing** tool source — the server announces its tools and
the runtime discovers them at start-up, so you do not write a `tool` definition.

The dashboard UI supports basic `sse`/`stdio` MCP handlers. The `streamable-http`
transport, tool filtering, and retry policy are CRD-only.

## Transports

| Transport | Use for | Endpoint field |
|-----------|---------|----------------|
| `sse` | Remote MCP server over Server-Sent Events | `endpoint` (SSE URL) |
| `streamable-http` | Remote MCP server over Streamable HTTP | `endpoint` (HTTP URL) |
| `stdio` | MCP server spawned as a subprocess | `command` (+ `args`) |

### SSE

```yaml
- name: mcp-server
  type: mcp
  mcpConfig:
    transport: sse
    endpoint: http://mcp-server.tools.svc.cluster.local:8080/sse
```

### Streamable HTTP

```yaml
- name: mcp-http
  type: mcp
  mcpConfig:
    transport: streamable-http
    endpoint: http://mcp-server.tools.svc.cluster.local:8080/mcp
```

### Stdio (subprocess)

```yaml
- name: filesystem
  type: mcp
  mcpConfig:
    transport: stdio
    command: /usr/local/bin/mcp-filesystem
    args: ["--root=/data"]
    workDir: /app
    env:
      LOG_LEVEL: info
```

## Filter which tools are exposed

An MCP server may expose more tools than you want an agent to see. `toolFilter`
narrows the set:

```yaml
- name: github
  type: mcp
  mcpConfig:
    transport: sse
    endpoint: http://mcp-github.tools.svc.cluster.local:8080/sse
    toolFilter:
      allowlist: [search_issues, get_pull_request]   # only these are exposed
      # blocklist: [delete_repo]                      # or exclude specific tools
```

- `allowlist` — expose only these tool names. If empty, all tools are allowed.
- `blocklist` — exclude these tool names.

## Retry CallTool failures

```yaml
mcpConfig:
  transport: sse
  endpoint: http://mcp-server.tools.svc.cluster.local:8080/sse
  retryPolicy:
    maxAttempts: 3          # 1–10; 1 = no retries
    initialBackoff: "100ms"
    backoffMultiplier: "2.0"
    maxBackoff: "30s"
```

Transport-level reconnect on a broken session is handled separately by the MCP
client and is not governed by this policy — `retryPolicy` covers `CallTool`
failures.

## Authentication

The [`auth` stanza](/how-to/tools/authenticate-tools/) works on `sse` and
`streamable-http` transports (the credential is attached as a transport header).
Auth on a `stdio` transport is **rejected** — a subprocess has no header channel.

```yaml
- name: secured-mcp
  type: mcp
  mcpConfig:
    transport: sse
    endpoint: https://mcp.example.com/sse
  auth:
    type: bearer
    secretRef:
      name: mcp-credentials
      key: token
```

## Status

At reconcile time an `mcp` handler contributes a single placeholder
`discoveredTools` entry named after the handler; the individual tools it exposes
are discovered by the runtime when the agent starts. As with all handlers,
`Available` reflects a valid configuration, not a live reachability probe.

## See also

- [ToolRegistry CRD reference](/reference/core/toolregistry/)
- [Authenticate tools](/how-to/tools/authenticate-tools/)
