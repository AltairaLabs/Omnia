# Arena Worker Service (Enterprise)

## Owns
- Executing arena evaluation work items (scenario × provider combinations)
- Two execution modes:
  - **Direct mode**: calls LLM providers directly via PromptKit SDK
  - **Fleet mode**: connects to a deployed agent via WebSocket (black-box testing)
- Provider credential resolution (binding arena config providers to K8s Provider CRDs)
- PromptKit engine lifecycle (build, plan, execute, close)
- Result reporting back to Redis work queue

## Inputs
- **Redis Streams**: work items from the arena controller (`queue.WorkItem`)
  - Direct mode items have `ScenarioID` + `ProviderID` (one provider per item)
  - Fleet mode items have `ScenarioID` only (agent handles its own provider)
- **Filesystem**: arena project content mounted from workspace PVC at `ContentPath`
  - Arena config YAML (`config.arena.yaml`)
  - Scenario files, persona files, provider YAML files
- **ConfigMap mount**: override configuration at `OverridesPath` (`overrides.json`)
  - Provider overrides (from `spec.providerOverrides` on ArenaJob)
  - Provider bindings (annotation-based credential resolution from Provider CRDs)
  - Tool overrides (from `spec.toolRegistryOverride` on ArenaJob)
- **Environment variables**: provider credentials (secrets injected by the controller)
- **Environment variables**: execution config (`ARENA_EXECUTION_MODE`, `ARENA_FLEET_WS_URL`)

## Outputs
- **Redis Streams**: work item status updates (pass/fail, duration, metrics, assertions)
- **Filesystem**: evaluation output (JUnit XML, JSON reports) written to `/tmp/arena-output`
- **OTel traces**: spans for work item execution, fleet session links

## Does NOT Own
- Work item creation or partitioning (Arena Controller's job)
- Provider CRD resolution or credential extraction (Arena Controller's job)
- Agent runtime management (Operator's job)
- Session storage (Session API's job)

## Architecture

### Execution Flow

```
                        ┌────────────────────────────────────────┐
                        │           Arena Worker Pod              │
                        │                                        │
  Redis ──work item──▶  │  1. loadConfig() from env vars         │
                        │  2. Load arena config from filesystem   │
                        │  3. Apply overrides from ConfigMap:     │
                        │     a. Provider overrides (CRD-based)   │
                        │     b. Provider bindings (annotations)  │
                        │     c. Tool overrides                   │
                        │  4. BuildEngineComponents (PromptKit)   │
                        │  5. [Fleet] Inject fleet-agent provider │
                        │  6. GenerateRunPlan (filtered)          │
                        │  7. ExecuteRuns                         │
                        │  8. Report results                      │
  Redis ◀──results───  │                                        │
                        └────────────────────────────────────────┘
```

### Provider Resolution Pipeline

Both modes share the same pipeline for resolving provider credentials.
The controller prepares everything; the worker applies it.

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

### Direct vs Fleet Mode

| Aspect | Direct Mode | Fleet Mode |
|--------|-------------|------------|
| Work items | scenario × provider matrix | scenario only (no provider dimension) |
| Primary provider | from `item.ProviderID` | synthetic `fleet-agent` |
| LLM calls | worker → LLM API directly | worker → agent WebSocket → agent's LLM |
| Provider filter | `[item.ProviderID]` | `["fleet-agent"]` |
| Credential validation | yes (`ValidateProviderCredentials`) | skipped (agent handles credentials) |
| Provider resolution | same pipeline | same pipeline + fleet-agent injection |
| Self-play providers | resolved via bindings | resolved via bindings (same as direct) |

### Fleet Mode Details

Fleet mode adds a `fleet.Provider` that connects to a deployed agent via WebSocket:

1. `fleet.NewProvider("fleet-agent", wsURL)` — creates provider
2. `fleetProvider.Connect(ctx)` — dials WebSocket, gets session ID
3. `providerRegistry.Register(fleetProvider)` — adds to runtime registry
4. `injectFleetProviderConfig(arenaCfg)` — adds `fleet-agent` to `LoadedProviders`
5. `providerFilter = ["fleet-agent"]` — run plan only generates fleet-agent combinations

The fleet provider is injected AFTER `BuildEngineComponents` because the PromptKit SDK
doesn't know the `"fleet"` type and would reject it during component building.

### Self-Play

Self-play scenarios use multiple providers within a single run:
- The **assistant** role uses the primary provider (direct: specified provider, fleet: `fleet-agent`)
- The **user simulation** role uses a separate provider referenced by ID in `self_play.roles[].provider`

Self-play providers are resolved through the same binding pipeline as all other providers.
They must exist in `LoadedProviders` and the `providerRegistry` after step 4 of the
worker-side resolution.

## Observability

**Traces** (OpenTelemetry):
- `arena.worker.execute` — per work item
- `arena.fleet.session` — links arena trace to agent session trace (fleet mode)

## Dependencies
- PromptKit SDK (`engine`, `config`, `providers` packages)
- Redis (work queue via `ee/pkg/arena/queue`)
- Fleet provider (`ee/pkg/arena/fleet`)
- Binding resolution (`ee/pkg/arena/binding`)
- Override types (`ee/pkg/arena/overrides`)
