---
title: "ArenaJob CRD"
description: "Complete reference for the ArenaJob custom resource"
sidebar:
  order: 12
  badge:
    text: Enterprise
    variant: tip
---

:::note[Enterprise Feature]
ArenaJob is an enterprise feature. The CRD is only installed when `enterprise.enabled=true` in your Helm values. See [Installing a License](/how-to/operations/install-license/) for details.
:::

The ArenaJob custom resource defines a test execution that runs scenarios from an [ArenaSource](/reference/evaluation/arenasource/) bundle. It reads the [arena config file](/reference/evaluation/arenaconfig/) inside that bundle (selected by `spec.arenaFile`) and executes evaluation, load testing, or data generation jobs with configurable workers and output destinations.

## API Version

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
```

**Short name:** `aj` (e.g. `kubectl get aj`).

## Overview

ArenaJob provides:

- **Multiple job types**: Evaluation, load testing, and data generation
- **CRD-based providers**: Resolve providers and agents from Provider/AgentRuntime CRDs
- **Worker scaling**: Configure replicas and autoscaling
- **Flexible output**: Store results in S3 or PVC
- **Scheduling support**: Cron-based recurring execution
- **Progress tracking**: Real-time status and progress updates

## Spec Fields

### `sourceRef`

Reference to the ArenaSource containing the bundle (arena config file, scenarios, prompts). Required.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Name of the ArenaSource |

```yaml
spec:
  sourceRef:
    name: my-evaluation-source
```

### `arenaFile`

Path to the arena config file **within the source bundle**. Supports glob patterns for multi-file configs. Defaults to `config.arena.yaml`.

See the [Arena Config File](/reference/evaluation/arenaconfig/) reference for the schema of this file.

```yaml
spec:
  arenaFile: config.arena.yaml
```

```yaml
spec:
  # Glob across several config files
  arenaFile: "evals/*.arena.yaml"
```

### `type`

The type of job to execute. Defaults to `evaluation`.

| Value | Description |
|-------|-------------|
| `evaluation` | Run prompt evaluation against test scenarios (default) |
| `loadtest` | Run load testing against providers |
| `datagen` | Generate synthetic data using prompts |

```yaml
spec:
  type: evaluation
```

### `trials`

Number of times to repeat each scenario × provider combination. Overrides per-scenario trials defined in the scenario YAML files. Minimum `1`.

- For **evaluation** jobs, trials provide statistical confidence (pass rate, flakiness score).
- For **loadtest** jobs, trials define the total load volume, consumed under concurrency control.

```yaml
spec:
  trials: 20
```

### `scenarios`

Filter which scenarios to run from the arena file. If not specified, runs all scenarios defined in the arena file.

| Field | Type | Description |
|-------|------|-------------|
| `include` | []string | Glob patterns for scenarios to include |
| `exclude` | []string | Glob patterns for scenarios to exclude |

Exclusions are applied after inclusions. If `include` is empty, all scenarios are included by default.

```yaml
spec:
  scenarios:
    include:
      - "scenarios/critical-*.yaml"
    exclude:
      - "*-slow.yaml"
```

### `evaluation`

Settings specific to evaluation jobs (used when `type: evaluation`).

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

Settings specific to load testing jobs (used when `type: loadtest`).

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `concurrency` | integer | 1 | Maximum number of work items in flight across all workers. Workers check the global in-flight count before popping new items. Minimum 1. |
| `vusPerWorker` | integer | 1 | Number of virtual users (concurrent goroutines) per worker pod. Each VU independently pops, executes, and reports work items. Minimum 1. |
| `ramp` | object | - | Linear concurrency ramp-up / ramp-down (see below). |
| `budgetLimit` | string | - | Maximum cost (in `budgetCurrency`) before the job is stopped. The controller checks the cost accumulator periodically and cancels remaining work if this limit is exceeded. |
| `budgetCurrency` | string | "USD" | Currency for `budgetLimit`. |
| `thresholds` | []object | - | SLO targets evaluated after the load test completes (see below). |

#### `loadTest.ramp`

Controls how concurrency changes over the course of a load test. Both values are duration strings (e.g. `"2m"`, `"30s"`).

| Field | Type | Description |
|-------|------|-------------|
| `up` | string | Duration to linearly ramp from 0 to target concurrency at the start. |
| `down` | string | Duration to linearly ramp from target concurrency to 0 at the end. Ramp-down is triggered when remaining pending items fall below `concurrency × 2`. |

#### `loadTest.thresholds`

Each threshold is an SLO gate evaluated after the load test completes. The job **fails** if any threshold is violated, which enables CI/CD gating.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `metric` | string | Yes | Metric to evaluate (see allowed values below). |
| `operator` | string | Yes | Comparison operator: `<`, `>`, `<=`, `>=`. |
| `value` | string | Yes | Target value to compare against. |

**Allowed `metric` values:** `latency_avg`, `latency_p50`, `latency_p90`, `latency_p95`, `latency_p99`, `ttft_avg`, `ttft_p50`, `ttft_p90`, `ttft_p95`, `ttft_p99`, `error_rate`, `pass_rate`, `total_cost`, `rate_limit_rate`.

**`value` formats:**
- Latency / TTFT metrics: a duration string (e.g. `"3s"`, `"500ms"`).
- Rate metrics (`error_rate`, `pass_rate`, `rate_limit_rate`): a float string (e.g. `"0.01"`, `"0.95"`).
- Cost metric (`total_cost`): a numeric string (e.g. `"50.00"`).

```yaml
spec:
  type: loadtest
  loadTest:
    concurrency: 50
    vusPerWorker: 10
    ramp:
      up: 2m
      down: 30s
    budgetLimit: "25.00"
    budgetCurrency: USD
    thresholds:
      - metric: latency_p95
        operator: "<"
        value: "3s"
      - metric: error_rate
        operator: "<="
        value: "0.01"
```

### `dataGen`

Settings specific to data generation jobs (used when `type: datagen`).

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
| `podOverrides` | object | - | Customizes the worker Job Pods (scheduling, ServiceAccount, CSI secret-stores, custom `envFrom` for provider credentials, etc.) |

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

### `providers`

Maps group names to provider groups. Group names correspond to the arena config file's provider groups — the `group:` value on each `providers:` entry in `config.arena.yaml` (e.g. `"default"`, `"judge"`).

When `providers` is set, provider YAML files from the arena bundle are **ignored** and the worker resolves providers directly from CRDs.

Each entry is an `ArenaProviderEntry` with exactly one of the following fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `providerRef` | object | Conditional | Reference to a Provider CRD |
| `providerRef.name` | string | Yes | Name of the Provider resource |
| `providerRef.namespace` | string | No | Namespace (defaults to the ArenaJob's namespace) |
| `agentRef` | object | Conditional | Reference to an AgentRuntime CRD |
| `agentRef.name` | string | Yes | Name of the AgentRuntime resource |

A CEL validation rule enforces that exactly one of `providerRef` or `agentRef` is set on each entry. Setting both or neither is rejected at admission time.

Agents and LLM providers are interchangeable in the scenario × provider matrix. An `agentRef` entry causes the worker to connect to the agent over WebSocket instead of making direct LLM API calls.

#### Array mode vs map mode

Each provider group accepts **two shapes**, and the two can be mixed across groups within the same job:

- **Array mode (default)** — a list of entries. The group is a *pool* of test providers, and every entry is run against every selected scenario.
- **Map mode** — an object keyed by config-provider-ID. Each key is the exact provider ID the arena config expects, mapped 1:1 to a CRD entry. Use this when the arena config references specific provider IDs (for example, the deploy wizard emits map mode so the config's provider IDs resolve exactly).

**Array mode:**

```yaml
spec:
  providers:
    default:
      - providerRef:
          name: gpt4-prod
      - providerRef:
          name: claude-sonnet
```

**Map mode:**

```yaml
spec:
  providers:
    default:
      # config-provider-ID → CRD
      primary:
        providerRef:
          name: gpt4-prod
      secondary:
        providerRef:
          name: claude-sonnet
```

#### Example: Multiple Providers in a Group

When an array group contains multiple entries, each provider is evaluated against every scenario:

```yaml
spec:
  providers:
    default:
      - providerRef:
          name: gpt4-prod
      - providerRef:
          name: claude-sonnet
      - providerRef:
          name: gemini-pro
```

#### Example: Separate Judge Provider

Use a dedicated provider group for the judge (evaluator) model. The group name (`judge`) must match a `group:` value used in the arena config's `providers:` list:

```yaml
spec:
  providers:
    default:
      - providerRef:
          name: gpt4-prod
      - providerRef:
          name: claude-sonnet
    judge:
      - providerRef:
          name: claude-opus
```

#### Example: Agent Entry

Reference a deployed AgentRuntime instead of a raw LLM provider. The worker connects to the agent's WebSocket endpoint:

```yaml
spec:
  providers:
    default:
      - agentRef:
          name: my-support-agent
```

#### Example: Self-Play with Mixed Types

Mix LLM providers and agents in a self-play evaluation:

```yaml
spec:
  providers:
    selfplay:
      - providerRef:
          name: gpt4-prod
      - agentRef:
          name: my-agent-v2
    judge:
      - providerRef:
          name: claude-opus
```

#### Example: Cross-Namespace Provider

Reference a Provider in a different namespace:

```yaml
spec:
  providers:
    default:
      - providerRef:
          name: shared-gpt4
          namespace: shared-providers
```

### `toolRegistries`

List of ToolRegistry CRD references whose discovered tools replace the arena config's tool and MCP server file references. When set, tool YAML files from the arena bundle are ignored.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Name of the ToolRegistry resource |

```yaml
spec:
  toolRegistries:
    - name: production-tools
```

#### How Tool Registries Work

1. The controller reads each referenced ToolRegistry CRD
2. Discovered tools from each registry's status are extracted
3. These tools replace any tools defined in the arena config files
4. The worker receives the resolved tool endpoints via configuration

This is useful for:

- Switching between mock and real tool implementations per environment
- Routing tool calls to different endpoints
- Dynamic service discovery for tool handlers

#### Example: Multiple Tool Registries

```yaml
spec:
  toolRegistries:
    - name: core-tools
    - name: billing-tools
```

#### Combining Providers and Tool Registries

You can use both `providers` and `toolRegistries` together for complete CRD-based runtime configuration:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: production-eval
spec:
  sourceRef:
    name: my-source
  arenaFile: config.arena.yaml
  providers:
    default:
      - providerRef:
          name: gpt4-prod
      - providerRef:
          name: claude-sonnet
    judge:
      - providerRef:
          name: claude-opus
  toolRegistries:
    - name: production-tools
  workers:
    replicas: 5
  output:
    type: s3
    s3:
      bucket: arena-results
      prefix: "evals/"
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
| `secretRef` | object | No | Credentials secret reference (keys: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`) |

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

### `cancelled`

Requests cancellation of a running job. When set to `true`, the operator deletes the worker Job and transitions the job to the `Cancelled` phase. Has no effect once the job has reached a terminal phase (`Succeeded`/`Failed`/`Cancelled`).

```yaml
spec:
  cancelled: true
```

### `verbose`

Enables verbose/debug logging for arena execution. When enabled, workers pass `--verbose` to the arena engine for detailed output.

```yaml
spec:
  verbose: true
```

### `sessionRecording`

Enables writing session data to session-api during execution. When `false` (default), no sessions are created and no events are recorded, reducing session-api load during high-volume load tests. Telemetry and traces are unaffected.

```yaml
spec:
  sessionRecording: true
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
| `SourceValid` | Referenced ArenaSource is valid and ready |
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
  sourceRef:
    name: my-source
  arenaFile: config.arena.yaml
```

### Multi-Worker Evaluation

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: parallel-eval
  namespace: arena
spec:
  sourceRef:
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
  sourceRef:
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
  sourceRef:
    name: load-test-source
  type: loadtest
  trials: 5000
  sessionRecording: false
  loadTest:
    concurrency: 100
    vusPerWorker: 20
    ramp:
      up: 2m
      down: 1m
    budgetLimit: "100.00"
    thresholds:
      - metric: latency_p95
        operator: "<"
        value: "3s"
      - metric: error_rate
        operator: "<="
        value: "0.02"
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
  sourceRef:
    name: datagen-source
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

1. **Create an ArenaSource** — a bundle containing the arena config file (`config.arena.yaml`) plus scenarios and prompts.
2. **Create an ArenaJob** — reference the source, select the config file with `arenaFile`, and specify execution parameters (job type, providers, workers).
3. **Monitor progress** — watch `status.progress` for completion.
4. **Retrieve results** — access results from the configured output destination.

```
ArenaSource ──▶ ArenaJob ──▶ Workers ──▶ Results
 (bundle:          │
  config.arena.yaml│
  + scenarios)     ├──▶ Progress tracking
                   └──▶ Output storage
```

## Related Resources

- **[ArenaSource](/reference/evaluation/arenasource/)**: Defines the bundle source (git, OCI, ConfigMap, or workspace)
- **[Arena Config File](/reference/evaluation/arenaconfig/)**: Schema of the `config.arena.yaml` file inside the bundle
- **[Provider](/reference/core/provider/)**: LLM provider configuration
