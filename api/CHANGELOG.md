# API Surface Changelog

Changes to any API surface (REST, gRPC, WebSocket) should be logged here
so that parallel workstreams have visibility into contract changes.

When modifying files in `internal/session/api/`, `internal/facade/protocol.go`,
or `api/proto/`, add an entry below with the date, affected API, and reason.

---

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
