---
title: "Session Management"
description: "How Omnia manages conversation sessions"
order: 2
---

# Session Management

This document explains how Omnia manages conversation sessions for AI agents.

## What is a Session?

A session represents a single conversation between a client and an agent. It maintains:

- **Conversation history** - All messages exchanged
- **Agent state** - Internal state maintained by the agent
- **Metadata** - Custom data attached to the session

Sessions enable multi-turn conversations where the agent remembers previous context.

## Session Lifecycle

### Creation

A new session is created when:

1. A client connects without providing a session ID
2. A client provides a session ID that doesn't exist or has expired

The server assigns a unique session ID and returns it in the `connected` message.

### Active

While active, a session:

- Stores new messages as they're exchanged
- Maintains the agent's internal state
- Tracks the last activity timestamp

### Expiration

Sessions expire after a period of inactivity defined by `session.ttl`. When expired:

- The session data is deleted
- Attempting to resume creates a new session
- The conversation history is lost

## Session Stores

### In-Memory Store

The simplest option, suitable for development:

```yaml
session:
  type: memory
  ttl: 1h
```

Characteristics:
- Fast access
- No external dependencies
- Lost on pod restart
- Not suitable for multiple replicas

### Redis Store

Production-ready distributed storage:

```yaml
session:
  type: redis
  ttl: 24h
  storeRef:
    name: redis-credentials
    key: url
```

Characteristics:
- Persistent across restarts
- Works with multiple replicas
- Supports large session counts
- Requires Redis infrastructure

## Session Resumption

Clients can resume sessions by including the session ID:

```json
{
  "type": "message",
  "session_id": "sess-abc123",
  "content": "What did we discuss earlier?"
}
```

### Resumption Flow

1. Client sends message with session ID
2. Server looks up session in store
3. If found and not expired:
   - Load conversation history
   - Process new message with context
4. If not found:
   - Create new session
   - Process message without history

### Cross-Replica Resumption

With Redis sessions, clients can resume on any replica:

```
Client ──▶ Load Balancer ──▶ Any Agent Pod ──▶ Redis
```

## Session Data Structure

Each session stores:

```go
type Session struct {
    ID        string
    AgentName string
    Messages  []Message
    State     map[string]interface{}
    Metadata  map[string]interface{}
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

### Messages

The conversation history:

```go
type Message struct {
    Role      string  // "user", "assistant", "tool"
    Content   string
    Timestamp time.Time
}
```

### State

Agent-specific state that persists across messages. Useful for:

- Tracking conversation progress
- Storing extracted entities
- Managing multi-step workflows

### Metadata

Client-provided metadata:

```json
{
  "type": "message",
  "content": "...",
  "metadata": {
    "user_id": "user-123",
    "source": "mobile-app"
  }
}
```

## TTL Considerations

### Choosing a TTL

| Use Case | Recommended TTL |
|----------|-----------------|
| Quick queries | 15m - 1h |
| Support conversations | 1h - 4h |
| Ongoing projects | 24h - 168h |
| Persistent context | Use external state |

### TTL and Memory

Longer TTLs require more storage:

- Each message adds to session size
- Tool calls include full arguments/results
- Consider message pruning for long sessions

### TTL Refresh

TTL is refreshed on every activity:

1. Client sends message
2. Server updates `UpdatedAt` timestamp
3. TTL countdown restarts

## Scaling Considerations

### Single Replica

With one replica, in-memory sessions work fine. But:

- Sessions are lost on pod restart
- No horizontal scaling

### Multiple Replicas

With multiple replicas, you must use Redis:

1. Install Redis in your cluster
2. Configure `session.type: redis`
3. All replicas share session state

### Session Affinity Alternative

If you can't use Redis, configure service affinity:

```yaml
apiVersion: v1
kind: Service
spec:
  sessionAffinity: ClientIP
```

This routes the same client to the same pod, but:

- Sessions still lost on pod restart
- Uneven load distribution
- Not recommended for production

## Best Practices

1. **Use Redis for production** - Always use Redis with multiple replicas
2. **Set appropriate TTLs** - Balance memory usage with user experience
3. **Handle expiration gracefully** - Clients should expect session loss
4. **Don't store sensitive data** - Sessions may be logged or cached
5. **Monitor session counts** - Alert on unusual growth
