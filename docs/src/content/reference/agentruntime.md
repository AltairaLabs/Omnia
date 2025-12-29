---
title: "AgentRuntime CRD"
description: "Complete reference for the AgentRuntime custom resource"
order: 1
---

# AgentRuntime CRD Reference

The AgentRuntime custom resource defines an AI agent deployment in Kubernetes.

## API Version

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
```

## Spec Fields

### `replicas`

Number of agent pod replicas to run.

| Field | Type | Default | Required |
|-------|------|---------|----------|
| `replicas` | integer | 1 | No |

```yaml
spec:
  replicas: 3
```

### `provider`

LLM provider configuration.

| Field | Type | Required |
|-------|------|----------|
| `provider.name` | string | Yes |
| `provider.model` | string | Yes |
| `provider.apiKeySecretRef.name` | string | Yes |
| `provider.apiKeySecretRef.key` | string | Yes |

```yaml
spec:
  provider:
    name: openai
    model: gpt-4
    apiKeySecretRef:
      name: llm-credentials
      key: api-key
```

Supported providers:
- `openai` - OpenAI GPT models
- `anthropic` - Anthropic Claude models
- `google` - Google Gemini models

### `promptPackRef`

Reference to the PromptPack resource.

| Field | Type | Required |
|-------|------|----------|
| `promptPackRef.name` | string | Yes |
| `promptPackRef.namespace` | string | No |

```yaml
spec:
  promptPackRef:
    name: my-prompts
    namespace: prompts  # Optional, defaults to AgentRuntime namespace
```

### `toolRegistryRef`

Optional reference to a ToolRegistry resource.

| Field | Type | Required |
|-------|------|----------|
| `toolRegistryRef.name` | string | No |
| `toolRegistryRef.namespace` | string | No |

```yaml
spec:
  toolRegistryRef:
    name: agent-tools
```

### `facade`

WebSocket facade configuration.

| Field | Type | Default | Required |
|-------|------|---------|----------|
| `facade.type` | string | websocket | No |
| `facade.port` | integer | 8080 | No |

```yaml
spec:
  facade:
    type: websocket
    port: 8080
```

### `session`

Session storage configuration.

| Field | Type | Default | Required |
|-------|------|---------|----------|
| `session.type` | string | memory | No |
| `session.ttl` | duration | 1h | No |
| `session.storeRef.name` | string | - | No |
| `session.storeRef.key` | string | - | No |

```yaml
spec:
  session:
    type: redis
    ttl: 24h
    storeRef:
      name: redis-credentials
      key: url
```

### `resources`

Container resource requirements.

```yaml
spec:
  resources:
    requests:
      cpu: "500m"
      memory: "256Mi"
    limits:
      cpu: "1000m"
      memory: "512Mi"
```

### `env`

Additional environment variables.

```yaml
spec:
  env:
    - name: LOG_LEVEL
      value: debug
    - name: API_TIMEOUT
      valueFrom:
        configMapKeyRef:
          name: agent-config
          key: timeout
```

## Status Fields

### `phase`

Current phase of the AgentRuntime.

| Value | Description |
|-------|-------------|
| `Pending` | Resource created, waiting for dependencies |
| `Running` | Agent pods are running and ready |
| `Failed` | Deployment failed |

### `replicas`

Replica counts.

| Field | Description |
|-------|-------------|
| `status.replicas` | Desired replicas |
| `status.readyReplicas` | Ready replicas |
| `status.availableReplicas` | Available replicas |

### `conditions`

Standard Kubernetes conditions:

| Type | Description |
|------|-------------|
| `Available` | Agent is ready to accept connections |
| `PromptPackReady` | Referenced PromptPack is valid |
| `ToolRegistryReady` | Referenced ToolRegistry is valid |

## Example

Complete AgentRuntime example:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: production-agent
  namespace: agents
spec:
  replicas: 3
  provider:
    name: openai
    model: gpt-4-turbo
    apiKeySecretRef:
      name: openai-credentials
      key: api-key
  promptPackRef:
    name: customer-service-prompts
  toolRegistryRef:
    name: service-tools
  facade:
    type: websocket
    port: 8080
  session:
    type: redis
    ttl: 24h
    storeRef:
      name: redis-credentials
      key: url
  resources:
    requests:
      cpu: "500m"
      memory: "256Mi"
    limits:
      cpu: "1000m"
      memory: "512Mi"
  env:
    - name: LOG_LEVEL
      value: info
```
