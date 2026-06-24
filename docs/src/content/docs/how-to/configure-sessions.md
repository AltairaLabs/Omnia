---
title: "Configure Context Store"
description: "Set up context persistence for agent conversations"
sidebar:
  order: 2
---


This guide explains how to configure the context store for maintaining conversation state across connections.

## Context Store Options

Omnia supports two context store backends:

| Backend | Use Case | Persistence |
|---------|----------|-------------|
| Memory | Development, testing | Pod lifetime only |
| Redis | Production, multi-replica | Persistent |

## Using In-Memory Context Store

In-memory storage is the default and requires no additional configuration:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: my-agent
spec:
  # ... other config
  context:
    type: memory
    ttl: 1h
```

> **Note**: Context is lost when the pod restarts. Not suitable for production with multiple replicas.

## Using Redis Context Store

For production deployments, use Redis:

### Step 1: Deploy Redis

```bash
kubectl create namespace redis
helm install redis bitnami/redis -n redis \
  --set auth.password=your-redis-password
```

### Step 2: Create Redis Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: redis-credentials
type: Opaque
stringData:
  url: "redis://:your-redis-password@redis-master.redis.svc:6379"
```

### Step 3: Configure AgentRuntime

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: my-agent
spec:
  # ... other config
  context:
    type: redis
    ttl: 24h
    storeRef:
      name: redis-credentials
      key: url
```

## Context TTL

The `ttl` field controls how long context remains valid after the last activity:

```yaml
context:
  ttl: 1h    # Context expires after 1 hour of inactivity
```

Supported formats:
- `30m` - 30 minutes
- `1h` - 1 hour
- `24h` - 24 hours
- `168h` - 1 week

## Resuming Sessions

Clients can resume existing sessions by providing the session ID:

```json
{
  "type": "message",
  "session_id": "existing-session-id",
  "content": "Continue our conversation..."
}
```

If the session exists and hasn't expired, the conversation history is preserved.

## Session Data

Each session stores:

- Conversation messages
- Agent state
- Custom metadata

Access session data programmatically using the session ID returned in the `connected` message:

```json
{"type": "connected", "session_id": "sess-abc123"}
```
