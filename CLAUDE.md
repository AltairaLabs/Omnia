# Omnia - Claude Code Project Instructions

## Git Workflow

- **Never push directly to main** — main has branch protection enabled.
- Always use feature branches: `git checkout -b feature/<issue-number>-<short-description>` or `feat/<description>`.
- Standard flow: branch → commit → push with `-u` → create PR via `gh pr create` → monitor CI → merge via `gh pr merge --squash`.
- When continuing a previous session, check `git status`, `git log --oneline -5`, and any existing plan files before taking action.
- **Never manually resolve conflicts in generated files** (`zz_generated.deepcopy.go`, `go.sum`, `package-lock.json`, `dashboard/src/types/generated/*.ts`). After merging, re-run `make generate && make manifests && go mod tidy` (and `cd dashboard && npm install` for dashboard changes). The `.gitattributes` merge drivers will auto-accept "ours" for these files.

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
- Runtime and runtime test packages (`internal/runtime/`, `cmd/runtime/`) are excluded from golangci-lint because they depend on the unpublished PromptKit SDK.

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

### Test coverage requirements

- **Happy path**: Basic functionality works
- **Error paths**: All error branches are exercised (not just logged-and-swallowed)
- **Degraded states**: Partial data, expired caches, split-key TTL expiry
- **Contract alignment**: Mock response shapes match real API structs

## CRD / Kubernetes Patterns

- CEL validation: For optional string fields with `omitempty`, always use `has(self.fieldName)` guard before accessing the value. Direct access fails with "no such key" when the field is omitted.
- After changing CRD types in `api/v1alpha1/`, run `make generate` (deepcopy) then `make manifests` (CRD YAML + RBAC).
- If CRD changes affect the dashboard, also run `make generate-dashboard-types`.

## Tiltfile / Local Development

- The Tiltfile controls local development via Tilt.
- Docker image `only` lists control build context — if you move or add Go packages, ensure they're listed in the relevant `docker_build` `only` array.
- Three images: operator, facade (agent), runtime. Each has its own `only` list.
