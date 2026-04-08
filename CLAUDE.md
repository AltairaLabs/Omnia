# Omnia - Claude Code Project Instructions

## PromptKit SDK (`promptkit-local/`)

**`promptkit-local/` is READ-ONLY.** It is a local checkout of the [PromptKit repo](https://github.com/AltairaLabs/PromptKit) used via `go.work` for local development. You MUST NOT:

- Edit, write, or modify any file under `promptkit-local/`
- Create new files in `promptkit-local/`
- Run `go mod tidy` or any command that modifies files in `promptkit-local/`

If you find a bug or need a change in PromptKit, **do not fix it here**. Instead:
- Note the issue and tell the user to create an issue at https://github.com/AltairaLabs/PromptKit/issues
- Work around it in Omnia code if possible (e.g., string literal fallbacks for missing constants)

## Git Workflow

- **Never push directly to main** — main has branch protection enabled.
- Always use feature branches: `git checkout -b feature/<issue-number>-<short-description>` or `feat/<description>`.
- Standard flow: branch → commit → push with `-u` → create PR via `gh pr create` → monitor CI → merge via `gh pr merge --squash`.
- When continuing a previous session, check `git status`, `git log --oneline -5`, and any existing plan files before taking action.
- **Never manually resolve conflicts in generated files** (`zz_generated.deepcopy.go`, `go.sum`, `package-lock.json`, `dashboard/src/types/generated/*.ts`). After merging, re-run `make generate && make manifests && go mod tidy` (and `cd dashboard && npm install` for dashboard changes). The `.gitattributes` merge drivers will auto-accept "ours" for these files.

## Keeping Documentation In Sync

Architecture docs are only useful if they reflect reality. When your changes affect service boundaries, update the docs as part of the same PR — not as a follow-up.

**When to update `SERVICE.md`** (in the service's `cmd/` or `ee/cmd/` directory):
- Adding or removing an input/output (new API endpoint, new gRPC method, new message type)
- Changing what a service owns or does NOT own
- Adding or removing Prometheus metrics or OpenTelemetry trace spans
- Adding or changing dependencies (new external service, database, cache)

**When to update `SERVICES.md`** (repo root):
- Adding a new deployable service
- Changing how services communicate (new protocol, new connection between services)
- Adding or removing trace spans (update the span inventory table and trace flow diagram)

**When to update `api/CHANGELOG.md`**:
- Any change to REST, gRPC, or WebSocket message schemas

**When to update `api/websocket/asyncapi.yaml`**:
- Adding/removing/changing WebSocket message types or payload schemas

## Pre-commit Hooks

The repo has a pre-commit hook at `hack/pre-commit` that runs on every commit. **Run checks locally before committing to avoid retry cycles.**

### Go checks (runs when `.go` files are staged)
- `gofmt` / `goimports` formatting
- `golangci-lint run` (config in `.golangci.yml`) — includes: errcheck, gocognit (threshold 20), gocyclo (threshold 20), govet, unparam, revive, staticcheck, unused, and more
- `go test` with coverage — **per-file coverage threshold is 80%** on changed files
- `go vet`
- Generated code check (`make generate`, `make manifests`)

### Dashboard checks (runs when `dashboard/**/*.{ts,tsx,js,jsx}` files are staged)
- ESLint (config in `dashboard/eslint.config.mjs`) — includes sonarjs rules; **no nested ternaries** (`sonarjs/no-nested-conditional`)
- TypeScript type checking (`npm run typecheck`)
- Vitest tests with coverage — **per-file coverage threshold is 80%** on changed files
- Cognitive complexity warnings at 25 (sonarjs)

### Before committing
1. For Go changes: run `golangci-lint run ./...` and `go test ./... -count=1` first
2. For dashboard changes: run `cd dashboard && npm run lint && npm run typecheck && npx vitest run --coverage` first
3. Fix ALL failures before attempting `git commit`

## Architecture Overview

### Service Topology
```
                    ┌──────────────┐
                    │   Dashboard  │  Next.js app (dashboard/)
                    │   (UI)       │  Proxies to operator API
                    └──────┬───────┘
                           │ HTTP
                    ┌──────┴───────┐
                    │   Operator   │  cmd/main.go — K8s controller-manager
                    │              │  Serves dashboard + REST API
                    └──────┬───────┘
                           │ Manages
            ┌──────────────┼──────────────┐
            │              │              │
     ┌──────┴───────┐ ┌───┴────┐  ┌──────┴───────┐
     │  Facade      │ │Runtime │  │ Session API  │
     │  (WebSocket) │ │(gRPC)  │  │ (HTTP)       │
     │  cmd/agent/  │ │cmd/    │  │ cmd/         │
     │  main.go     │ │runtime/│  │ session-api/ │
     └──────┬───────┘ └───┬────┘  └──────┬───────┘
            │    gRPC      │              │
            └──────────────┘              │
                    │                     │
                    │ writes via          │ reads/writes
                    │ httpclient          │ direct
                    └─────────┬───────────┘
                              │
                       ┌──────┴───────┐
                       │  PostgreSQL  │  Session storage
                       │  (+ Redis)   │  (warm cache)
                       └──────────────┘
```

- **Facade container** (`cmd/agent/main.go`): WebSocket server that accepts browser connections. Each agent pod runs a facade + runtime as sidecars.
- **Runtime container** (`cmd/runtime/main.go`): gRPC server that wraps the LLM provider (via PromptKit SDK). Called by facade over gRPC.
- **Session API** (`cmd/session-api/`): Standalone HTTP service for session CRUD. The facade writes session data via an HTTP client (`internal/session/httpclient/`), NOT directly to Postgres.
- **Operator** (`cmd/main.go`): Kubernetes controller-manager. Reconciles AgentRuntime, PromptPack, ToolRegistry, Provider, Workspace, and SessionRetentionPolicy CRDs. Also serves the dashboard and REST API.
- **Dashboard** (`dashboard/`): Next.js frontend embedded in the operator binary via `dashboard/server.js`. Reads session data from session-api through proxy routes.

### Key data flow for sessions
Browser → WebSocket → Facade → `internal/session/httpclient` → Session API HTTP → PostgreSQL.
Redis is a **warm cache within session-api**, not a separate path.

### Tool execution model
Tools have different execution locations, which determines their visibility in the WebSocket stream:

- **Client tools** (`client://browser` endpoint): Defined in ToolRegistry CRDs with a client-side executor. The facade forwards these via WebSocket `tool_call` messages — the browser executes them and returns results. These are the **only** tools visible in the WS stream.
- **Server tools** (HTTP, MCP, and other executor types in ToolRegistry): Executed by the runtime. NOT forwarded via WebSocket.
- **Platform tools** (memory, workflow, etc.): Registered via PromptKit SDK capabilities (`sdk.WithMemory()`). Executed server-side in the runtime. NOT forwarded via WebSocket.

When testing or debugging: only client tools appear in WebSocket messages. All other tools must be verified via runtime logs or session-api `/tool-calls` endpoint.

### Enterprise code
Enterprise features live under `ee/`. This includes Arena (prompt testing/evaluation), ArenaJob controller, and ArenaDevSession controller.

## Project Structure

| Path | Purpose |
|------|---------|
| `api/v1alpha1/` | CRD type definitions (Go structs with kubebuilder markers) |
| `cmd/main.go` | Operator binary entrypoint |
| `cmd/agent/main.go` | Facade (agent) binary entrypoint |
| `cmd/runtime/main.go` | Runtime binary entrypoint |
| `cmd/session-api/` | Session API binary |
| `internal/controller/` | Kubernetes controllers (reconcilers) |
| `internal/facade/` | WebSocket server, connection handling, session recording |
| `internal/runtime/` | gRPC runtime server, tool execution, LLM streaming |
| `internal/tracing/` | Shared OpenTelemetry tracing (used by both facade and runtime) |
| `internal/session/` | Session store interfaces and implementations |
| `internal/session/api/` | Session API HTTP handlers |
| `internal/session/httpclient/` | HTTP client for session-api |
| `internal/session/postgres/` | PostgreSQL session storage + migrations |
| `internal/api/` | Operator REST API handlers |
| `dashboard/` | Next.js dashboard application |
| `dashboard/src/types/` | Hand-written TypeScript types (source of truth) |
| `dashboard/src/types/generated/` | Auto-generated TS types from CRDs (reference only) |
| `dashboard/src/lib/data/` | Data services (operator-service, session-api-service, live-service) |
| `charts/omnia/` | Helm chart |
| `ee/` | Enterprise edition code (Arena) |
| `docs/` | Starlight documentation site |
| `test/e2e/` | End-to-end tests (kind cluster) |
| `hack/` | Scripts (pre-commit, validation, etc.) |

## Build & Test Commands

```bash
# Go
go build ./...                                    # Build all packages
go test ./... -count=1                            # Run all tests
go test ./internal/controller/... -count=1 -v     # Controller tests (envtest/ginkgo)
go test ./internal/facade/... -count=1 -v         # Facade tests
go test ./internal/runtime/... -count=1 -v        # Runtime tests
golangci-lint run ./...                           # Full lint check

# Dashboard
cd dashboard && npm run lint                      # ESLint
cd dashboard && npm run typecheck                 # TypeScript check
cd dashboard && npx vitest run --coverage         # Tests with coverage
cd dashboard && npx next build                    # Production build

# Code generation
make generate                                     # Deepcopy generation
make generate-dashboard-types                     # CRD → YAML → TS types (chains manifests → sync-chart-crds → generate)
make manifests                                    # Regenerate CRD YAML + RBAC

# E2E
make test-e2e                                     # Full E2E suite (creates kind cluster)
```

### Running E2E Tests Locally

**Always debug E2E failures locally** — never push to CI and wait for logs. Each CI cycle is ~15 min; local runs give immediate `kubectl logs` access.

```bash
# Core E2E (non-arena tests)
kind create cluster --name omnia-test-e2e --wait 60s
env GOWORK=off KIND_CLUSTER=omnia-test-e2e E2E_SKIP_CLEANUP=true \
  go test -tags=e2e ./test/e2e/ -v -ginkgo.v \
  -ginkgo.label-filter='!arena' -timeout 20m

# Arena E2E (enterprise features)
env GOWORK=off ./scripts/setup-arena-e2e.sh
kubectl config use-context kind-omnia-arena-e2e
env GOWORK=off E2E_SKIP_SETUP=true E2E_PREDEPLOYED=true ENABLE_ARENA_E2E=true \
  E2E_SKIP_CLEANUP=true go test -tags=e2e ./test/e2e/ -v -ginkgo.v \
  -ginkgo.label-filter=arena -timeout 30m

# After failure, inspect pods directly:
kubectl logs -n test-agents <pod> --all-containers
kubectl get pods -n test-agents -o wide
```

**Critical gotchas:**
- `GOWORK=off` — required; `promptkit-local` has type mismatches with the published SDK
- `KIND_CLUSTER=omnia-test-e2e` — the test defaults to `"kind"`, mismatch = "no nodes found"
- `E2E_SKIP_CLEANUP=true` — leaves resources in cluster for debugging
- **Never use `--ginkgo.focus` on specs inside Ordered containers** — it skips the `BeforeAll` that deploys the operator, so the focused spec fails because nothing is deployed
- The cluster must exist before `go test` — `BeforeSuite` doesn't create it

## SonarCloud Quality Gate (CI)

SonarCloud runs on every PR and enforces the **Sonar Way** quality profile. The quality gate checks **new code only** (changes in the PR):

| Metric | Threshold | What it means |
|--------|-----------|---------------|
| Coverage | >= 80% | New/changed lines must be tested |
| Duplicated lines | <= 3% | Avoid copy-paste code |
| Reliability rating | A | No new bugs |
| Security rating | A | No new vulnerabilities |
| Maintainability rating | A | No new code smells (includes cognitive complexity) |
| Security hotspots reviewed | 100% | All hotspots must be triaged |

**Cognitive complexity** is the most common CI failure. SonarCloud uses a threshold of **15** (rule `go:S3776` / `typescript:S3776`). This is stricter than the local golangci-lint threshold of 20. Functions above 15 will create code smell issues that can downgrade the maintainability rating and fail the quality gate.

Exceptions configured in `sonar-project.properties` — cognitive complexity is ignored for:
- `cmd/**/main.go` — entry points
- `internal/controller/**` — K8s controllers (reconciliation logic)
- `internal/api/**` — API handlers
- `ee/cmd/**`, `ee/internal/controller/**` — enterprise code

**Duplicated string literals** (`go:S1192`): SonarCloud flags strings duplicated 3+ times. Extract to constants.

## Go Code Standards

- **Cognitive complexity**: Keep functions below **15** (SonarCloud threshold). Local golangci-lint allows 20, but SonarCloud will fail the PR at 15. Proactively extract helper functions.
- **Cyclomatic complexity**: Keep below 20 (golangci-lint gocyclo threshold).
- **Test coverage**: All changed files must have >= 80% coverage. Write tests for error paths and edge cases, not just happy paths.
- **Duplicated strings**: Extract string literals used 3+ times into constants (SonarCloud `go:S1192`).
- **Naming**: Follow Go conventions. Test doubles should use `Mock` prefix (e.g., `MockStore`).
- **Formatting**: `gofmt` and `goimports` are enforced. Run before committing.
- **Never exclude packages from golangci-lint** to work around build problems. If a package doesn't compile with `GOWORK=off`, fix the dependency — don't suppress the lint. All Go code in the repo must lint cleanly.

### PromptKit SDK Version Strategy

Local development uses `go.work` with `promptkit-local/`, but CI uses the published module (`GOWORK=off`). To avoid CI-only failures:

- **Rule**: Code in `internal/runtime/` must compile with the **published** SDK. If you need a new PromptKit type or event, the PromptKit release must happen first.
- **Guard**: CI runs `GOWORK=off go build ./...` — this catches unpublished SDK dependencies.
- **Pattern for unreleased types**: Use the string literal form with a TODO comment:
  ```go
  events.EventType("tool.client.resolved") // TODO: use events.EventClientToolResolved when published
  ```
- **Never** add types/constants from `promptkit-local/` that don't exist in the published SDK without this pattern.

## Structured Logging

All Go code uses **structured logging** via `logr.Logger` backed by Zap (`pkg/logging/`). Production emits JSON; development emits human-readable output.

**Rules:**
- **Message**: Short, stable event name — NOT a prose sentence. Think grep-able identifier. Examples: `"stream starting"`, `"eval options built"`, `"event bridge skipped"`.
- **Context**: ALL variable data goes in key-value pairs, never interpolated into the message string. Conditions and reasons go in keys like `"reason"`, not in the message.
- **Levels**: `V(0)` = info (default), `V(1)` = debug (enabled via `LOG_LEVEL=debug`), `V(2)` = trace. Use `V(1)` for diagnostic/pipeline visibility. Use `.Error(err, ...)` for errors.
- **Keys**: Use camelCase (`"evalDefCount"`, `"sessionID"`, `"hasMetrics"`). Boolean keys use `has` prefix (`"hasEvalCollector"`).

```go
// GOOD — structured
log.V(1).Info("eval options built",
    "evalDefCount", len(defs),
    "registeredTypes", registry.Types())

log.V(1).Info("event bridge skipped",
    "reason", "disabled",
    "eventType", event.Type)

// BAD — prose message with context baked in
log.V(1).Info("eval options skipped: no eval collector configured")
log.V(1).Info("forwarding event to session-api successfully")
```

## Dashboard Code Standards

- **No nested ternaries** — ESLint `sonarjs/no-nested-conditional` is an error. Extract to variables or if/else.
- **Cognitive complexity**: Keep below **15** (SonarCloud `typescript:S3776`). The ESLint sonarjs plugin warns at 25, but SonarCloud will fail the PR at 15. Extract helper functions for complex logic.
- **Test coverage**: Per-file threshold is 80% for staged files. Write tests in `*.test.ts` files alongside the source.
- **Duplicated lines**: Keep below 3% on new code. Avoid copy-paste patterns.
- **Type safety**: `npm run typecheck` must pass. Use proper TypeScript types, not `any`.

## Testing Standards

### Mock-to-contract, not mock-to-code

When mocking API responses (Go→TS, service→service), the mock's response shape MUST match the **real API response struct**, not what the calling code expects. If the code has a bug reading `data.evalResults` but the API returns `data.results`, the test must use `{ results: [...] }` — catching the bug immediately.

**Rules:**
- Always check the Go response struct's `json:"..."` tags when writing a TS mock for a proxy or API call
- For proxy routes that pass through backend responses, mock the backend's actual response shape
- When writing a new API endpoint, verify the consumer (dashboard service, test mock) uses the correct JSON field names

### Tiered/fallback system tests

For any system with tiered fallback (hot → warm → cold, cache → database → archive), test **all degraded states**, not just hit/miss:

| Scenario | What to test |
|----------|-------------|
| Cache hit | Returns cached data |
| Cache miss (not found) | Falls through to next tier |
| Cache empty (found but no data) | Falls through to next tier — empty is not the same as "definitive answer" for caches |
| Cache error (non-NotFound) | Falls through and logs error |
| All tiers miss | Returns appropriate not-found error |

The most dangerous case is **"cache empty"** — the cache returns success with zero results, which the service trusts as definitive, never checking the authoritative store. Always validate that an empty cache result falls through.

### Wiring tests (service startup verification)

Unit tests verify logic works. **Wiring tests** verify it's actually connected. Every service binary (`cmd/*/main.go`) should have a test that starts the real server and verifies cross-service contracts are met:

- **Interceptors registered**: If a gRPC interceptor exists, test that the server has it installed (e.g., send metadata via gRPC, verify the handler receives it in context).
- **Middleware active**: If middleware is wired conditionally (enterprise flags, env vars), test both enabled and disabled paths.
- **Dependencies injected**: If a service depends on an interface implementation (store, deleter, logger), test that the real implementation is injected, not nil.

**Why:** Code that exists, has passing unit tests, but isn't wired into the binary is the most common failure mode in this project. Unit tests for interceptors, middleware, and handlers all pass — but if `cmd/*/main.go` doesn't register them, the feature silently doesn't work. This is only caught by smoke tests in-cluster, which are slow and hard to debug.

**Pattern:** One test per service binary that creates the server with real options and asserts the contract (e.g., "gRPC metadata arrives in handler context", "POST without user_id returns 400", "enterprise middleware is active when flag is set").

See: https://github.com/AltairaLabs/Omnia/issues/714 (wiring test backlog)

### Test coverage requirements

- **Happy path**: Basic functionality works
- **Error paths**: All error branches are exercised (not just logged-and-swallowed)
- **Degraded states**: Partial data, expired caches, split-key TTL expiry
- **Contract alignment**: Mock response shapes match real API structs
- **Wiring**: Service binaries register all interceptors, middleware, and dependencies

## CRD / Kubernetes Patterns

- CEL validation: For optional string fields with `omitempty`, always use `has(self.fieldName)` guard before accessing the value. Direct access fails with "no such key" when the field is omitted.
- After changing CRD types in `api/v1alpha1/`, run `make generate` (deepcopy) then `make manifests` (CRD YAML + RBAC).
- If CRD changes affect the dashboard, also run `make generate-dashboard-types`.

## Tiltfile / Local Development

- The Tiltfile controls local development via Tilt.
- Docker image `only` lists control build context — if you move or add Go packages, ensure they're listed in the relevant `docker_build` `only` array.
- Three images: operator, facade (agent), runtime. Each has its own `only` list.
