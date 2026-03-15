# Arena Dev Console Service (Enterprise)

## Owns
- Interactive WebSocket server for testing Arena agents
- Hot-reload of agent configuration without restart
- Provider listing and configuration for testing
- Session recording for dev sessions

## Inputs
- **WebSocket** from Dashboard: chat messages, config reload requests
- **K8s API**: PromptPack and provider configuration

## Outputs
- **WebSocket** to Dashboard: LLM response stream, tool calls
- **HTTP** to Session API: session recording
- **HTTP**: provider listing, health endpoints

## Does NOT Own
- Dev session lifecycle management (Arena Controller's job)
- Dashboard UI (Dashboard's job)
- Production agent serving (Facade + Runtime's job)

## Observability

**Metrics**: Shares the same LLM metrics as Runtime (`omnia_llm_*`) via PromptKit SDK collector.

**Traces**: Uses the shared tracing provider (`internal/tracing`) — emits conversation and LLM spans like the Runtime.

## Dependencies
- PromptKit SDK (conversation management)
- Session API (session recording)
- LLM providers (configured at runtime)
