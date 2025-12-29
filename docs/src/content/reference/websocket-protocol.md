---
title: "WebSocket Protocol"
description: "WebSocket message protocol reference"
order: 4
---

# WebSocket Protocol Reference

This document describes the WebSocket protocol used by Omnia agent facades.

## Connection

### URL Format

```
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
| `content` | string | Yes | User message content |
| `session_id` | string | No | Resume existing session |
| `metadata` | object | No | Custom metadata |

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

```
Client                          Server
   |                               |
   |-- connect ------------------>|
   |<-- connected (session_id) ---|
   |                               |
   |-- message ------------------->|
   |<-- chunk --------------------|
   |<-- chunk --------------------|
   |<-- done ---------------------|
```

### With Tool Calls

```
Client                          Server
   |                               |
   |-- message ------------------->|
   |<-- tool_call ----------------|
   |<-- tool_result --------------|
   |<-- chunk --------------------|
   |<-- done ---------------------|
```

### Session Resumption

```
Client                          Server
   |                               |
   |-- connect ------------------>|
   |<-- connected (session_id) ---|
   |                               |
   |-- message (session_id) ----->|
   |<-- done ---------------------|
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
