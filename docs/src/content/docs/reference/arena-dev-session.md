---
title: "ArenaDevSession CRD"
description: "Reference for the ArenaDevSession custom resource (auto-managed)"
sidebar:
  order: 15
  badge:
    text: Enterprise
    variant: tip
---

:::note[Enterprise Feature]
ArenaDevSession is an enterprise feature. The CRD is only installed when `enterprise.enabled=true` in your Helm values. See [Installing a License](/how-to/install-license/) for details.
:::

:::caution[Auto-Managed Resource]
ArenaDevSession resources are **automatically created and managed** by the Omnia dashboard when you use the "Test Agent" feature in the Project Editor. You typically do not need to create these manually.
:::

The ArenaDevSession custom resource represents an ephemeral interactive testing session for an Arena project. When created, the controller deploys a dev console pod that allows real-time chat testing with your agent configuration.

## API Version

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaDevSession
```

## Overview

ArenaDevSession provides:

- **Ephemeral dev consoles**: Per-session pods for isolated testing
- **Hot reload**: Update agent configuration without reconnecting
- **Provider integration**: Uses workspace Provider CRDs for credentials
- **Automatic cleanup**: Sessions are deleted after idle timeout

## How It Works

```
Project Editor                 ArenaDevSession              Dev Console Pod
      │                              │                            │
      │  Click "Test Agent"          │                            │
      │─────────────────────────────▶│                            │
      │                              │  Create Pod + Service      │
      │                              │───────────────────────────▶│
      │                              │                            │
      │                              │◀── Status: Ready ──────────│
      │◀──── WebSocket URL ──────────│                            │
      │                              │                            │
      │════════════ WebSocket Connection ═════════════════════════│
      │                              │                            │
      │  Idle timeout expires        │                            │
      │                              │  Delete session            │
      │                              │───────────────────────────▶│ (cleanup)
```

## Spec Fields

### `projectId`

The ID of the Arena project being tested. This corresponds to the project directory in the workspace filesystem.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `projectId` | string | Yes | Arena project identifier |

```yaml
spec:
  projectId: "my-chatbot-project"
```

### `workspace`

The workspace name for reference and labeling.

```yaml
spec:
  workspace: "my-workspace"
```

### `idleTimeout`

How long the session can be idle before automatic cleanup. Default: `30m`.

```yaml
spec:
  idleTimeout: 1h
```

### `image`

Override the default dev console image. Typically not needed.

```yaml
spec:
  image: ghcr.io/altairalabs/omnia-arena-dev-console:custom
```

### `resources`

Override the default resource requests/limits for the dev console pod.

```yaml
spec:
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 512Mi
```

## Status Fields

### `phase`

| Value | Description |
|-------|-------------|
| `Pending` | Session is waiting to be processed |
| `Starting` | Dev console pod is being created |
| `Ready` | Dev console is ready for connections |
| `Stopping` | Session is being cleaned up |
| `Stopped` | Session has been cleaned up |
| `Failed` | Session failed to start |

### `endpoint`

The WebSocket URL to connect to the dev console.

Format: `ws://arena-dev-console-{name}.{namespace}.svc:8080/ws`

### `serviceName`

The name of the Kubernetes Service created for the dev console.

### `lastActivityAt`

Timestamp of the last client activity. Used for idle timeout cleanup.

### `startedAt`

When the dev console became ready.

### `conditions`

| Type | Description |
|------|-------------|
| `Ready` | Overall readiness of the session |
| `PodReady` | Dev console pod is running |
| `ServiceReady` | Service is created |

## WebSocket Protocol

The dev console uses a WebSocket protocol for real-time communication. Connect to the endpoint from `status.endpoint`.

### Connection URL

```text
ws://arena-dev-console-{session-name}.{namespace}.svc:8080/ws
```

From the dashboard, connections are proxied through the WebSocket proxy service.

### Client Messages

#### Chat Message

Send a message to the agent:

```json
{
  "type": "chat",
  "content": "Hello, how can you help me?",
  "timestamp": "2025-01-16T10:00:00Z"
}
```

#### Chat with Attachments

Send a message with file attachments:

```json
{
  "type": "chat",
  "content": "What's in this image?",
  "parts": [
    {
      "type": "text",
      "text": "What's in this image?"
    },
    {
      "type": "image",
      "media": {
        "data": "base64-encoded-image-data",
        "mime_type": "image/jpeg"
      }
    }
  ],
  "timestamp": "2025-01-16T10:00:00Z"
}
```

#### Reload Configuration

Hot reload the agent configuration after making changes:

```json
{
  "type": "chat",
  "content": "/path/to/arena.config.yaml",
  "metadata": {
    "reload": "true"
  },
  "timestamp": "2025-01-16T10:00:00Z"
}
```

#### Reset Conversation

Clear the conversation history:

```json
{
  "type": "chat",
  "content": "",
  "metadata": {
    "reset": "true"
  },
  "timestamp": "2025-01-16T10:00:00Z"
}
```

#### Switch Provider

Change the active provider:

```json
{
  "type": "chat",
  "content": "",
  "metadata": {
    "provider": "my-openai-provider"
  },
  "timestamp": "2025-01-16T10:00:00Z"
}
```

### Server Messages

#### Connected

Sent when connection is established:

```json
{
  "type": "connected",
  "session_id": "abc123",
  "timestamp": "2025-01-16T10:00:00Z"
}
```

#### Streaming Chunk

Partial response content (streaming):

```json
{
  "type": "chunk",
  "content": "Hello! I'm here to ",
  "timestamp": "2025-01-16T10:00:00Z"
}
```

#### Message Complete

Full response with all parts:

```json
{
  "type": "done",
  "content": "Hello! I'm here to help you.",
  "parts": [
    {
      "type": "text",
      "text": "Hello! I'm here to help you."
    }
  ],
  "timestamp": "2025-01-16T10:00:00Z"
}
```

#### Tool Call

When the agent calls a tool:

```json
{
  "type": "tool_call",
  "tool_call": {
    "id": "call_123",
    "name": "get_weather",
    "arguments": "{\"city\": \"San Francisco\"}"
  },
  "timestamp": "2025-01-16T10:00:00Z"
}
```

#### Tool Result

Result of a tool execution:

```json
{
  "type": "tool_result",
  "tool_result": {
    "id": "call_123",
    "result": "{\"temperature\": 72, \"conditions\": \"sunny\"}",
    "error": null
  },
  "timestamp": "2025-01-16T10:00:00Z"
}
```

#### Reloaded

Confirmation that configuration was reloaded:

```json
{
  "type": "reloaded",
  "timestamp": "2025-01-16T10:00:00Z"
}
```

#### Error

Error message:

```json
{
  "type": "error",
  "error": {
    "message": "Provider connection failed",
    "code": "PROVIDER_ERROR"
  },
  "timestamp": "2025-01-16T10:00:00Z"
}
```

## Provider Integration

The dev console automatically resolves Provider CRDs from the workspace namespace. Provider credentials are mounted as environment variables in the dev console pod.

Supported provider types:
- OpenAI
- Anthropic
- Google (Gemini)
- Azure OpenAI
- Ollama
- Custom HTTP providers

See [Provider CRD](/reference/provider) for configuration details.

## Example Session Lifecycle

```yaml
# Created by dashboard when user clicks "Test Agent"
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaDevSession
metadata:
  name: dev-session-abc123
  namespace: workspace-ns
  labels:
    app.kubernetes.io/managed-by: omnia-dashboard
    arena.omnia.altairalabs.ai/project-id: my-chatbot
spec:
  projectId: my-chatbot
  workspace: my-workspace
  idleTimeout: 30m

status:
  phase: Ready
  endpoint: ws://arena-dev-console-dev-session-abc123.workspace-ns.svc:8080/ws
  serviceName: arena-dev-console-dev-session-abc123
  startedAt: "2025-01-16T10:00:00Z"
  lastActivityAt: "2025-01-16T10:30:00Z"
```

## Cleanup

Sessions are automatically cleaned up when:

1. **Idle timeout expires** - No WebSocket activity for the configured duration
2. **User closes the session** - Dashboard deletes the resource
3. **Workspace is deleted** - Owner references cascade deletion

## Helm Configuration

Configure the dev console image in your Helm values:

```yaml
enterprise:
  arena:
    devConsole:
      image:
        repository: ghcr.io/altairalabs/omnia-arena-dev-console
        tag: ""  # Defaults to Chart appVersion
        pullPolicy: IfNotPresent
```

## Related Resources

- **[Project Editor](/how-to/use-arena-project-editor)**: Where dev sessions are started
- **[Provider](/reference/provider)**: LLM provider configuration
- **[WebSocket Protocol](/reference/websocket-protocol)**: Base WebSocket message types
- **[PromptKit Documentation](https://promptkit.dev)**: Agent configuration format
