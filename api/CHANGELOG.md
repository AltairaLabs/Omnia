# API Surface Changelog

Changes to any API surface (REST, gRPC, WebSocket) should be logged here
so that parallel workstreams have visibility into contract changes.

When modifying files in `internal/session/api/`, `internal/facade/protocol.go`,
or `api/proto/`, add an entry below with the date, affected API, and reason.

---

## Unreleased

### Breaking (CRD rename, no GitHub issue)

- `MemoryRetentionPolicy` CRD renamed to `MemoryPolicy`. The schema is
  identical; only the kind / plural / shortname change:
  - `kind: MemoryRetentionPolicy` → `kind: MemoryPolicy`
  - plural `memoryretentionpolicies` → `memorypolicies`
  - shortname `mrp` → `mp`
- Migration for clusters with existing instances:

  ```bash
  kubectl get memoryretentionpolicies -o yaml \
    | sed 's/MemoryRetentionPolicy/MemoryPolicy/g; s/memoryretentionpolicies/memorypolicies/g' \
    | kubectl apply -f -
  kubectl delete crd memoryretentionpolicies.omnia.altairalabs.ai
  ```

- The rename frees up `MemoryPolicy` as the natural home for upcoming
  per-workspace memory configuration knobs (next: tier-precedence
  ranking weights for multi-tier retrieval).

### Added (memory consent classifier, #1005)

- `memory_entities.consent_category` is now populated automatically on EE
  deployments via the new content classifier (`ee/pkg/privacy/classify`):
  PII regex + optional embedding similarity, upgrade-only validator. No
  schema change. OSS deployments unaffected — column stays `NULL`.
- `SaveMemoryRequest.Category` is now propagated into
  `mem.Metadata[consent_category]` in the handler (was a silent dropped
  field). EE middleware classification results now reach the column.

### Added (per-session consent grants, #1006)

- WebSocket: `ClientMessage.session_consent_grants` (`[]string`, optional).
  First message with a non-empty list stamps the per-session default on the
  Connection. Subsequent messages with a non-empty list replace the cached
  value (last-writer-wins). Empty / omitted lists are ignored — to revoke
  all categories for a session use the binary opt-out.
- gRPC: `x-omnia-consent-layer` metadata key carries which layer
  (`per-message` | `session` | `persistent`) produced the per-request grants
  forwarded by the facade.
- Memory API: `X-Consent-Layer` request header (forwarded by the runtime
  httpclient). `X-Consent-Decision` response header on 204 dropped writes.
  New `omnia_memory_writes_suppressed_total{layer,category,reason}`
  Prometheus metric. Suppressed-write log line promoted from V(1) to V(0)
  and enriched with layer + grants.

### Added (memory analytics backend, #1004)

- `GET /api/v1/memories/aggregate?workspace=X&groupBy={category|agent|day}&metric={count|distinct_users}` —
  workspace-scoped aggregate over `memory_entities`. Composes the
  Phase D `analytics:aggregate` consent filter so users without that
  grant are excluded by construction. Optional `from`/`to` (RFC3339)
  for time bounds; `limit` clamped to [1, 1000]. Returns
  `[{key, value, count}]` matching the eval-results aggregate pilot
  shape from `docs/local-backlog/2026-04-17-observability-split-design.md`.
- `GET /api/v1/privacy/consent/stats?workspace=X` (EE only) —
  workspace-wide consent posture: `totalUsers`, `optedOutAll`,
  `grantsByCategory`. Workspace param reserved for future per-workspace
  scoping; currently a no-op (preferences table is user-keyed).

### Added (analytics:aggregate enforcement foundation, #1007)

- New `memory.AggregateConsentJoin(alias)` helper produces SQL JOIN +
  WHERE fragments restricting a cross-user aggregate to users who have
  granted `analytics:aggregate`. Institutional and agent-tier rows pass.
- New Prometheus metrics:
  - `omnia_memory_consent_analytics_optin_ratio` (gauge, 0..1)
  - `omnia_memory_consent_analytics_users_total{granted}` (gauge pair)
  - `omnia_memory_consent_analytics_worker_errors_total{reason}` (counter)
- `AnalyticsOptInWorker` queries `user_privacy_preferences` every 5
  minutes to refresh the gauges.
- No existing queries retrofitted — memory-api has zero cross-user
  aggregation today. The helper lands as foundation for the upcoming
  #1004 dashboard.

### Breaking

- `SessionPrivacyPolicy.spec.level`, `spec.workspaceRef`, and `spec.agentRef` removed. Policies are now reusable namespaced documents; binding has moved to consumers (`Workspace` service groups and `AgentRuntime`).
- `SessionPrivacyPolicy` is now **namespace-scoped** (was cluster-scoped).

### Added (podOverrides, #844)

- New shared `PodOverrides` type (`api/v1alpha1/shared_types.go`) with pod-level fields (`serviceAccountName`, `labels`, `annotations`, `nodeSelector`, `tolerations`, `affinity`, `priorityClassName`, `topologySpreadConstraints`, `imagePullSecrets`) and container-level fields (`extraEnv`, `extraEnvFrom`, `extraVolumes`, `extraVolumeMounts`).
- `AgentRuntime.spec.podOverrides` — facade + runtime Pod customization. Container-level fields apply to both user containers but skip operator-injected sidecars (e.g. `policy-proxy`).
- `AgentRuntime.spec.evals.podOverrides` — namespace-level eval-worker customization (last-writer-wins, matches existing eval-worker semantics).
- `Workspace.spec.services[].session.podOverrides` — managed session-api Pod customization.
- `Workspace.spec.services[].memory.podOverrides` — managed memory-api Pod customization.
- `ArenaJob.spec.workers.podOverrides` — worker Job Pod customization.
- `ArenaDevSession.spec.podOverrides` — dev-console Pod customization.

All fields optional; default rendering is byte-identical to before. Existing hooks (`FacadeConfig.ExtraEnv`, `RuntimeConfig.NodeSelector`/`Tolerations`/`Affinity`/`Volumes`/`VolumeMounts`/`ExtraEnv`, `AgentRuntimeSpec.ExtraPodAnnotations`) are preserved and applied first; PodOverrides values are merged or appended after.

### Added

- `Workspace.spec.services[].privacyPolicyRef` (`LocalObjectReference`) — selects the `SessionPrivacyPolicy` applied to sessions managed by that service group's session-api.
- `AgentRuntime.spec.privacyPolicyRef` (`LocalObjectReference`) — per-agent override that takes precedence over the service group policy.
- `PrivacyPolicyResolved` status condition on `Workspace` and `AgentRuntime`. Reason values:
  - `PolicyResolved` — a named policy was found and is active.
  - `PolicyNotFound` — the `privacyPolicyRef` points to a missing policy.
  - `DefaultPolicy` — no service group in the workspace sets a ref; the global default (or none) will be used.
  - `WorkspaceDefault` — the AgentRuntime has no override ref; effective policy comes from the service group or global default.

### Added (skills, #806)

- New `SkillSource` CRD (core, namespace-scoped). Syncs AgentSkills.io content from Git/OCI/ConfigMap into the workspace content PVC, with a post-fetch `filter` block (globs + explicit names). Status conditions: `SourceAvailable`, `ContentValid`.
- `PromptPack.spec.skills[]` (`SkillRef` array): each entry references a `SkillSource` in the pack's namespace and optionally narrows it via `include` and renames the group via `mountAs`.
- `PromptPack.spec.skillsConfig`: tunes PromptKit's skill runtime (`maxActive`, `selector`).
- `PromptPack` status conditions: `SkillsResolved`, `SkillsValid`, `SkillToolsResolved`.
- AgentRuntime runtime container: read-only mount of the workspace content PVC at `/workspace-content` and `OMNIA_PROMPTPACK_MANIFEST_PATH` env var, when the operator's `--workspace-content-path` flag is set (default `/workspace-content`).

---

## 2026-04-12
- **Session API**: Added `GET /api/v1/privacy-policy?namespace={ns}&agent={agent}` endpoint
  - Returns the facade-visible subset of the effective `SessionPrivacyPolicy`: `{"recording":{"enabled":bool,"facadeData":bool,"richData":bool}}`.
  - Responds with 200 + JSON when a policy applies, or 204 No Content when no policy applies.
  - Consumed by the facade at WebSocket-connect time (cached 60s per connection) to decide whether to record.
- **Session API**: Behavior change on recording write endpoints — `POST /api/v1/sessions/{id}/messages`, `POST /api/v1/sessions/{id}/tool-calls`, `POST /api/v1/sessions/{id}/provider-calls`, and `POST /api/v1/sessions/{id}/events`
  - When the effective `SessionPrivacyPolicy.Recording.Enabled` is `false`, these endpoints now return **204 No Content** and drop the write.
  - When `SessionPrivacyPolicy.Recording.RichData` is `false`, the middleware blocks assistant messages, tool-call, runtime-event, and provider-call writes (204). User messages, status updates, and TTL refreshes are still accepted.
  - **Non-breaking** for deployments without a `SessionPrivacyPolicy` — default behavior is unchanged (recording enabled, rich data allowed).
- **Note**: Session data at-rest encryption infrastructure is in place (`ee/pkg/encryption` extended for `ToolCall`/`RuntimeEvent`; `KeyRotationReconciler` wired) but session-api integration is pending — see follow-up issue. The current `SessionPrivacyPolicy.Encryption` wiring assumed a single global policy at startup, which is incorrect for multi-workspace deployments; a CRD redesign that makes `SessionPrivacyPolicy` a reusable policy referenced by `Workspace`/`AgentRuntime` is required before shipping encryption.

## 2026-04-07
- **Session API**: Added `cohortId` and `variant` fields to `CreateSessionRequest` and `Session` schemas
  - Supports rollout cohort tracking: Istio routes set `x-omnia-cohort-id` and `x-omnia-variant` headers,
    which the facade extracts and persists to the session for per-variant analysis.
  - New Postgres columns `cohort_id` and `variant` with partial indexes (migration 000025).
  - New OTel span attributes `omnia.cohort.id` and `omnia.variant` on `omnia.facade.message` spans.
  - Non-breaking: both fields are optional with `omitempty`.

## 2026-03-28
- **ArenaJob CRD** (Enterprise): Added `spec.sessionRecording` boolean (default: false)
  - **Breaking**: Session recording was previously always enabled when `SESSION_API_URL` was configured on the controller. Now requires explicit `sessionRecording: true` on the ArenaJob spec. Existing ArenaJobs without this field will stop recording sessions after upgrade.
  - **Migration**: Add `sessionRecording: true` to any ArenaJob that needs session transcripts.

## 2026-03-26
- **ArenaJob CRD** (Enterprise): Replaced `LoadTestSettings` and added `spec.trials` for load testing support (#661)
  - **Added** `spec.trials` — number of times to repeat each scenario × provider combination
  - **Replaced** `spec.loadTest` — old fields (`rampUp`, `duration`, `targetRPS`) replaced with scenario-driven model:
    - `concurrency` — max in-flight work items globally
    - `vusPerWorker` — concurrent goroutines per worker pod
    - `ramp` — linear ramp-up/down configuration (`up`, `down` duration strings)
    - `budgetLimit` / `budgetCurrency` — cost safety limit
    - `rateLimits` — per-provider concurrency caps
    - `thresholds` — SLO pass/fail gating (metric, operator, value)
  - **Added** `RampConfig`, `ProviderRateLimit`, `LoadThreshold`, `LoadThresholdMetric`, `LoadThresholdOperator` types
  - Old `LoadTestSettings` fields were never wired up, so this is a safe breaking change

## 2026-03-17
- **ArenaJob CRD** (Enterprise): Replaced provider/tool override pipeline with CRD-based provider groups
  - **Added** `spec.providers` — map of group names to lists of `ArenaProviderEntry` (providerRef or agentRef)
  - **Added** `spec.toolRegistries` — list of ToolRegistry CRD refs by name
  - **Removed** `spec.providerOverrides` — label-selector-based provider matching
  - **Removed** `spec.toolRegistryOverride` — label-selector-based tool registry matching
  - **Removed** `spec.execution` — fleet mode is replaced by `agentRef` entries in provider groups
  - **Removed** `ExecutionMode`, `ExecutionConfig`, `FleetTarget`, `ProviderGroupSelector`, `ToolRegistrySelector` types
  - Agents and LLM providers are now interchangeable in the scenario × provider matrix
  - Deleted `ee/pkg/selector/` package (no longer needed)

## 2026-03-15
- **Session API**: Fixed `eval_results.session_id` type from TEXT to UUID (migration 000020)
  - Added `(session_id, message_id)` composite index for faster lookups
  - Removed `::text` cast from cascade delete trigger
  - Eval events from runtime now written to `runtime_events` instead of messages
- **Session API**: Added structured multi-modal fields to `Message` schema (`hasMedia`, `mediaTypes`)
  - Migration `000019_structured_multimodal` adds `has_media`/`media_types` to messages and queryable columns to message_artifacts
  - Enables queries like "sessions with voice input" without parsing JSON metadata
- **Session API**: Added runtime events endpoints (`POST/GET /api/v1/sessions/{sessionID}/events`)
  - New `RuntimeEvent` schema in OpenAPI spec
  - Stores PromptKit lifecycle events (pipeline, stage, middleware, validation, workflow) in dedicated `runtime_events` table instead of as system messages
  - Migration `000018_create_runtime_events` creates partitioned table
- **Session API**: Added OpenAPI 3.0 spec at `api/session-api/openapi.yaml` covering all 18 endpoints
  - Generated Go client: `pkg/sessionapi/` (via oapi-codegen v2.4.1)
  - Generated TS types: `dashboard/src/lib/api/session-api-schema.d.ts` (via openapi-typescript)
  - Makefile targets: `generate-session-api-client`, `generate-session-api-types`, `validate-session-api-spec`
- **WebSocket protocol**: Removed `execution` field from `ToolCallInfo`
  - Go struct: `internal/facade/protocol.go:ToolCallInfo`
  - Reason: All tool calls sent over WebSocket are client-side by definition. Server-side tool calls are filtered at the facade and never forwarded.
  - Generated types updated: `dashboard/src/types/generated/websocket.ts`

## 2026-03-14
- **WebSocket protocol**: Added `tool_result` client message type and `ClientToolResultInfo` struct
  - Go struct: `internal/facade/protocol.go:ClientToolResultInfo`
  - Reason: Client-side tool execution (#617) requires the dashboard to send tool results back
- **gRPC protocol**: Removed `tool_result` from `ServerMessage` oneof
  - Proto: `api/proto/runtime/v1/runtime.proto`
  - Reason: Server-side tool results are handled internally by the runtime, not sent to the facade
