---
title: "Client-side (browser) tools"
description: "Define tools that execute in the connected browser over the WebSocket facade, with optional user consent"
sidebar:
  order: 3
---

A `client` handler defines a tool that is executed by the **connected browser
client**, not by the runtime. The runtime emits a `tool_call` over the WebSocket
facade; the browser runs the tool and returns the result before the conversation
continues. This is CRD-only — the dashboard UI does not create `client` handlers.

Client tools are the **only** tools visible in the WebSocket message stream.
Server-side tools (HTTP, gRPC, MCP, OpenAPI, and platform tools) execute in the
runtime and never appear as `tool_call` messages to the browser.

## When to use a client tool

- Reading something only the browser has: geolocation, camera, clipboard, local files
- Driving the page: navigation, filling a form, showing a UI element
- Anything that must run on the user's device rather than server-side

## Define a client handler

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ToolRegistry
metadata:
  name: browser-tools
  namespace: agents
spec:
  handlers:
    - name: geolocation
      type: client
      clientConfig:
        consentMessage: "Allow the assistant to read your current location?"
        categories: [location]
      tool:
        name: get_location
        description: "Read the user's current location from the browser"
        inputSchema:
          type: object
```

Like `http` and `grpc` handlers, a `client` handler **requires** a `tool`
definition (`name`, `description`, `inputSchema`). It does not take an endpoint —
the resolved endpoint is `client://browser`.

## Consent

`clientConfig` is optional and controls the consent prompt the client shows
before executing the tool:

| Field | Description |
|-------|-------------|
| `consentMessage` | Human-readable prompt shown before the tool runs. If empty, the tool runs without a consent prompt. |
| `categories` | Semantic consent categories (e.g. `location`, `camera`). Clients can remember a user's consent decision per category. |

The consent UX itself is implemented by the client; the categories let a client
remember "the user already allowed `location`" across tools and sessions.

## Wire it to an agent

Reference the ToolRegistry from the AgentRuntime as usual:

```yaml
spec:
  toolRegistryRef:
    name: browser-tools
  facades:
    - type: websocket
```

Client tools require a `websocket` facade — that is the channel used to forward
the `tool_call` to the browser and receive the result.

## See also

- [ToolRegistry CRD reference](/reference/core/toolregistry/)
- [Connect MCP servers](/how-to/tools/connect-mcp-servers/)
