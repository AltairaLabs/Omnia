---
title: "Arena Config File"
description: "Reference for the config.arena.yaml file inside an ArenaSource bundle"
sidebar:
  order: 11
  badge:
    text: Enterprise
    variant: tip
---

:::note[Enterprise Feature]
The arena config file drives the enterprise Arena Fleet feature. Arena CRDs are only installed when `enterprise.enabled=true` in your Helm values. See [Installing a License](/how-to/operations/install-license/) for details.
:::

The **arena config file** (conventionally `config.arena.yaml`) is a YAML file that lives **inside an [ArenaSource](/reference/evaluation/arenasource/) bundle**. It ties together the prompts, scenarios, providers, tools, and evaluation settings that a run uses. An [ArenaJob](/reference/evaluation/arenajob/) selects it with `spec.arenaFile` (default `config.arena.yaml`) and the arena worker loads it from the bundle at execution time.

:::caution[This is a file, not a CRD]
There is no `ArenaConfig` Kubernetes resource. Do **not** `kubectl apply` this document — it is a plain YAML file committed alongside your scenarios in the ArenaSource bundle. The only Arena CRDs are [ArenaSource](/reference/evaluation/arenasource/) and [ArenaJob](/reference/evaluation/arenajob/).
:::

## File shape

The file uses a lightweight PromptKit envelope. Everything below lives under `spec:`.

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: customer-support-eval
spec:
  # ... fields documented below
```

Most sections reference sibling files inside the bundle (relative to the config file's directory) rather than inlining content. This keeps prompts, scenarios, and providers in their own files.

## Spec Fields

### `prompt_configs`

Prompt configurations available to scenarios, referenced by file.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | Yes | Prompt identifier referenced by scenarios |
| `file` | string | Yes | Path to the prompt YAML within the bundle |

```yaml
spec:
  prompt_configs:
    - id: assistant
      file: prompts/assistant.yaml
```

### `providers`

The provider entries used by scenarios. Each entry references a provider YAML file and, optionally, assigns the provider to a **group**.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `file` | string | Yes | Path to the provider YAML within the bundle |
| `group` | string | No | Provider group name (e.g. `default`, `judge`) |

```yaml
spec:
  providers:
    - file: providers/gpt4.provider.yaml
      group: default
    - file: providers/claude-opus.provider.yaml
      group: judge
```

:::tip[Groups connect the file to the ArenaJob]
The `group` value is the join key with [`ArenaJob.spec.providers`](/reference/evaluation/arenajob/#providers). When an ArenaJob sets `spec.providers`, the provider YAML files here are ignored and the worker resolves providers from Provider/AgentRuntime CRDs instead — matching each ArenaJob provider-group name to the `group` used here. Leave `providers` empty (`providers: []`) when you intend to supply all providers from CRDs.
:::

### `scenarios`

Scenario files to run, each referenced by file.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `file` | string | Yes | Path to the scenario YAML within the bundle |

```yaml
spec:
  scenarios:
    - file: scenarios/billing.scenario.yaml
    - file: scenarios/support.scenario.yaml
```

### `judges` and `judge_defaults`

Named judge (evaluator) targets. Each judge inherits its model from the referenced provider ID.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Judge identifier used in assertions |
| `provider` | string | Yes | Provider ID reference (must exist in `spec.providers`) |

```yaml
spec:
  judges:
    - name: quality-judge
      provider: claude-opus
```

### `evals` and `tools`

Additional evaluation definitions and tool definitions, each referenced by file.

```yaml
spec:
  evals:
    - file: evals/quality.eval.yaml
  tools:
    - file: tools/search.tool.yaml
```

:::note
When an ArenaJob sets `spec.toolRegistries`, the tool file references here are ignored and tools are resolved from ToolRegistry CRDs instead.
:::

### `mcp_servers`

Model Context Protocol (MCP) servers to expose to the model during a run. Each server specifies exactly one transport (`command` for stdio, `url` for HTTP, or `source` for a host-provisioned endpoint).

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Server identifier |
| `command` / `args` / `env` | string / []string / map | stdio transport (local subprocess) |
| `url` / `headers` | string / map | HTTP transport |
| `transport` | string | `stdio`, `sse`, or `streamable_http` |
| `tool_filter` | object | `allowlist` / `blocklist` of tool names |

```yaml
spec:
  mcp_servers:
    - name: filesystem
      command: mcp-server-filesystem
      args: ["--root", "/data"]
```

### `self_play`

Configure self-play (agent-vs-simulated-user) evaluation. Self-play is enabled whenever this section is present.

| Field | Type | Description |
|-------|------|-------------|
| `personas` | []object | Persona files, each `{ file: <path> }` |
| `roles` | []object | Role-to-provider mapping, each `{ id: <role>, provider: <provider-id> }` |

```yaml
spec:
  self_play:
    personas:
      - file: personas/frustrated-customer.yaml
    roles:
      - id: user
        provider: gpt4
```

### `defaults`

Default execution settings applied to every scenario unless it overrides them.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `temperature` | number | - | Sampling temperature |
| `max_tokens` | integer | - | Maximum output tokens |
| `seed` | integer | - | Deterministic sampling seed |
| `concurrency` | integer | - | Parallel runs |
| `run_timeout` | string | "5m" | Per-run timeout (e.g. `"30s"`) |
| `fail_on` | []string | - | Conditions that mark the run failed |
| `output` | object | - | Output directory and formats |

```yaml
spec:
  defaults:
    temperature: 0.5
    max_tokens: 500
    seed: 42
    run_timeout: 2m
    output:
      dir: out
      formats:
        - json
```

### `globals`

Cross-cutting settings applied additively to every scenario.

| Field | Type | Description |
|-------|------|-------------|
| `conversation_assertions` | []object | Assertions appended to every scenario's own conversation assertions |

## Complete Example

A minimal bundle layout:

```
my-bundle/
├── config.arena.yaml
├── prompts/
│   └── assistant.yaml
├── providers/
│   └── gpt4.provider.yaml
└── scenarios/
    ├── billing.scenario.yaml
    └── support.scenario.yaml
```

`config.arena.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: customer-support-eval
spec:
  prompt_configs:
    - id: assistant
      file: prompts/assistant.yaml

  providers:
    - file: providers/gpt4.provider.yaml
      group: default

  scenarios:
    - file: scenarios/billing.scenario.yaml
    - file: scenarios/support.scenario.yaml

  defaults:
    temperature: 0.5
    max_tokens: 500
    run_timeout: 2m
    output:
      dir: out
      formats:
        - json
```

### Providers from CRDs

When you plan to supply providers from Provider/AgentRuntime CRDs via the ArenaJob, leave the `providers` list empty in the config file. The `group` names you would have used still apply — the ArenaJob's provider-group keys map onto them:

```yaml
spec:
  # Providers supplied by ArenaJob.spec.providers (resolved from CRDs)
  providers: []

  scenarios:
    - file: scenarios/billing.scenario.yaml
```

## Using the config file with an ArenaJob

An ArenaJob references the bundle via `sourceRef` and selects this file via `arenaFile`:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: eval-001
  namespace: arena
spec:
  sourceRef:
    name: customer-support-source
  arenaFile: config.arena.yaml
  providers:
    default:
      - providerRef:
          name: gpt4-prod
    judge:
      - providerRef:
          name: claude-opus
```

## Workflow

1. **Author the bundle** — write `config.arena.yaml` alongside your prompts and scenarios.
2. **Create an ArenaSource** — point it at the bundle (git, OCI, ConfigMap, or workspace).
3. **Create an ArenaJob** — reference the source and set `arenaFile`; optionally supply providers/tools from CRDs.

```
config.arena.yaml (in bundle)
        │
ArenaSource ──▶ ArenaJob ──▶ Workers ──▶ Results
```

## Related Resources

- **[ArenaSource](/reference/evaluation/arenasource/)**: Defines the bundle source that contains this config file
- **[ArenaJob](/reference/evaluation/arenajob/)**: Executes a run using this config file (`spec.arenaFile`)
- **[Provider](/reference/core/provider/)**: LLM provider configuration
- **[ToolRegistry](/reference/core/toolregistry/)**: Tool definitions
