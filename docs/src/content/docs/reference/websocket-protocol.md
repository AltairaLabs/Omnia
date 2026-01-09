---
title: "WebSocket Protocol"
description: "WebSocket message protocol reference"
sidebar:
  order: 4
---


This document describes the WebSocket protocol used by Omnia agent facades.

## Connection

### URL Format

```text
ws://host:port?agent=<agent-name>&namespace=<namespace>
```

| Parameter | Required | Description |
|-----------|----------|-------------|
| `agent` | Yes | Name of the AgentRuntime |
| `namespace` | No | Namespace (defaults to `default`) |

### Example

```bash
websocat "ws://localhost:8080?agent=my-agent&namespace=production"
```

## Message Types

### Client Messages

Messages sent from client to server.

#### Message

Send a user message to the agent:

```json
{
  "type": "message",
  "content": "Hello, how are you?",
  "session_id": "optional-session-id",
  "metadata": {
    "user_id": "user-123"
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | Must be `"message"` |
| `content` | string | No | User message content (text-only) |
| `parts` | array | No | Multi-modal content parts (see below) |
| `session_id` | string | No | Resume existing session |
| `metadata` | object | No | Custom metadata |

> **Note**: Either `content` or `parts` should be provided. If both are present, `parts` takes precedence.

#### Multi-Modal Message

Send a message with images or other media:

```json
{
  "type": "message",
  "session_id": "sess-abc123",
  "parts": [
    {
      "type": "text",
      "text": "What's in this image?"
    },
    {
      "type": "image",
      "media": {
        "url": "https://example.com/photo.jpg",
        "mime_type": "image/jpeg"
      }
    }
  ]
}
```

##### ContentPart Types

| Type | Description |
|------|-------------|
| `text` | Plain text content |
| `image` | Image (JPEG, PNG, GIF, WebP) |
| `audio` | Audio file (MP3, WAV, OGG) |
| `video` | Video file (MP4, WebM) |
| `file` | Generic file attachment |

##### ContentPart Structure

```typescript
interface ContentPart {
  type: "text" | "image" | "audio" | "video" | "file"
  text?: string        // For type: "text"
  media?: MediaContent // For media types
}

interface MediaContent {
  // Data source (exactly one required)
  data?: string        // Base64-encoded (< 256KB recommended)
  url?: string         // HTTP/HTTPS URL
  storage_ref?: string // Backend storage reference

  // Required
  mime_type: string    // e.g., "image/jpeg", "audio/mp3"

  // Optional metadata
  filename?: string
  size_bytes?: number

  // Image-specific
  width?: number
  height?: number
  detail?: "low" | "high" | "auto"  // Vision model hint

  // Audio/Video-specific
  duration_ms?: number
  sample_rate?: number  // Audio: Hz
  channels?: number     // Audio: 1=mono, 2=stereo
}
```

##### Example: Image with Base64 Data

```json
{
  "type": "message",
  "parts": [
    { "type": "text", "text": "Describe this image" },
    {
      "type": "image",
      "media": {
        "data": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR...",
        "mime_type": "image/png"
      }
    }
  ]
}
```

### Server Messages

Messages sent from server to client.

#### Connected

Sent immediately after connection:

```json
{
  "type": "connected",
  "session_id": "sess-abc123"
}
```

#### Chunk

Streaming response chunk:

```json
{
  "type": "chunk",
  "content": "Hello! I'm doing"
}
```

#### Done

Final response completion:

```json
{
  "type": "done",
  "content": "Hello! I'm doing great, thank you for asking!"
}
```

#### Multi-Modal Response

For responses containing media (e.g., generated images), the server uses the `parts` array:

```json
{
  "type": "done",
  "session_id": "sess-abc123",
  "parts": [
    {
      "type": "text",
      "text": "Here's the image you requested:"
    },
    {
      "type": "image",
      "media": {
        "url": "https://storage.example.com/generated/img-123.png",
        "mime_type": "image/png",
        "width": 1024,
        "height": 1024
      }
    }
  ]
}
```

> **Note**: When `parts` is present, it takes precedence over `content`. For backward compatibility, text-only responses may use either format.

#### Tool Call

Agent is calling a tool:

```json
{
  "type": "tool_call",
  "tool_call": {
    "id": "tc-123",
    "name": "weather",
    "arguments": {
      "location": "San Francisco"
    }
  }
}
```

#### Tool Result

Result from a tool call:

```json
{
  "type": "tool_result",
  "tool_result": {
    "id": "tc-123",
    "result": "72°F, Sunny"
  }
}
```

#### Error

Error message:

```json
{
  "type": "error",
  "error": {
    "code": "INVALID_MESSAGE",
    "message": "Failed to parse message"
  }
}
```

## Error Codes

| Code | Description |
|------|-------------|
| `INVALID_MESSAGE` | Message format is invalid |
| `SESSION_NOT_FOUND` | Specified session doesn't exist |
| `PROVIDER_ERROR` | LLM provider returned an error |
| `TOOL_ERROR` | Tool execution failed |
| `INTERNAL_ERROR` | Internal server error |

## Message Flow

### New Conversation

```mermaid
sequenceDiagram
    participant C as Client
    participant S as Server

    C->>S: WebSocket connect
    S-->>C: connected (session_id)
    C->>S: message
    S-->>C: chunk
    S-->>C: chunk
    S-->>C: done
```

### With Tool Calls

```mermaid
sequenceDiagram
    participant C as Client
    participant S as Server
    participant T as Tool Service

    C->>S: message
    S->>T: Execute tool
    S-->>C: tool_call
    T-->>S: Result
    S-->>C: tool_result
    S-->>C: chunk
    S-->>C: done
```

### Session Resumption

```mermaid
sequenceDiagram
    participant C as Client
    participant S as Server
    participant R as Session Store

    C->>S: WebSocket connect
    S-->>C: connected (session_id)
    C->>S: message (with session_id)
    S->>R: Load session history
    R-->>S: History
    S-->>C: done (with context)
```

## Session Handling

### New Session

Omit `session_id` to create a new session:

```json
{"type": "message", "content": "Hello"}
```

The server responds with a `connected` message containing the new session ID.

### Resume Session

Include `session_id` to resume:

```json
{
  "type": "message",
  "session_id": "sess-abc123",
  "content": "Continue our conversation"
}
```

If the session exists and hasn't expired, conversation history is preserved.

### Session Expiration

Sessions expire based on the AgentRuntime's `session.ttl` configuration. Attempting to resume an expired session creates a new one.

## Connection Health

The server sends WebSocket ping frames to maintain connection health. Clients should respond with pong frames automatically (most WebSocket libraries handle this).

Default timeouts:
- Ping interval: 30 seconds
- Pong timeout: 60 seconds
