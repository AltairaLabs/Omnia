# ToolRegistry retry policies — CRD redesign

**Date**: 2026-04-11
**Status**: Approved design, awaiting implementation
**Related issue**: precursor to #779 (forward parsed tool config fields to HTTP/MCP executors)
**Author**: chaholl + Claude (brainstorming session)

## Summary

Restructure the `ToolRegistry` CRD so retry policies are transport-specific instead of top-level, and replace the current shallow `Retries *int32` field with richer `HTTPRetryPolicy`, `GRPCRetryPolicy`, and `MCPRetryPolicy` types. This is a **pure shape refactor** — the PR does not implement retry behavior, it only corrects the CRD so subsequent work can wire retries cleanly.

## Problem

`ToolRegistry` currently declares two transport-agnostic knobs at the top of `HandlerDefinition`:

```go
type HandlerDefinition struct {
    // ...
    Timeout *string `json:"timeout,omitempty"`
    Retries *int32  `json:"retries,omitempty"`
}
```

Both are propagated through the operator's `buildHandlerEntry` into the runtime's `HandlerEntry`, but neither is wired to any executor behavior. The current shape has three problems:

1. **Retry semantics vary by transport, but the field doesn't.** An `int32` retry count means fundamentally different things for HTTP (retry on 5xx/network errors), gRPC (retry on UNAVAILABLE/DEADLINE_EXCEEDED), MCP (retry a CallTool, optionally reconnecting the session first), and Client (meaningless — the browser owns retry). There is no single retry count that serves all transports, even if each transport had retry logic.

2. **The shape is too shallow for HTTP alone.** Even a well-designed HTTP retry policy needs to specify which status codes trigger retry, whether to retry on network errors, backoff strategy, and whether to honor `Retry-After` headers. `*int32` expresses none of that. Wiring the current field would force us to pick one interpretation and hardcode the rest, which is a footgun for anyone whose real retry semantics differ.

3. **It's silently unused.** The CRD accepts `retries: 3`, the controller happily serializes it to `tools.yaml`, and the runtime drops it on the floor. Users setting this field experience a no-op that passes CRD validation.

`Timeout` is genuinely generic (wall-clock budget applies uniformly across transports), and staying top-level is defensible — but its `*string` type relies on the controller to parse at reconcile time, so typos like `"30seconds"` surface late rather than at `kubectl apply`.

## Scope

This PR:
1. Removes `HandlerDefinition.Retries`.
2. Changes `HandlerDefinition.Timeout` from `*string` to `*metav1.Duration` (validated at admission time).
3. Adds `HTTPRetryPolicy`, `GRPCRetryPolicy`, `MCPRetryPolicy` as new Go types in `api/v1alpha1/toolregistry_types.go`.
4. Adds `RetryPolicy *HTTPRetryPolicy` to `HTTPConfig` and `OpenAPIConfig`, `RetryPolicy *GRPCRetryPolicy` to `GRPCConfig`, `RetryPolicy *MCPRetryPolicy` to `MCPConfig`.
5. Updates `internal/runtime/tools/config.go` to mirror the new shape with `Runtime*RetryPolicy` structs using parsed Go types.
6. Updates `internal/controller/tools_config.go` to translate CRD → runtime with validating builders and error propagation.
7. Adds a `RetryPolicyInvalid` status condition on `ToolRegistry` when translation fails.
8. Adds comprehensive tests at every layer (CRD validation, struct marshalling, builder functions, controller status, runtime loader, wiring).

This PR does **NOT**:
- Implement retry behavior. No retry loops. No backoff. No executor changes beyond field plumbing that makes the runtime compile.
- Wire field forwarding to PromptKit (that's issue #779, which follows this PR).
- Fix MCP session reconnect on broken transport (separate runtime issue, to be filed).
- Touch the circuit breaker (#778 is the unification work with retries, which follows both this PR and #779).

## Architecture

```
┌─────────────────────────────────────────┐
│ ToolRegistry CR                          │
│   handlers[]:                            │
│     - name: foo                          │
│       type: http                         │
│       timeout: 30s                       │  ← top-level, generic
│       httpConfig:                        │
│         endpoint: ...                    │
│         retryPolicy:                     │  ← transport-specific
│           maxAttempts: 3                 │
│           retryOn: [502, 503]            │
└──────────────────┬──────────────────────┘
                   │ reconcile
                   ▼
┌─────────────────────────────────────────┐
│ internal/controller/tools_config.go     │
│   buildHandlerEntry()                    │
│   buildHTTPRetryPolicy()   ─── validate, apply defaults, return error on bad config
│   buildGRPCRetryPolicy()                 │
│   buildMCPRetryPolicy()                  │
└──────────────────┬──────────────────────┘
                   │ serialize
                   ▼
┌─────────────────────────────────────────┐
│ tools.yaml ConfigMap                     │
│   (operator→runtime contract, both       │
│    sides controlled, uses parsed Go      │
│    types like time.Duration)             │
└──────────────────┬──────────────────────┘
                   │ load at runtime start
                   ▼
┌─────────────────────────────────────────┐
│ internal/runtime/tools/config.go         │
│   HandlerEntry.HTTPConfig.RetryPolicy    │
│     → *RuntimeHTTPRetryPolicy            │
│   (this PR: loaded, stored, not used)    │
│   (future PR: wired into executor)       │
└─────────────────────────────────────────┘
```

The PR's blast radius stops at the runtime load — the `omnia_executor.go` executor functions continue to ignore retry policy and only need source updates to satisfy the new field types in the structs they read.

## CRD types

### New retry policy types

All in `api/v1alpha1/toolregistry_types.go`.

```go
// HTTPRetryPolicy defines retry behavior for HTTP (and OpenAPI) tool calls.
// When nil or when MaxAttempts is 1, no retries are performed.
type HTTPRetryPolicy struct {
    // maxAttempts is the maximum total number of attempts, including the first.
    // A value of 1 means no retries. Must be between 1 and 10.
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:Minimum=1
    // +kubebuilder:validation:Maximum=10
    MaxAttempts int32 `json:"maxAttempts"`

    // initialBackoff is the delay before the first retry attempt.
    // Subsequent attempts apply exponential backoff via backoffMultiplier.
    // +kubebuilder:default="100ms"
    // +optional
    InitialBackoff *metav1.Duration `json:"initialBackoff,omitempty"`

    // backoffMultiplier multiplies the delay between successive retries.
    // Expressed as a decimal string (e.g. "2.0", "1.5"). Must parse as a float >= 1.0.
    // +kubebuilder:default="2.0"
    // +kubebuilder:validation:Pattern=`^[0-9]+(\.[0-9]+)?$`
    // +optional
    BackoffMultiplier *string `json:"backoffMultiplier,omitempty"`

    // maxBackoff is the upper bound on delay between retry attempts.
    // Must be >= initialBackoff (validated by the controller).
    // +kubebuilder:default="30s"
    // +optional
    MaxBackoff *metav1.Duration `json:"maxBackoff,omitempty"`

    // retryOn is the list of HTTP status codes that trigger a retry.
    // Defaults to [408, 429, 500, 502, 503, 504] if unset.
    // Set to an empty list to disable status-code-based retry entirely.
    // +optional
    RetryOn []int32 `json:"retryOn,omitempty"`

    // retryOnNetworkError enables retries on connection failures, DNS failures,
    // and request timeouts (errors returned before a status code is received).
    // +kubebuilder:default=true
    // +optional
    RetryOnNetworkError *bool `json:"retryOnNetworkError,omitempty"`

    // respectRetryAfter honors the HTTP Retry-After header on 429 and 503 responses,
    // overriding backoff calculations for that attempt.
    // +kubebuilder:default=true
    // +optional
    RespectRetryAfter *bool `json:"respectRetryAfter,omitempty"`
}

// GRPCRetryPolicy defines retry behavior for gRPC tool calls.
// Retries are implemented as an Omnia-side loop, not via gRPC native service config,
// so that retry attempts compose correctly with the existing circuit breaker.
type GRPCRetryPolicy struct {
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:Minimum=1
    // +kubebuilder:validation:Maximum=10
    MaxAttempts int32 `json:"maxAttempts"`

    // +kubebuilder:default="100ms"
    // +optional
    InitialBackoff *metav1.Duration `json:"initialBackoff,omitempty"`

    // +kubebuilder:default="2.0"
    // +kubebuilder:validation:Pattern=`^[0-9]+(\.[0-9]+)?$`
    // +optional
    BackoffMultiplier *string `json:"backoffMultiplier,omitempty"`

    // +kubebuilder:default="30s"
    // +optional
    MaxBackoff *metav1.Duration `json:"maxBackoff,omitempty"`

    // retryableStatusCodes is the list of gRPC status codes that trigger a retry.
    // Defaults to ["UNAVAILABLE", "DEADLINE_EXCEEDED", "RESOURCE_EXHAUSTED"] if unset.
    // Set to an empty list to disable status-code-based retry entirely.
    // Values must be valid gRPC status code names.
    // +optional
    RetryableStatusCodes []string `json:"retryableStatusCodes,omitempty"`
}

// MCPRetryPolicy defines retry behavior for MCP CallTool failures.
//
// Note: MCP session reconnect on broken transport is handled separately by the
// MCP client wrapper and is not governed by this retry policy. See the runtime
// session-health issue (to be filed separately).
type MCPRetryPolicy struct {
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:Minimum=1
    // +kubebuilder:validation:Maximum=10
    MaxAttempts int32 `json:"maxAttempts"`

    // +kubebuilder:default="100ms"
    // +optional
    InitialBackoff *metav1.Duration `json:"initialBackoff,omitempty"`

    // +kubebuilder:default="2.0"
    // +kubebuilder:validation:Pattern=`^[0-9]+(\.[0-9]+)?$`
    // +optional
    BackoffMultiplier *string `json:"backoffMultiplier,omitempty"`

    // +kubebuilder:default="30s"
    // +optional
    MaxBackoff *metav1.Duration `json:"maxBackoff,omitempty"`
}
```

### Changes to existing types

```go
// HandlerDefinition: replace Retries + Timeout fields
type HandlerDefinition struct {
    // ... all existing fields unchanged ...

    // timeout specifies the maximum duration for a single tool invocation (wall clock).
    // Applies to all handler types.
    // +kubebuilder:default="30s"
    // +optional
    Timeout *metav1.Duration `json:"timeout,omitempty"`

    // Retries field REMOVED. Retry policies are now transport-specific,
    // defined inside httpConfig.retryPolicy, grpcConfig.retryPolicy,
    // mcpConfig.retryPolicy, and openAPIConfig.retryPolicy.
}

// HTTPConfig: add RetryPolicy as the last field
type HTTPConfig struct {
    // ... all existing fields unchanged ...

    // retryPolicy configures retry behavior for this HTTP tool.
    // When nil, tool calls are executed once with no retries.
    // +optional
    RetryPolicy *HTTPRetryPolicy `json:"retryPolicy,omitempty"`
}

// GRPCConfig: add RetryPolicy
type GRPCConfig struct {
    // ... all existing fields unchanged ...

    // +optional
    RetryPolicy *GRPCRetryPolicy `json:"retryPolicy,omitempty"`
}

// MCPConfig: add RetryPolicy
type MCPConfig struct {
    // ... all existing fields unchanged ...

    // +optional
    RetryPolicy *MCPRetryPolicy `json:"retryPolicy,omitempty"`
}

// OpenAPIConfig: add RetryPolicy using HTTPRetryPolicy
// (OpenAPI delegates to the HTTP executor, so it shares HTTP retry semantics.)
type OpenAPIConfig struct {
    // ... all existing fields unchanged ...

    // retryPolicy configures retry behavior for operations exposed by this handler.
    // Uses HTTPRetryPolicy because OpenAPI execution delegates to the HTTP executor.
    // +optional
    RetryPolicy *HTTPRetryPolicy `json:"retryPolicy,omitempty"`
}
```

### Design choices worth recording

1. **`BackoffMultiplier *string`, not `*float64`.** Kubernetes CRDs handle floats awkwardly — kubebuilder emits `type: number` in the OpenAPI schema but some downstream tooling trips over it. A regex-validated string is the Kubernetes idiom (same approach as `resource.Quantity`). The controller parses with `strconv.ParseFloat` at load time.

2. **`Timeout *metav1.Duration`, not `*string`.** `metav1.Duration` is validated at admission time via kubebuilder-generated regex. Typos like `"30seconds"` fail at `kubectl apply` instead of at reconcile. This is one extra breaking change beyond removing `Retries`, justified because the validation guarantees are strictly better.

3. **`MaxAttempts` is required when any retry policy is set.** If a user creates a `retryPolicy` object at all, they're making an explicit decision about retries and must specify the count. Defaulting to 1 would create silent single-attempt policies; defaulting above 1 would enable retries too aggressively on users who set only one field. Required forces the decision.

4. **`BackoffMultiplier` minimum enforced in the controller, not the CRD.** The regex `^[0-9]+(\.[0-9]+)?$` admits `0.5`, which is invalid. The controller validates `>= 1.0` at load time with a clear error message. A CEL validation rule on the CRD is possible but harder to write correctly for decimal-string comparison.

5. **No `MaxBackoff >= InitialBackoff` CEL constraint.** Comparing `metav1.Duration` in CEL is fiddly because you're comparing serialized strings. The controller validates this at load time with a clear error.

6. **`MCPRetryPolicy` exists as its own type even with identical fields to a hypothetical base struct.** Per our design decision on struct factoring, each transport gets a distinct type, matching the existing `HTTPConfig`/`GRPCConfig`/`MCPConfig` pattern. Keeps the CRD self-documenting and lets future MCP-specific fields land without refactoring.

7. **OpenAPI reuses `HTTPRetryPolicy` (alias, not a distinct type).** OpenAPI delegates execution to the HTTP executor, so retry semantics are identical. Defining a separate `OpenAPIRetryPolicy` struct with the same fields would be pointless duplication without the self-documenting benefit, since the reader would immediately realize they're HTTP semantics anyway.

## Controller translation

All in `internal/controller/tools_config.go`.

### Runtime-side type mirror

The runtime's `HandlerEntry` and its sub-configs need parallel retry policy types with parsed Go types:

```go
// internal/runtime/tools/config.go

type RuntimeHTTPRetryPolicy struct {
    MaxAttempts         int32
    InitialBackoff      time.Duration
    BackoffMultiplier   float64
    MaxBackoff          time.Duration
    RetryOn             []int32
    RetryOnNetworkError bool
    RespectRetryAfter   bool
}

type RuntimeGRPCRetryPolicy struct {
    MaxAttempts          int32
    InitialBackoff       time.Duration
    BackoffMultiplier    float64
    MaxBackoff           time.Duration
    RetryableStatusCodes []string
}

type RuntimeMCPRetryPolicy struct {
    MaxAttempts       int32
    InitialBackoff    time.Duration
    BackoffMultiplier float64
    MaxBackoff        time.Duration
}
```

The `Runtime` prefix disambiguates these from the CRD types with the same shape. The controller converts across the boundary.

Two other changes to `HandlerEntry` and its sub-configs:
- **Remove** `HandlerEntry.Retries int32`.
- **Parse durations once at load time, not per call.** The tools.yaml wire format stays string-based for the duration fields (`timeout: "30s"`) because that's human-readable. The runtime's in-memory types (`HandlerEntry`, `RuntimeHTTPRetryPolicy`, etc.) carry `time.Duration` fields. `LoadConfig` converts string → `time.Duration` during load, via whatever mechanism the implementation plan prefers (custom `UnmarshalYAML` on each struct, a `Duration` wrapper type, or a post-unmarshal walk that populates parsed fields from raw fields). The spec commits to the behavior — a bad duration string becomes a `LoadConfig` error, executors never re-parse strings — not to the exact struct layout.

### `buildHandlerEntry` signature change

```go
func buildHandlerEntry(h *omniav1alpha1.HandlerDefinition, endpoint string) (HandlerEntry, error)
```

Now returns an error. Retry policy translation can fail (bad `BackoffMultiplier`, unknown gRPC status code, `MaxBackoff < InitialBackoff`). The callers in `buildToolsConfig` propagate errors — if any handler has an invalid retry policy, the whole reconcile fails with a clear status condition rather than silently dropping the handler.

### Per-transport retry builders

Three small helpers, one per retry policy type, all in `tools_config.go`:

```go
func buildHTTPRetryPolicy(p *omniav1alpha1.HTTPRetryPolicy) (*RuntimeHTTPRetryPolicy, error)
func buildGRPCRetryPolicy(p *omniav1alpha1.GRPCRetryPolicy) (*RuntimeGRPCRetryPolicy, error)
func buildMCPRetryPolicy(p *omniav1alpha1.MCPRetryPolicy) (*RuntimeMCPRetryPolicy, error)
```

Each:
1. Returns `(nil, nil)` when the input is `nil`.
2. Seeds a runtime struct with Go-level defaults (same values as the kubebuilder defaults).
3. Overrides each default with the user's value when set.
4. Parses `BackoffMultiplier` with `strconv.ParseFloat` and validates `>= 1.0`.
5. Validates `MaxBackoff >= InitialBackoff`.
6. For gRPC specifically: validates each entry in `RetryableStatusCodes` against a known-set constant.
7. Returns the runtime struct or an error with context identifying the invalid field.

### Why defaults live in both kubebuilder and the controller

kubebuilder defaults only apply when the user omits a field entirely — they don't apply when the user sets an empty `retryPolicy: {}` object with only `maxAttempts`. The controller's builder applies defaults explicitly on the Go struct after unmarshal, so the behavior is identical regardless of whether the user omitted the field or set it to an empty object.

Both layers get defaults: kubebuilder defaults make `kubectl get` and dry-run show sensible values; controller defaults ensure the runtime gets a fully-populated policy regardless of API server quirks.

### Error propagation to `ToolRegistry` status

`buildToolsConfig` currently iterates handlers and silently skips failures. The new version:
1. Collects per-handler build errors.
2. If any occurred, sets a `RetryPolicyInvalid` condition on the `ToolRegistry` status with a message listing the failing handlers and the specific field-level errors.
3. Does NOT generate a partial `tools.yaml` — either the whole config is valid or the reconcile surfaces the error.

This is strictly more honest than the current silent-drop and gives admins an immediate signal when their CR is misconfigured.

## Runtime changes

Deliberately minimal in this PR. The runtime:
- Loads the new retry policy fields from `tools.yaml` via the existing `LoadConfig` path.
- Stores them on `HandlerEntry.HTTPConfig.RetryPolicy`, etc.
- Makes them available to the executor function signatures so tests can assert they're there.
- **Does nothing with them at execution time.** `executeHTTP`, `executeGRPC`, `executeMCP`, `executeOpenAPI` paths remain unchanged behaviorally.

Retry logic lands in a follow-up PR that plugs into these structs. Keeping the shape refactor behaviorally inert makes review and rollback tractable.

## Testing

### 1. CRD type validation (`api/v1alpha1/toolregistry_types_test.go`)

Go unit tests for struct shapes — cheap, fast, catch regression on DeepCopy.

- For each of `HTTPRetryPolicy`, `GRPCRetryPolicy`, `MCPRetryPolicy`: round-trip JSON marshal/unmarshal (declare → marshal → unmarshal → deep-equal).
- DeepCopy exercise on a fully-populated struct (regenerated by `make generate`; test catches pointer-vs-value mistakes).

### 2. CRD YAML validation (envtest)

Lives in `internal/controller/toolregistry_controller_envtest_test.go` (new file if one doesn't exist; follow existing envtest patterns in the project).

- **Happy path**: create a `ToolRegistry` with each transport containing a retry policy, expect clean apply.
- **Rejection — invalid `BackoffMultiplier` string** (`"abc"`, `"-1.5"`, `"2.0x"`): expect API server rejection on the regex pattern.
- **Rejection — `MaxAttempts` out of range** (`0`, `11`, `-1`): expect min/max validation rejection.
- **Rejection — invalid Duration string** (`"30seconds"`): expect `metav1.Duration` pattern rejection.
- **Rejection — top-level `retries` field present**: expect API server to reject the unknown field. Proves the breaking change actually breaks old YAML.
- **Acceptance — retry policy omitted entirely**: applied unchanged, retry policy stays nil.
- **Acceptance — retry policy with only `maxAttempts` set**: kubebuilder defaults populate the remaining fields on the returned object.

### 3. Controller translation (`internal/controller/tools_config_test.go`, existing file)

Unit tests against `buildHandlerEntry` and the three retry builders. Table-driven.

For each `buildXRetryPolicy` function:
- **Nil input → nil output, no error.**
- **Fully-populated valid policy → fully-populated runtime policy**, field-by-field match.
- **Minimum valid policy (only `MaxAttempts` set)**: runtime policy has Go-level defaults applied.
- **Invalid `BackoffMultiplier` string** → error containing the bad value.
- **`BackoffMultiplier < 1.0`** → error mentioning `"must be >= 1.0"`.
- **`MaxBackoff < InitialBackoff`** → error with both durations.
- **gRPC-specific**: unknown status code string → error listing valid codes.
- **HTTP-specific**: empty `RetryOn` **slice** (`[]int32{}`) stays empty — user explicitly said "never retry on status codes". Distinct from nil (which means apply defaults).

For `buildHandlerEntry`:
- **Happy path** per transport type with retry policy: correct runtime entry.
- **Error propagation**: bad retry policy in any transport → error wrapped with handler name.
- **Top-level timeout conversion**: `metav1.Duration("30s")` → `time.Duration(30s)` on runtime entry.
- **Backward compat — no retry policy set**: runtime entry has nil retry policies, everything else identical to current behavior.

### 4. Controller status propagation (`internal/controller/toolregistry_controller_test.go`)

Envtest-style. Create a `ToolRegistry` with a bad retry policy that passes CRD validation but fails controller validation (`maxBackoff < initialBackoff`). Expect reconcile to set `RetryPolicyInvalid` condition with a readable message. Expect no new `tools.yaml` ConfigMap to be created; if one existed from a prior good state, it remains unchanged.

### 5. Runtime loader (`internal/runtime/tools/config_test.go`)

Round-trip: write a tools.yaml containing retry policies for each transport, load via `LoadConfig`, assert fields parse correctly.

- Per-transport retry policy parses.
- Default values from operator translation serialize to yaml and reload correctly.
- Missing retry policy → nil field, no error.

### 6. Wiring test

Per CLAUDE.md's wiring-test rule, in the appropriate `cmd/*/wiring_test.go` (wherever tool config is loaded at process start — likely `cmd/runtime/` or `cmd/agent/`; determined during implementation). Start the real binary with a tools.yaml that has retry policies, assert the policy field is accessible on the handler entry at the executor call site.

This is a "the runtime sees the field" test, not a "retries actually happen" test.

### 7. Helm template verification (build-time check, not a test)

`helm template charts/omnia` with a `ToolRegistry` CR containing retry policies, assert:
- The generated CRD schema includes the new types
- No references to the removed top-level `retries` field anywhere in the chart output

### Coverage targets

Per CLAUDE.md 80% coverage on changed files:

- `api/v1alpha1/toolregistry_types.go` — no logic, covered by marshalling tests.
- `internal/controller/tools_config.go` — 90%+ on the new builders via table-driven tests.
- `internal/runtime/tools/config.go` — loader round-trip tests cover new fields.
- `internal/runtime/tools/omnia_executor.go` — no changes beyond source-level field types; existing executor tests continue to pass.

### Out of scope for this PR's tests

- Retry **behavior** (no retry loop in this PR).
- Circuit breaker interaction.
- MCP session reconnect on broken transport.
- Timeout enforcement (the runtime stores `time.Duration` but doesn't apply it as a deadline — that's #779's job).

## Breaking change notes

1. **`HandlerDefinition.Retries` removed.** Any existing `ToolRegistry` CR with `retries: 3` at the top level will fail `kubectl apply` post-upgrade. The current behavior is a silent no-op, so no user-visible functionality changes — users just get an honest error where today they get a silent drop. v1alpha1 allows breaking changes without version bumps, and the project is pre-release.
2. **`HandlerDefinition.Timeout` type change** from `*string` to `*metav1.Duration`. The YAML syntax is identical (`timeout: 30s`) but validation now happens at admission time instead of reconcile. Strictly stricter.
3. **No migration shim, no deprecation warning.** Clean break.

## Size estimate

| Area | Deletions | Additions |
|---|---|---|
| `api/v1alpha1/toolregistry_types.go` | ~3 lines (`Retries`, old `Timeout`) | ~180 lines (3 new types + 4 new fields + updated `Timeout`) |
| `api/v1alpha1/zz_generated.deepcopy.go` | regenerated | regenerated |
| `api/v1alpha1/toolregistry_types_test.go` | 0 | ~120 lines (marshalling tests) |
| `internal/controller/tools_config.go` | ~5 lines | ~180 lines (3 builders + error propagation) |
| `internal/controller/tools_config_test.go` | 0 | ~300 lines (table-driven builder tests) |
| `internal/controller/toolregistry_controller_test.go` | 0 | ~80 lines (envtest status) |
| `internal/controller/toolregistry_controller_envtest_test.go` | 0 (new file) | ~200 lines (CRD validation tests) |
| `internal/runtime/tools/config.go` | ~2 lines | ~60 lines (runtime types + new fields) |
| `internal/runtime/tools/config_test.go` | 0 | ~120 lines (loader round-trip) |
| `cmd/*/wiring_test.go` | 0 | ~40 lines |
| Generated CRD YAML | regenerated | regenerated |
| Helm CRD copy | regenerated via `make sync-chart-crds` | regenerated |
| `agentruntime_controller_test.go` (existing `Timeout: &timeout, Retries: &retries` test fixtures) | ~2 lines | ~2 lines (replace `Retries:` with retry policy set, adjust `Timeout:` type) |

**Estimated total**: ~1100 LOC added, ~10 LOC removed, plus regenerated files. Review surface is focused on `toolregistry_types.go` + `tools_config.go` + tests.

## Follow-up work (explicit, out of this PR)

1. **#779 rebased on the new shape** — forward the HTTP fields (`ContentType`, `QueryParams`, etc.) to PromptKit's `HTTPConfig`, wire `OutputSchema` in `buildDescriptor`, apply `Timeout` to each transport's call-site context. Uses the new retry policy fields as placeholders but doesn't implement retry behavior yet.
2. **Retry implementation PR** — implement the actual retry loop for HTTP/OpenAPI/MCP/gRPC against the `Runtime*RetryPolicy` structs. Integrates with the circuit breaker work from #778.
3. **MCP session reconnect on broken transport** — separate runtime issue, new file in `internal/runtime/tools/`. Detect broken sessions via transport errors, close and reconnect on next use. Orthogonal to retry policy.
4. **#778 circuit breaker extension** — already tracked, now naturally composes with retries since both live at the Omnia call-site layer.

## Unresolved items

None. Design is ready for implementation planning.
