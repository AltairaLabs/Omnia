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

```yaml
spec:
  promptPackRef:
    name: my-prompts
    version: "1.0.0"
```

### `providers`

A list of named provider references. Each entry maps a logical name to a [Provider](/reference/provider/) CRD. This enables centralized credential management, consistent configuration across agents, and explicit judge provider mapping for evals.

| Field | Type | Required |
|-------|------|----------|
| `providers[].name` | string | Yes |
| `providers[].providerRef.name` | string | Yes |
| `providers[].providerRef.namespace` | string | No (defaults to same namespace) |

The `name` field is a logical identifier used to look up providers by role:

| Name | Purpose |
|------|---------|
| `default` | Primary LLM provider for the runtime |
| `judge` | LLM judge for eval execution |
| Any custom name | Referenced by name in PromptPack eval definitions |

```yaml
spec:
  providers:
    - name: default
      providerRef:
        name: claude-sonnet
    - name: judge
      providerRef:
        name: claude-haiku
        namespace: shared-providers  # Optional cross-namespace reference
```

See the [Provider](/reference/provider/) reference for details on configuring Provider CRDs (types, secrets, defaults, etc.).

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

### `facade.media`

Optional media storage configuration for the facade. When enabled, clients can upload files via HTTP endpoints before referencing them in WebSocket messages.

| Field | Type | Default | Required |
|-------|------|---------|----------|
| `facade.media.enabled` | boolean | false | No |
| `facade.media.storagePath` | string | /var/omnia/media | No |
| `facade.media.publicURL` | string | - | Yes (if enabled) |
| `facade.media.maxFileSize` | string | 10Mi | No |
| `facade.media.defaultTTL` | duration | 24h | No |

```yaml
spec:
  facade:
    type: websocket
    port: 8080
    media:
      enabled: true
      storagePath: /var/omnia/media
      publicURL: https://agent.example.com
      maxFileSize: 10Mi
      defaultTTL: 24h
```

#### When to Use Facade Media Storage

Facade media storage is useful when:
- Using a custom runtime without built-in media externalization
- Need a runtime-agnostic upload endpoint
- Want to avoid base64-encoding large files in WebSocket messages

> **Note**: Runtimes like PromptKit have built-in media externalization, so facade media storage can remain disabled (the default).

#### Environment Variables

The facade media configuration is passed to the container via environment variables:

| Variable | Description |
|----------|-------------|
| `OMNIA_MEDIA_STORAGE_TYPE` | `none` (disabled) or `local` (enabled) |
| `OMNIA_MEDIA_STORAGE_PATH` | Directory for storing uploaded files |
| `OMNIA_MEDIA_PUBLIC_URL` | Base URL for generating download URLs |
| `OMNIA_MEDIA_MAX_FILE_SIZE` | Maximum upload size in bytes |
| `OMNIA_MEDIA_DEFAULT_TTL` | Default time-to-live for uploads |

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

### `media`

Media configuration for resolving `mock://` URLs in mock provider responses.

| Field | Type | Default | Required |
|-------|------|---------|----------|
| `media.basePath` | string | /etc/omnia/media | No |

```yaml
spec:
  media:
    basePath: /etc/omnia/media
```

The `basePath` sets the `OMNIA_MEDIA_BASE_PATH` environment variable, which the runtime uses to resolve `mock://` URLs to actual file paths. This is primarily used with the mock provider for testing multimodal responses.

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

### `runtime.volumes` and `runtime.volumeMounts`

Mount additional volumes in the runtime container for media files, mock configurations, or other data.

| Field | Type | Description |
|-------|------|-------------|
| `runtime.volumes` | []Volume | Kubernetes Volume definitions |
| `runtime.volumeMounts` | []VolumeMount | Volume mounts for the runtime container |

```yaml
spec:
  runtime:
    volumes:
      - name: mock-media
        persistentVolumeClaim:
          claimName: media-pvc
      - name: mock-config
        configMap:
          name: mock-responses
    volumeMounts:
      - name: mock-media
        mountPath: /etc/omnia/media
        readOnly: true
      - name: mock-config
        mountPath: /etc/omnia/mock
        readOnly: true
```

Supported volume types include:
- `persistentVolumeClaim` - Mount a PVC for persistent storage
- `configMap` - Mount a ConfigMap as files
- `secret` - Mount a Secret as files
- `emptyDir` - Temporary storage (cleared on pod restart)

This is commonly used with the mock provider to mount media files (images, audio) and mock response configurations for testing.

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

### `evals`

Configures realtime eval execution for this agent. When enabled, session events trigger evaluation of live conversations against eval definitions in the referenced PromptPack. See [Realtime Evals](/explanation/realtime-evals/) for the full architecture and [Configure Realtime Evals](/how-to/configure-realtime-evals/) for a step-by-step guide.

| Field | Type | Default | Required |
|-------|------|---------|----------|
| `evals.enabled` | boolean | false | No |

```yaml
spec:
  evals:
    enabled: true
```

#### `evals.inline` and `evals.worker`

Split realtime evals between two paths by group. Each eval's groups — auto-classified by handler type or declared explicitly on the eval — decide which path runs it.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `evals.inline.groups` | string[] | `["fast-running"]` | Groups executed synchronously in the runtime |
| `evals.worker.groups` | string[] | `["long-running", "external"]` | Groups executed out-of-band in the eval-worker |

```yaml
spec:
  evals:
    enabled: true
    inline:
      groups: ["fast-running"]
    worker:
      groups: ["long-running", "external"]
```

The defaults are disjoint — no eval runs on both paths. An absent or empty list falls back to the default; to disable all evals, use `enabled: false` (an empty `groups` list is not an off-switch).

Rows written to the `eval_results` table are tagged `source="runtime-inline"` from the inline path and `source="worker"` from the eval-worker, so aggregation over the table can distinguish them.

#### Judge Provider Resolution

LLM judge evals resolve their provider from the AgentRuntime's `spec.providers` list. Add a provider named `"judge"` (or any custom name referenced in your PromptPack eval definitions):

```yaml
spec:
  providers:
    - name: default
      providerRef:
        name: claude-sonnet       # Primary LLM for the agent
    - name: judge
      providerRef:
        name: claude-haiku        # Cheap/fast model for eval judging
```

The eval worker resolves provider credentials from the referenced Provider CRDs and their associated Secrets.

#### `evals.sampling`

Controls what percentage of sessions and turns are evaluated to manage cost.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `evals.sampling.defaultRate` | integer (0-100) | 100 | Sampling percentage for lightweight (in-process) evals |
| `evals.sampling.extendedRate` | integer (0-100) | 10 | Sampling percentage for extended (model-powered) evals |

```yaml
spec:
  evals:
    sampling:
      defaultRate: 100   # Run all lightweight evals
      extendedRate: 10   # Sample 10% for extended evals (cost control)
```

Sampling uses deterministic hashing on `sessionID:turnIndex`, so the same session/turn always produces the same sampling decision. Lightweight evals (e.g., `content_includes`) are fast and free to run, using `defaultRate`. Extended evals (model-powered evaluations) incur API costs and latency, so `extendedRate` is set lower by default.

#### `evals.rateLimit`

Limits eval execution throughput to prevent runaway costs.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `evals.rateLimit.maxEvalsPerSecond` | integer | 50 | Maximum evals executed per second |
| `evals.rateLimit.maxConcurrentJudgeCalls` | integer | 5 | Maximum concurrent LLM judge API calls |

```yaml
spec:
  evals:
    rateLimit:
      maxEvalsPerSecond: 50
      maxConcurrentJudgeCalls: 5
```

#### `evals.sessionCompletion`

Configures how session completion is detected for `on_session_complete` evals.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `evals.sessionCompletion.inactivityTimeout` | duration string | "5m" | Duration after last message before a session is considered complete |

```yaml
spec:
  evals:
    sessionCompletion:
      inactivityTimeout: 10m
```

### `rollout`

Progressive rollout configuration. When `rollout.candidate` is set and differs from the current spec, the controller creates a candidate Deployment and progresses through the defined steps.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `rollout.candidate` | object | No | Overrides for the candidate version |
| `rollout.candidate.promptPackVersion` | string | No | PromptPack version for the candidate |
| `rollout.candidate.providerRefs` | array | No | Provider overrides for the candidate |
| `rollout.candidate.toolRegistryRef` | object | No | ToolRegistry override for the candidate |
| `rollout.steps` | array | Yes | Ordered sequence of rollout actions |
| `rollout.steps[].setWeight` | integer | — | Set candidate traffic weight (0-100) |
| `rollout.steps[].pause` | object | — | Pause the rollout |
| `rollout.steps[].pause.duration` | string | No | Pause duration (e.g., "5m"). Omit for indefinite |
| `rollout.steps[].analysis` | object | — | Run a RolloutAnalysis template |
| `rollout.steps[].analysis.templateName` | string | Yes | Name of the RolloutAnalysis CRD |
| `rollout.steps[].analysis.args` | array | No | Argument overrides for the template |
| `rollout.stickySession` | object | No | Consistent routing for experiments |
| `rollout.stickySession.hashOn` | string | Yes | Header for consistent hashing (e.g., "x-user-id") |
| `rollout.rollback` | object | No | Rollback configuration |
| `rollout.rollback.mode` | string | No | `automatic`, `manual` (default), or `disabled` |
| `rollout.rollback.cooldown` | string | No | Debounce duration (default: "5m") |
| `rollout.trafficRouting` | object | No | Traffic management provider |
| `rollout.trafficRouting.istio.virtualService.name` | string | Yes | VirtualService to patch |
| `rollout.trafficRouting.istio.virtualService.routes` | array | Yes | Route names to manage |
| `rollout.trafficRouting.istio.destinationRule.name` | string | Yes | DestinationRule to patch |

:::note[Enterprise]
The `analysis` step type requires the `RolloutAnalysis` CRD, which is an enterprise feature.
:::

#### Rollout Example

```yaml
# Canary rollout with analysis
spec:
  promptPackRef:
    name: customer-support-pack
    version: "1.0.0"
  rollout:
    candidate:
      promptPackVersion: "2.0.0"
    steps:
      - setWeight: 10
      - pause:
          duration: "5m"
      - analysis:
          templateName: quality-check
      - setWeight: 50
      - pause:
          duration: "10m"
      - setWeight: 100
    rollback:
      mode: automatic
    trafficRouting:
      istio:
        virtualService:
          name: customer-support-vs
          routes: [primary]
        destinationRule:
          name: customer-support-dr
```

When candidate matches the current spec, the rollout is idle. Promotion copies candidate overrides into the main spec. Rollback reverts the candidate to match the current spec.

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

### `rollout` (status)

| Field | Description |
|-------|-------------|
| `status.rollout.active` | Whether a rollout is in progress |
| `status.rollout.currentStep` | Current step index |
| `status.rollout.currentWeight` | Current candidate traffic weight |
| `status.rollout.stableVersion` | Version serving stable traffic |
| `status.rollout.candidateVersion` | Version serving candidate traffic |

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

  providers:
    - name: default
      providerRef:
        name: claude-production
    - name: judge
      providerRef:
        name: claude-haiku

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

  evals:
    enabled: true
    sampling:
      defaultRate: 100
      extendedRate: 10
    rateLimit:
      maxEvalsPerSecond: 50
      maxConcurrentJudgeCalls: 5
    sessionCompletion:
      inactivityTimeout: 5m

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
