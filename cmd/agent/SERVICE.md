# Facade Service

## Owns
- WebSocket server for browser/client connections
- Protocol translation: WebSocket JSON <-> gRPC bidirectional stream
- Connection lifecycle (upgrade, ping/pong, close, rate limiting)
- Session creation and routing
- Binary frame encoding/decoding for media
- Media upload URL negotiation (S3/GCS/Azure/local)
- Client-side tool result routing to active handler
- Session recording via HTTP client to Session API

## Inputs
- **WebSocket** from browser/dashboard:
  - `message` — user text or multimodal content
  - `tool_result` — client-side tool execution result
  - `upload_request` — file upload initiation
- **gRPC** from Runtime (response stream):
  - `chunk` — streaming text
  - `done` — response complete
  - `tool_call` — client-side tool call (server-side tool calls are filtered)
  - `error` — error response
  - `media_chunk` — streaming audio/video

## Outputs
- **WebSocket** to browser/dashboard: ServerMessage (chunk, done, tool_call, error, connected, media_chunk, upload_ready, upload_complete)
- **gRPC** to Runtime: ClientMessage (user message, client tool result)
- **HTTP** to Session API: session create, message append, TTL refresh

## Does NOT Own
- Tool execution logic (Runtime's job — client or server)
- LLM provider interaction (Runtime's job)
- Session persistence (Session API's job)
- Prompt pack content or evaluation (Runtime's job)
- UI state management (Dashboard's job)
- Authentication (passes headers through)

## Dependencies
- Runtime gRPC server (default `localhost:9000`)
- Session API HTTP endpoint (configurable via `SESSION_API_URL`)
- Media storage provider (optional: S3/GCS/Azure/local)
