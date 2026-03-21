# Arena Worker Service (Enterprise)

## Owns
- Executing arena evaluation work items (scenario × provider combinations)
- Provider resolution — two paths:
  - **CRD-based** (new): reads Provider/AgentRuntime CRDs directly from `spec.providers` groups
  - **Override/binding** (legacy): applies ConfigMap overrides and annotation-based bindings
- PromptKit engine lifecycle (build, plan, execute, close)
- Result reporting back to Redis work queue

## Inputs
- **Redis Streams**: work items from the arena controller (`queue.WorkItem`)
  - Items have `ScenarioID` + `ProviderID` (one provider per item in the scenario × provider matrix)
  - For CRD-resolved agents, `ProviderID` is `"agent-{name}"`
- **Filesystem**: arena project content mounted from workspace PVC at `ContentPath`
  - Arena config YAML (`config.arena.yaml`)
  - Scenario files, persona files, provider YAML files (skipped when CRD providers are used)
- **K8s API** (CRD path only): reads Provider, AgentRuntime, ToolRegistry, and ArenaJob CRDs
- **ConfigMap mount** (legacy path): override configuration at `OverridesPath` (`overrides.json`)
  - Provider overrides (from `spec.providerOverrides` on ArenaJob)
  - Provider bindings (annotation-based credential resolution from Provider CRDs)
  - Tool overrides (from `spec.toolRegistryOverride` on ArenaJob)
- **Environment variables**: provider credentials (secrets injected by the controller)
- **Environment variables**: execution config (see table below)

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `ARENA_JOB_NAME` | yes | — | Job identifier |
| `ARENA_CONTENT_PATH` | yes | — | Mount point for arena bundle |
| `ARENA_JOB_NAMESPACE` | no | — | K8s namespace |
| `ARENA_CONFIG_FILE` | no | auto-detect | Arena config filename in bundle |
| `ARENA_OVERRIDES_PATH` | no | — | Path to mounted overrides.json (legacy) |
| `ARENA_PROVIDER_GROUPS` | no | — | `"true"` to enable CRD-based provider resolution |
| `ARENA_AGENT_WS_URLS` | no | — | JSON map of agent name → WebSocket URL (CRD path) |
| `ARENA_EXECUTION_MODE` | no | `"direct"` | `"fleet"` for legacy fleet mode |
| `ARENA_FLEET_WS_URL` | fleet only | — | WebSocket URL for legacy fleet mode |
| `ARENA_VERBOSE` | no | — | `"true"` for debug logging |
| `REDIS_ADDR` | no | `redis:6379` | Redis address |
| `REDIS_PASSWORD` | no | — | Redis password |
| `SESSION_API_URL` | no | — | Session-api URL for recording arena sessions (opt-in) |
| `TRACING_ENABLED` | no | — | `"true"` to enable OTel tracing |
| `TRACING_ENDPOINT` | no | — | OTLP gRPC endpoint |

## Outputs
- **Redis Streams**: work item status updates (pass/fail, duration, metrics, assertions)
- **Filesystem**: evaluation output (JUnit XML, JSON reports) written to `/tmp/arena-output`
- **OTel traces**: spans for work item execution, fleet session links
- **HTTP** to Session API (optional): session creation, provider call recording, tool call recording via OmniaEventStore

## Does NOT Own
- Work item creation or partitioning (Arena Controller's job)
- Agent runtime management (Operator's job)
- Session storage (Session API's job)

## Architecture

### Execution Flow

There are two provider resolution paths. The worker selects based on `ARENA_PROVIDER_GROUPS`:

```
                        ┌────────────────────────────────────────────┐
                        │           Arena Worker Pod                  │
                        │                                            │
  Redis ──work item──▶  │  1. loadConfig() from env vars             │
                        │  2. Load arena config from filesystem       │
                        │                                            │
                        │  ┌─ CRD path (ARENA_PROVIDER_GROUPS=true) │
                        │  │  3a. Read ArenaJob CRD via K8s API      │
                        │  │  3b. Resolve Provider CRDs → config     │
                        │  │  3c. Connect fleet providers (agentRef) │
                        │  │  3d. Resolve ToolRegistry CRDs          │
                        │  │                                         │
                        │  ├─ Legacy path                            │
                        │  │  3a. Load overrides from ConfigMap      │
                        │  │  3b. Apply provider bindings            │
                        │  │  3c. Apply tool overrides               │
                        │  │  3d. [Fleet] Connect fleet-agent        │
                        │  └─────────────────────────────────────────│
                        │                                            │
                        │  4. BuildEngineComponents (PromptKit)      │
                        │  5. Register fleet providers               │
                        │  6. GenerateRunPlan (filtered)             │
                        │  7. ExecuteRuns                            │
                        │  8. Report results                         │
  Redis ◀──results───  │                                            │
                        └────────────────────────────────────────────┘
```

### CRD-Based Provider Resolution (New)

When `ARENA_PROVIDER_GROUPS=true`, the worker reads providers directly from CRDs.
No ConfigMap, no binding pipeline, no label selectors.

**Controller side** (`arenajob_controller.go`):
1. `resolveProviderGroups()` — fetches Provider/AgentRuntime CRDs from `spec.providers` refs
2. `buildProviderEnvVarsFromCRDs()` — injects credential secrets as env vars
3. `buildProviderGroupEnvVars()` — encodes agent WebSocket URLs as `ARENA_AGENT_WS_URLS` JSON
4. Sets `ARENA_PROVIDER_GROUPS=true` to signal the new path

**Worker side** (`provider_groups.go`):
1. `getArenaJob()` — reads ArenaJob CRD via unstructured K8s client
2. For each `providerRef` entry:
   - `resolveProviderRefEntry()` — fetches Provider CRD, builds `config.Provider` with credential env var
3. For each `agentRef` entry:
   - `resolveAgentRefEntry()` — creates `fleet.Provider`, connects via WebSocket URL from `ARENA_AGENT_WS_URLS`
4. `resolveToolsFromCRD()` — fetches ToolRegistry CRDs, extracts discovered tools as overrides
5. Fleet providers registered AFTER `BuildEngineComponents`, BEFORE `NewEngine`

**Key**: agents and LLM providers are interchangeable in the scenario × provider matrix.
There is no separate "fleet mode" — a single agent is just a 1-provider matrix.

### Legacy Provider Resolution Pipeline

When `ARENA_PROVIDER_GROUPS` is not set, the worker uses the ConfigMap-based override pipeline.

**Controller side** (`arenajob_controller.go`):
1. `resolveProviderOverrides()` — matches Provider CRDs via label selectors from `spec.providerOverrides`
2. `resolveBindingRegistry()` — lists ALL Provider CRDs in the namespace
3. `buildOverrideConfig()` — creates ConfigMap with provider overrides + bindings
4. `buildProviderEnvVarsFromCRDs()` — injects credential secrets as env vars

**Worker side** (`worker.go`):
1. `loadOverrides()` — reads the ConfigMap JSON
2. `applyOverridesFromConfig()` → `applyProviderOverrides()` — adds CRD providers to `LoadedProviders`
3. `applyProviderBindings()` — matches arena config provider IDs to CRD names, injects credentials:
   - `binding.ApplyBindings()` — annotation-based matching (`omnia.altairalabs.ai/provider-name`)
   - `binding.ApplyNameMatching()` — fallback name matching (`{namespace}-{name}` format)
4. `BuildEngineComponents()` (PromptKit SDK) — creates provider instances from `LoadedProviders`

### Legacy Direct vs Fleet Mode

| Aspect | Direct Mode | Fleet Mode |
|--------|-------------|------------|
| Work items | scenario × provider matrix | scenario only (no provider dimension) |
| Primary provider | from `item.ProviderID` | synthetic `fleet-agent` |
| LLM calls | worker → LLM API directly | worker → agent WebSocket → agent's LLM |
| Provider filter | `[item.ProviderID]` | `["fleet-agent"]` |
| Credential validation | yes (`ValidateProviderCredentials`) | skipped (agent handles credentials) |

### Self-Play

Self-play scenarios use multiple providers within a single run:
- The **assistant** role uses the primary provider
- The **user simulation** role uses a separate provider referenced by ID in `self_play.roles[].provider`

With CRD-based resolution, self-play "just works" — all providers (including agents) are in
`LoadedProviders`, and the engine resolves role providers by ID. An `agentRef` entry can serve
as the assistant while a `providerRef` serves as user-simulator, or vice versa.

## Testing

### Unit tests (no infrastructure needed)
```bash
go test ./ee/cmd/arena-worker/... -count=1 -v
```
Covers: config loading, tool/provider override application, CRD resolution (fake k8s client),
sanitization, credential resolution.

### Integration tests (no infrastructure needed)
```bash
go test -tags=integration ./ee/cmd/arena-worker/... -count=1 -v
```
Creates temp directories with full arena bundles and calls `executeWorkItem()` directly.
Uses PromptKit's `mock` provider — no Redis, no K8s, no external LLMs.

The `TestExecuteWorkItemWithProviderGroups` test exercises the CRD path end-to-end:
fake k8s client with Provider CRD + unstructured ArenaJob → engine → mock execution → pass.

### Key test utilities
- `queue.NewMemoryQueueWithDefaults()` — in-memory Redis replacement
- `fleet.NewProvider()` with mock `Dialer` — fleet without WebSocket server
- `fake.NewClientBuilder().WithScheme(k8s.Scheme())` — fake k8s client for CRD tests
- `Config.K8sClient` — injectable client for testing (avoids in-cluster requirement)

## Observability

**Traces** (OpenTelemetry):
- `arena.worker` — root span for worker lifecycle
- `arena.work-item` — per work item execution
- `arena.engine.execute` — engine run
- `arena.fleet.session` — links arena trace to agent session trace

## Dependencies
- PromptKit SDK (`engine`, `config`, `providers` packages)
- Redis (work queue via `ee/pkg/arena/queue`)
- K8s API (CRD path: Provider, AgentRuntime, ToolRegistry, ArenaJob reads via `pkg/k8s`)
- Fleet provider (`ee/pkg/arena/fleet`)
- Binding resolution (`ee/pkg/arena/binding`) — legacy path only
- Override types (`ee/pkg/arena/overrides`) — legacy path only
