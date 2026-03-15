# Arena Eval Worker Service (Enterprise)

## Owns
- Consuming session events from Redis Streams
- Executing LLM judge evaluations against session turns
- Writing eval results to Session API
- PromptPack-based eval definition loading

## Inputs
- **Redis Streams**: session events (message appended, session completed)
- **K8s API**: PromptPack ConfigMaps for eval definitions

## Outputs
- **HTTP** to Session API: eval result writes
- **Prometheus**: eval execution metrics

## Does NOT Own
- Event publishing (Runtime/Session API's job)
- Session storage (Session API's job)
- Job scheduling (Arena Controller's job)
- LLM conversation management (Runtime's job)

## Observability

**Metrics** (Prometheus, prefix `omnia_eval_worker_`):
- Events: `events_received_total` (by event_type), `event_processing_duration_seconds`
- Evals: `evals_executed_total` (by eval_type, trigger, status), `eval_duration_seconds`
- Sampling: `evals_sampled_total` (by decision: sampled/skipped)
- Stream health: `stream_lag` gauge (pending messages per stream)
- Results: `results_written_total` (by status)

**Traces**: Inherits trace context from session events when available.

## Dependencies
- Redis (event stream consumption)
- Session API (result storage)
- LLM provider (for judge evals)
- PromptKit SDK (eval execution)
