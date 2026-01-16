---
title: "ArenaConfig CRD"
description: "Complete reference for the ArenaConfig custom resource"
sidebar:
  order: 11
  badge:
    text: Arena
    variant: note
---

The ArenaConfig custom resource defines a test configuration that combines an ArenaSource with providers and evaluation settings. It bridges PromptKit bundles with Omnia's existing Provider and ToolRegistry CRDs.

## API Version

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaConfig
```

## Overview

ArenaConfig provides:

- **Source binding**: Reference an ArenaSource for the PromptKit bundle
- **Provider selection**: Test scenarios against multiple LLM providers
- **Tool access**: Make ToolRegistry tools available during evaluation
- **Scenario filtering**: Include/exclude patterns for scenario selection
- **Self-play support**: Configure agent vs agent evaluation
- **Evaluation tuning**: Configure timeouts, retries, and concurrency

## Spec Fields

### `sourceRef`

Reference to the ArenaSource containing the PromptKit bundle.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Name of the ArenaSource |

```yaml
spec:
  sourceRef:
    name: customer-support-prompts
```

### `scenarios`

Filter which scenarios to run from the bundle.

| Field | Type | Description |
|-------|------|-------------|
| `include` | []string | Glob patterns for scenarios to include |
| `exclude` | []string | Glob patterns for scenarios to exclude |

```yaml
spec:
  scenarios:
    include:
      - "scenarios/billing-*.yaml"
      - "scenarios/support-*.yaml"
    exclude:
      - "*-wip.yaml"
      - "scenarios/experimental/*"
```

**Pattern matching:**
- Uses glob syntax (`*` matches any characters, `**` matches paths)
- Exclusions are applied after inclusions
- If `include` is empty, all scenarios are included by default

### `providers`

List of Provider CRDs to use for LLM credentials. Each provider is tested against all selected scenarios.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Name of the Provider |
| `namespace` | string | No | Namespace (defaults to config namespace) |

```yaml
spec:
  providers:
    - name: claude-sonnet
    - name: gpt-4o
    - name: gemini-pro
      namespace: shared-providers
```

### `toolRegistries`

List of ToolRegistry CRDs to make available during evaluation. Tools from all registries are merged.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Name of the ToolRegistry |
| `namespace` | string | No | Namespace (defaults to config namespace) |

```yaml
spec:
  toolRegistries:
    - name: customer-tools
    - name: billing-tools
```

### `selfPlay`

Configure self-play evaluation where agents compete against each other.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | boolean | false | Enable self-play mode |
| `rounds` | integer | 1 | Number of rounds per scenario |
| `swapRoles` | boolean | false | Alternate roles between rounds |

```yaml
spec:
  selfPlay:
    enabled: true
    rounds: 3
    swapRoles: true
```

### `evaluation`

Configure evaluation criteria and execution settings.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `metrics` | []string | - | Metrics to collect (latency, tokens, cost, quality) |
| `timeout` | string | "5m" | Max duration per evaluation |
| `maxRetries` | integer | 3 | Max retries for failures (0-10) |
| `concurrency` | integer | 1 | Parallel evaluations per worker (1-100) |

```yaml
spec:
  evaluation:
    metrics:
      - latency
      - tokens
      - cost
      - quality
    timeout: 10m
    maxRetries: 3
    concurrency: 5
```

### `suspend`

When `true`, prevents new jobs from being created. Existing jobs continue running.

```yaml
spec:
  suspend: true
```

## Status Fields

### `phase`

| Value | Description |
|-------|-------------|
| `Pending` | Config is being validated |
| `Ready` | Config is valid and ready for jobs |
| `Invalid` | Config has validation errors |
| `Error` | Error occurred during validation |

### `resolvedSource`

Information about the resolved ArenaSource.

| Field | Description |
|-------|-------------|
| `revision` | Artifact revision from the source |
| `url` | Artifact download URL |
| `scenarioCount` | Number of scenarios matching filter |

### `resolvedProviders`

List of validated provider names.

### `conditions`

| Type | Description |
|------|-------------|
| `Ready` | Overall readiness of the config |
| `SourceResolved` | ArenaSource successfully resolved |
| `ProvidersValid` | All provider references are valid |
| `ToolRegistriesValid` | All tool registry references are valid |

### `lastValidatedAt`

Timestamp of the last successful validation.

## Complete Examples

### Basic Configuration

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaConfig
metadata:
  name: basic-eval
  namespace: arena
spec:
  sourceRef:
    name: my-prompts

  providers:
    - name: claude-provider

  evaluation:
    timeout: 5m
```

### Multi-Provider Comparison

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaConfig
metadata:
  name: provider-comparison
  namespace: arena
spec:
  sourceRef:
    name: customer-support-prompts

  scenarios:
    include:
      - "scenarios/*.yaml"
    exclude:
      - "*-experimental.yaml"

  providers:
    - name: claude-sonnet
    - name: gpt-4o
    - name: gemini-pro

  evaluation:
    metrics:
      - latency
      - tokens
      - cost
      - quality
    timeout: 10m
    concurrency: 10
```

### Self-Play Evaluation

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaConfig
metadata:
  name: debate-eval
  namespace: arena
spec:
  sourceRef:
    name: debate-prompts

  providers:
    - name: claude-sonnet

  selfPlay:
    enabled: true
    rounds: 5
    swapRoles: true

  evaluation:
    timeout: 15m
    maxRetries: 2
```

### With Tool Access

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaConfig
metadata:
  name: tool-eval
  namespace: arena
spec:
  sourceRef:
    name: agent-prompts

  providers:
    - name: claude-sonnet

  toolRegistries:
    - name: search-tools
    - name: calculator-tools

  evaluation:
    timeout: 5m
    concurrency: 5
```

## Using ArenaConfig with ArenaJob

ArenaConfig is referenced by ArenaJob to execute test runs:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: evaluation-001
  namespace: arena
spec:
  configRef:
    name: provider-comparison
```

## Workflow

1. **Create ArenaSource** - Define where to fetch PromptKit bundles
2. **Create Providers** - Configure LLM credentials
3. **Create ArenaConfig** - Combine source with providers and settings
4. **Create ArenaJob** - Execute the evaluation

```
ArenaSource ──┐
              ├──▶ ArenaConfig ──▶ ArenaJob ──▶ Results
Provider(s) ──┘
```

## Related Resources

- **[ArenaSource](/reference/arenasource)**: Defines bundle sources
- **[ArenaJob](/reference/arenajob)**: Executes test runs
- **[Provider](/reference/provider)**: LLM provider configuration
- **[ToolRegistry](/reference/toolregistry)**: Tool definitions
