---
title: "AgentRuntime CRD"
description: "Complete reference for the AgentRuntime custom resource"
sidebar:
  order: 1
---


The AgentRuntime custom resource defines an AI agent deployment in Kubernetes.

## API Version

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
```

## Spec Fields

### `promptPackRef`

Reference to the PromptPack containing agent prompts.

| Field | Type | Required |
|-------|------|----------|
| `promptPackRef.name` | string | Yes |
| `promptPackRef.version` | string | No |
| `promptPackRef.track` | string | No (default: "stable") |

```yaml
spec:
  promptPackRef:
    name: my-prompts
    version: "1.0.0"  # Or use track: "canary"
```

### `providerRef` (Recommended)

Reference to a [Provider](/reference/provider/) resource for LLM configuration. This is the recommended approach as it enables centralized credential management and consistent configuration across agents.

| Field | Type | Required |
|-------|------|----------|
| `providerRef.name` | string | Yes |
| `providerRef.namespace` | string | No (defaults to same namespace) |

```yaml
spec:
  providerRef:
    name: claude-provider
    namespace: shared-providers  # Optional
```

### `provider` (Inline Configuration)

Inline provider configuration. Use `providerRef` instead for production deployments.

| Field | Type | Required |
|-------|------|----------|
| `provider.type` | string | Yes (`claude`, `openai`, `gemini`, `auto`) |
| `provider.model` | string | No |
| `provider.secretRef.name` | string | Yes |
| `provider.secretRef.key` | string | No |
| `provider.defaults.temperature` | string | No |
| `provider.defaults.topP` | string | No |
| `provider.defaults.maxTokens` | integer | No |

```yaml
spec:
  provider:
    type: claude
    model: claude-sonnet-4-20250514
    secretRef:
      name: llm-credentials
    defaults:
      temperature: "0.7"
```

The secret should contain the appropriate API key:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: llm-credentials
stringData:
  ANTHROPIC_API_KEY: "sk-ant-..."  # For Claude
  # Or: OPENAI_API_KEY: "sk-..."   # For OpenAI
  # Or: GEMINI_API_KEY: "..."      # For Gemini
```

> **Note**: If both `providerRef` and `provider` are specified, `providerRef` takes precedence.

### `framework`

Agent framework configuration. Specifies which runtime framework the agent uses.

| Field | Type | Default | Required |
|-------|------|---------|----------|
| `framework.type` | string | promptkit | No |
| `framework.version` | string | - | No |
| `framework.image` | string | - | No |

```yaml
spec:
  framework:
    type: promptkit
    version: "1.0.0"  # Optional version pinning
    image: myregistry.io/omnia-runtime:v1.0.0  # Optional image override
```

#### Framework Types

| Type | Description |
|------|-------------|
| `promptkit` | Default framework using PromptKit (recommended) |
| `custom` | Custom framework (requires `image` field) |

#### Image Override

The `framework.image` field allows you to override the default runtime container image. This is:
- **Required** when using `type: custom`
- **Optional** for built-in frameworks when you need a private registry or custom build

### `facade`

WebSocket facade configuration.

| Field | Type | Default | Required |
|-------|------|---------|----------|
| `facade.type` | string | websocket | Yes |
| `facade.port` | integer | 8080 | No |
| `facade.handler` | string | runtime | No |
| `facade.image` | string | - | No |

```yaml
spec:
  facade:
    type: websocket
    port: 8080
    handler: runtime
    image: myregistry.io/omnia-facade:v1.0.0  # Optional override
```

#### Handler Modes

| Mode | Description | Requires API Key |
|------|-------------|------------------|
| `runtime` | Production mode using the runtime framework | Yes |
| `demo` | Demo mode with simulated streaming responses | No |
| `echo` | Simple echo handler for testing connectivity | No |

#### Image Override

The `facade.image` field allows you to override the default facade container image. Use this when:
- Using a private container registry
- Running a custom build of the facade
- Pinning to a specific version different from the operator default

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
    namespace: tools  # Optional
```

### `session`

Session storage configuration.

| Field | Type | Default | Required |
|-------|------|---------|----------|
| `session.type` | string | memory | No |
| `session.ttl` | duration | 24h | No |
| `session.storeRef.name` | string | - | No |

```yaml
spec:
  session:
    type: redis
    ttl: 24h
    storeRef:
      name: redis-credentials
```

Session store types:
- `memory` - In-memory (not recommended for production)
- `redis` - Redis backend (recommended)
- `postgres` - PostgreSQL backend

### `runtime`

Deployment-related settings including replicas, resources, and autoscaling.

```yaml
spec:
  runtime:
    replicas: 3
    resources:
      requests:
        cpu: "500m"
        memory: "256Mi"
      limits:
        cpu: "1000m"
        memory: "512Mi"
    nodeSelector:
      node-type: agents
    tolerations:
      - key: "dedicated"
        operator: "Equal"
        value: "agents"
        effect: "NoSchedule"
```

### `runtime.autoscaling`

Horizontal pod autoscaling configuration. Supports both standard HPA and KEDA.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | boolean | false | Enable autoscaling |
| `type` | string | hpa | `hpa` or `keda` |
| `minReplicas` | integer | 1 | Minimum replicas (0 for KEDA scale-to-zero) |
| `maxReplicas` | integer | 10 | Maximum replicas |
| `targetMemoryUtilizationPercentage` | integer | 70 | Memory target (HPA only) |
| `targetCPUUtilizationPercentage` | integer | 90 | CPU target (HPA only) |
| `scaleDownStabilizationSeconds` | integer | 300 | Scale-down cooldown (HPA only) |

#### Standard HPA Example

```yaml
spec:
  runtime:
    autoscaling:
      enabled: true
      type: hpa
      minReplicas: 2
      maxReplicas: 10
      targetMemoryUtilizationPercentage: 70
      targetCPUUtilizationPercentage: 80
      scaleDownStabilizationSeconds: 300
```

#### KEDA Example

```yaml
spec:
  runtime:
    autoscaling:
      enabled: true
      type: keda
      minReplicas: 1  # Set to 0 for scale-to-zero
      maxReplicas: 20
      keda:
        pollingInterval: 15
        cooldownPeriod: 60
        triggers:
          - type: prometheus
            metadata:
              serverAddress: "http://prometheus-server:9090"
              query: 'sum(omnia_agent_connections_active{agent="my-agent"})'
              threshold: "10"
```

### `runtime.autoscaling.keda`

KEDA-specific configuration (only used when `type: keda`).

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `pollingInterval` | integer | 30 | Seconds between trigger checks |
| `cooldownPeriod` | integer | 300 | Seconds before scaling down |
| `triggers` | array | - | Custom KEDA triggers |

If no triggers are specified, a default Prometheus trigger scales based on `omnia_agent_connections_active`.

#### KEDA Trigger Types

**Prometheus trigger:**
```yaml
triggers:
  - type: prometheus
    metadata:
      serverAddress: "http://prometheus:9090"
      query: 'sum(rate(requests_total[1m]))'
      threshold: "100"
```

**Cron trigger:**
```yaml
triggers:
  - type: cron
    metadata:
      timezone: "America/New_York"
      start: "0 8 * * 1-5"   # 8am weekdays
      end: "0 18 * * 1-5"    # 6pm weekdays
      desiredReplicas: "5"
```

## Status Fields

### `phase`

| Value | Description |
|-------|-------------|
| `Pending` | Resource created, waiting for dependencies |
| `Running` | Agent pods are running and ready |
| `Failed` | Deployment failed |

### `replicas`

| Field | Description |
|-------|-------------|
| `status.replicas.desired` | Desired replicas |
| `status.replicas.ready` | Ready replicas |
| `status.replicas.available` | Available replicas |

### `conditions`

| Type | Description |
|------|-------------|
| `Ready` | Overall readiness |
| `DeploymentReady` | Deployment is ready |
| `ServiceReady` | Service is ready |
| `PromptPackReady` | Referenced PromptPack is valid |
| `ProviderReady` | Referenced Provider is valid |
| `ToolRegistryReady` | Referenced ToolRegistry is valid |

## Complete Example

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: production-agent
  namespace: agents
spec:
  promptPackRef:
    name: customer-service-prompts
    version: "2.1.0"

  providerRef:
    name: claude-production

  toolRegistryRef:
    name: service-tools

  facade:
    type: websocket
    port: 8080
    handler: runtime

  session:
    type: redis
    ttl: 24h
    storeRef:
      name: redis-credentials

  runtime:
    replicas: 3  # Ignored when autoscaling enabled
    resources:
      requests:
        cpu: "500m"
        memory: "256Mi"
      limits:
        cpu: "1000m"
        memory: "512Mi"
    autoscaling:
      enabled: true
      type: keda
      minReplicas: 1
      maxReplicas: 20
      keda:
        pollingInterval: 15
        cooldownPeriod: 120
        triggers:
          - type: prometheus
            metadata:
              serverAddress: "http://omnia-prometheus-server.omnia-system.svc.cluster.local/prometheus"
              query: 'sum(omnia_agent_connections_active{agent="production-agent",namespace="agents"}) or vector(0)'
              threshold: "10"
```
