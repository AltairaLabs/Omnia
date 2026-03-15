# Dashboard Service

## Owns
- Next.js web application (UI)
- WebSocket proxy to backend services (facade, LSP, dev console)
- Proxy routes to Operator REST API
- Client-side state management (Zustand stores)
- Client-side tool consent UI
- Arena project management UI

## Inputs
- **HTTP** from browser: page requests, API proxy calls
- **WebSocket** from browser: chat messages, tool results

## Outputs
- **HTTP** to Operator: CRUD proxy requests
- **WebSocket** to Facade: agent chat (port 8080 on agent pods)
- **WebSocket** to PromptKit LSP: code intelligence
- **WebSocket** to Arena Dev Console: interactive testing
- **HTML/JS** to browser: rendered UI

## Does NOT Own
- Session storage or persistence (reads via proxy to Session API)
- LLM execution (Runtime's job via Facade)
- Tool execution (Runtime or client-side, never dashboard-initiated)
- K8s resource management (Operator's job)
- Authentication (relies on external auth headers)

## Observability

**Metrics**: None — client-side only. Server-side metrics are handled by the Operator (which hosts the dashboard).

**Traces**: None — does not emit OpenTelemetry spans.

## Dependencies
- Operator REST API (proxied via Next.js API routes)
- Session API (proxied via Operator)
- Facade WebSocket (proxied via server.js on port 3002)
