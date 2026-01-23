---
title: "ArenaJob CRD"
description: "Complete reference for the ArenaJob custom resource"
sidebar:
  order: 12
  badge:
    text: Arena
    variant: note
---

The ArenaJob custom resource defines a test execution that runs scenarios from an ArenaConfig. It supports evaluation, load testing, and data generation job types with configurable workers and output destinations.

## API Version

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
```

## Overview

ArenaJob provides:

- **Multiple job types**: Evaluation, load testing, and data generation
- **Worker scaling**: Configure replicas and autoscaling
- **Flexible output**: Store results in S3 or PVC
- **Scheduling support**: Cron-based recurring execution
- **Progress tracking**: Real-time status and progress updates

## Spec Fields

### `configRef`

Reference to the ArenaConfig containing the test configuration.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Name of the ArenaConfig |

```yaml
spec:
  configRef:
    name: my-evaluation-config
```

### `type`

The type of job to execute.

| Value | Description |
|-------|-------------|
| `evaluation` | Run prompt evaluation against test scenarios (default) |
| `loadtest` | Run load testing against providers |
| `datagen` | Generate synthetic data using prompts |

```yaml
spec:
  type: evaluation
```

### `scenarios`

Override scenario selection from the ArenaConfig. If not specified, uses the ArenaConfig's scenario settings.

| Field | Type | Description |
|-------|------|-------------|
| `include` | []string | Glob patterns for scenarios to include |
| `exclude` | []string | Glob patterns for scenarios to exclude |

```yaml
spec:
  scenarios:
    include:
      - "scenarios/critical-*.yaml"
    exclude:
      - "*-slow.yaml"
```

### `evaluation`

Settings specific to evaluation jobs.

| Field | Type | Description |
|-------|------|-------------|
| `outputFormats` | []string | Result formats: junit, json, csv |

```yaml
spec:
  type: evaluation
  evaluation:
    outputFormats:
      - junit
      - json
```

### `loadTest`

Settings specific to load testing jobs.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `rampUp` | string | "30s" | Duration to ramp up to target |
| `duration` | string | "5m" | Total test duration |
| `targetRPS` | integer | - | Target requests per second |

```yaml
spec:
  type: loadtest
  loadTest:
    rampUp: 1m
    duration: 10m
    targetRPS: 100
```

### `dataGen`

Settings specific to data generation jobs.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `count` | integer | 100 | Number of items to generate |
| `format` | string | "jsonl" | Output format: json, jsonl, csv |

```yaml
spec:
  type: datagen
  dataGen:
    count: 1000
    format: jsonl
```

### `workers`

Configure the worker pool for job execution.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `replicas` | integer | 1 | Number of worker replicas |
| `minReplicas` | integer | - | Minimum for autoscaling |
| `maxReplicas` | integer | - | Maximum for autoscaling |

```yaml
spec:
  workers:
    replicas: 10
```

For autoscaling:

```yaml
spec:
  workers:
    minReplicas: 2
    maxReplicas: 20
```

### `providerOverrides`

Override providers for specific groups using Kubernetes label selectors. This allows dynamic provider selection at runtime based on Provider CRD labels.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `<groupName>` | object | - | Override for a specific group |
| `<groupName>.selector` | LabelSelector | Yes | Kubernetes label selector for Provider CRDs |

Groups are defined in the ArenaConfig's pack configuration. Use `*` as a catch-all for groups not explicitly specified.

```yaml
spec:
  providerOverrides:
    default:
      selector:
        matchLabels:
          tier: production
          team: ml
    judge:
      selector:
        matchLabels:
          role: evaluator
        matchExpressions:
          - key: model-size
            operator: In
            values: ["large", "xlarge"]
```

#### Label Selector Fields

The selector follows standard Kubernetes label selector syntax:

| Field | Type | Description |
|-------|------|-------------|
| `matchLabels` | map[string]string | Exact label matches (AND logic) |
| `matchExpressions` | []LabelSelectorRequirement | Advanced matching rules |

**matchExpressions operators:**

| Operator | Description | Example |
|----------|-------------|---------|
| `In` | Value in set | `values: ["a", "b"]` |
| `NotIn` | Value not in set | `values: ["test"]` |
| `Exists` | Label key exists | (no values needed) |
| `DoesNotExist` | Label key absent | (no values needed) |

#### Example: Multi-Provider Evaluation

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: gpt4-prod
  labels:
    tier: production
    provider-type: openai
spec:
  type: openai
  model: gpt-4
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: claude-judge
  labels:
    role: evaluator
    provider-type: anthropic
spec:
  type: claude
  model: claude-3-opus
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: eval-with-overrides
spec:
  configRef:
    name: my-config
  providerOverrides:
    default:
      selector:
        matchLabels:
          tier: production
    judge:
      selector:
        matchLabels:
          role: evaluator
```

### `toolRegistryOverride`

Override tools defined in `arena.config.yaml` with handlers from ToolRegistry CRDs. This allows dynamic tool endpoint resolution at runtime based on Kubernetes label selectors.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `selector` | LabelSelector | Yes | Kubernetes label selector for ToolRegistry CRDs |

When specified, all tools from matching ToolRegistry CRDs will override tools with matching names in the arena config file. This is useful for:

- Switching between mock and real tool implementations
- Routing tool calls to different endpoints per environment
- Dynamic service discovery for tool handlers

```yaml
spec:
  toolRegistryOverride:
    selector:
      matchLabels:
        environment: production
```

#### How Tool Overrides Work

1. The controller finds all ToolRegistry CRDs matching the label selector
2. Tools are extracted from each registry's handlers and discovered tools
3. These tools override any tools with the same name in `arena.config.yaml`
4. The worker receives the resolved tool endpoints via configuration

#### Example: Override Mock Tools with Real Implementations

```yaml
# ToolRegistry with real tool implementations
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ToolRegistry
metadata:
  name: production-tools
  labels:
    environment: production
spec:
  handlers:
    - name: weather-handler
      type: http
      tool:
        name: get_weather
        description: Get real weather data
        inputSchema:
          type: object
          properties:
            city:
              type: string
      httpConfig:
        endpoint: http://weather-api.prod.svc:8080/weather
---
# ArenaJob using real tools
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: eval-with-real-tools
spec:
  configRef:
    name: my-config
  toolRegistryOverride:
    selector:
      matchLabels:
        environment: production
```

#### Combining with Provider Overrides

You can use both `providerOverrides` and `toolRegistryOverride` together for complete runtime configuration:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: production-eval
spec:
  configRef:
    name: my-config
  providerOverrides:
    default:
      selector:
        matchLabels:
          tier: production
  toolRegistryOverride:
    selector:
      matchLabels:
        environment: production
```

### `output`

Configure where job results are stored.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | Destination type: s3, pvc |
| `s3` | object | Conditional | S3 configuration (when type is s3) |
| `pvc` | object | Conditional | PVC configuration (when type is pvc) |

#### S3 Output

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `bucket` | string | Yes | S3 bucket name |
| `prefix` | string | No | Key prefix for objects |
| `region` | string | No | AWS region |
| `endpoint` | string | No | Custom S3-compatible endpoint |
| `secretRef` | object | No | Credentials secret reference |

```yaml
spec:
  output:
    type: s3
    s3:
      bucket: arena-results
      prefix: "evals/nightly/"
      region: us-west-2
      secretRef:
        name: s3-credentials
```

#### PVC Output

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `claimName` | string | Yes | PVC name |
| `subPath` | string | No | Subdirectory within PVC |

```yaml
spec:
  output:
    type: pvc
    pvc:
      claimName: arena-results-pvc
      subPath: "evals/"
```

### `schedule`

Configure scheduled/recurring job execution.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `cron` | string | - | Cron expression for scheduling |
| `timezone` | string | "UTC" | Timezone for cron |
| `concurrencyPolicy` | string | "Forbid" | Allow, Forbid, or Replace |

```yaml
spec:
  schedule:
    cron: "0 2 * * *"  # 2am daily
    timezone: "America/New_York"
    concurrencyPolicy: Forbid
```

### `ttlSecondsAfterFinished`

How long to keep completed jobs before automatic cleanup.

```yaml
spec:
  ttlSecondsAfterFinished: 86400  # 24 hours
```

## Status Fields

### `phase`

| Value | Description |
|-------|-------------|
| `Pending` | Job is waiting to start |
| `Running` | Job is actively executing |
| `Succeeded` | Job completed successfully |
| `Failed` | Job failed |
| `Cancelled` | Job was cancelled |

### `progress`

Tracks job execution progress.

| Field | Description |
|-------|-------------|
| `total` | Total number of work items |
| `completed` | Successfully completed items |
| `failed` | Failed items |
| `pending` | Pending items |

### `result`

Contains summary results for completed jobs.

| Field | Description |
|-------|-------------|
| `url` | URL to access detailed results |
| `summary` | Aggregated result metrics |

### `conditions`

| Type | Description |
|------|-------------|
| `Ready` | Overall readiness of the job |
| `ConfigValid` | ArenaConfig reference is valid and ready |
| `JobCreated` | Worker K8s Job has been created |
| `Progressing` | Job is actively executing workers |

### Timing Fields

| Field | Description |
|-------|-------------|
| `startTime` | When the job started |
| `completionTime` | When the job completed |
| `lastScheduleTime` | Last scheduled job trigger |
| `nextScheduleTime` | Next scheduled execution |

### `activeWorkers`

Current number of active worker pods.

## Complete Examples

### Basic Evaluation Job

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: basic-eval
  namespace: arena
spec:
  configRef:
    name: my-config
```

### Multi-Worker Evaluation

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: parallel-eval
  namespace: arena
spec:
  configRef:
    name: provider-comparison
  type: evaluation
  evaluation:
    outputFormats:
      - junit
      - json
  workers:
    replicas: 10
  output:
    type: s3
    s3:
      bucket: arena-results
      prefix: "evals/parallel/"
```

### Scheduled Nightly Evaluation

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: nightly-eval
  namespace: arena
spec:
  configRef:
    name: production-tests
  type: evaluation
  workers:
    replicas: 5
  output:
    type: s3
    s3:
      bucket: arena-results
      prefix: "evals/nightly/"
  schedule:
    cron: "0 2 * * *"
    timezone: "UTC"
  ttlSecondsAfterFinished: 604800  # 7 days
```

### Load Testing Job

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: provider-loadtest
  namespace: arena
spec:
  configRef:
    name: load-test-config
  type: loadtest
  loadTest:
    rampUp: 2m
    duration: 30m
    targetRPS: 500
  workers:
    minReplicas: 5
    maxReplicas: 50
  output:
    type: s3
    s3:
      bucket: loadtest-results
      prefix: "loadtests/"
```

### Data Generation Job

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: synthetic-data
  namespace: arena
spec:
  configRef:
    name: datagen-config
  type: datagen
  dataGen:
    count: 10000
    format: jsonl
  workers:
    replicas: 4
  output:
    type: pvc
    pvc:
      claimName: generated-data
      subPath: "batch-001/"
```

## Workflow

1. **Create ArenaConfig** - Define test configuration with providers and settings
2. **Create ArenaJob** - Reference the config and specify execution parameters
3. **Monitor Progress** - Watch status.progress for completion
4. **Retrieve Results** - Access results from configured output destination

```
ArenaConfig ──▶ ArenaJob ──▶ Workers ──▶ Results
                    │
                    ├──▶ Progress tracking
                    └──▶ Output storage
```

## Related Resources

- **[ArenaSource](/reference/arenasource)**: Defines bundle sources
- **[ArenaConfig](/reference/arenaconfig)**: Test configuration
- **[Provider](/reference/provider)**: LLM provider configuration
