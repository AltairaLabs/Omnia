# PromptKit LSP Service (Enterprise)

## Owns
- Language Server Protocol implementation for Arena agent definitions
- Real-time validation of PromptPack YAML/JSON
- Code completion and hover documentation
- File access via dashboard API proxy

## Inputs
- **WebSocket** from Dashboard: LSP protocol messages (via proxy)
- **HTTP** from Dashboard: file content requests

## Outputs
- **WebSocket** to Dashboard: diagnostics, completions, hover info
- **HTTP**: health probes

## Does NOT Own
- File storage (workspace filesystem, managed by Operator)
- Agent execution (Runtime's job)
- Dashboard UI (Dashboard's job)

## Observability

**Metrics**: None currently.

**Traces**: None.

## Dependencies
- Dashboard API (file access proxy)
- PromptKit schema definitions
