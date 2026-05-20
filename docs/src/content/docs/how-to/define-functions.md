---
title: "Define Functions"
description: "Author a function-mode AgentRuntime: one-shot HTTP invocations with structured input and output schemas"
sidebar:
  order: 7
---

Functions are AgentRuntimes that expose a single, structured-I/O HTTP
endpoint instead of a long-lived WebSocket conversation. They are the
right shape when you want to call a PromptPack like a service:
deterministic input, validated output, no session state, no streaming.

Typical use cases:

- Summarising a document on demand.
- Extracting structured data from free-text input.
- Wrapping a PromptPack as a callable API consumed by other services.
- Memory summarisation, evaluation aggregation, and similar
  "pack + input → output" workloads.

If you want a chat surface, browser console, or long-lived session,
[deploy an agent-mode AgentRuntime](/how-to/expose-agents/) instead.

## When agent vs function

| Concern | Agent mode | Function mode |
|---------|------------|---------------|
| Transport | WebSocket (`/ws`) | HTTP POST (`/functions/{name}`) |
| Conversation state | Yes (sessions) | No (one-shot) |
| Input validation | None at the boundary | Required JSON Schema |
| Output validation | None at the boundary | Required JSON Schema |
| Audit trail | Sessions table | `function_invocations` table (opt-in) |
| Browser UI | Console / dashboard chat | None |

## CRD shape

A function-mode AgentRuntime sets `spec.mode: function` and declares
two JSON Schemas: one for the request body, one for the model output.

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: summarizer
  namespace: my-workspace
spec:
  mode: function
  promptPackRef:
    name: summarizer-pack
  facade:
    type: grpc          # function mode requires the gRPC facade
    port: 8080
  providers:
    - name: default
      providerRef:
        name: claude-sonnet
  inputSchema:
    type: object
    required: ["text"]
    properties:
      text:
        type: string
        description: "Text to summarise"
      maxWords:
        type: integer
        minimum: 10
        maximum: 500
  outputSchema:
    type: object
    required: ["summary"]
    properties:
      summary:
        type: string
  invocationRecording:
    state: enabled      # opt-in audit trail; defaults to disabled
```

The CEL validation gates on the CRD enforce:

- `spec.mode == "function"` requires both `spec.inputSchema` and
  `spec.outputSchema`.
- `spec.mode == "agent"` (the default) forbids those schemas.
- `spec.mode == "function"` rejects `spec.facade.type == "websocket"` —
  use `grpc`.

Apply the resource the usual way:

```bash
kubectl apply -f summarizer.yaml
kubectl wait --for=condition=Ready agentruntime/summarizer -n my-workspace --timeout=60s
```

## Invoke it

The facade pod exposes `POST /functions/{name}` where `{name}` matches
the AgentRuntime's `metadata.name`.

```bash
kubectl port-forward -n my-workspace svc/summarizer 8080:8080 &

curl -X POST http://localhost:8080/functions/summarizer \
  -H "Content-Type: application/json" \
  -d '{"text": "Omnia is a Kubernetes operator for AI agent deployments.", "maxWords": 20}'
```

A successful call returns the model output plus metadata:

```json
{
  "output": { "summary": "Omnia is a K8s operator for managing AI agents." },
  "invocation_id": "9c0e2c1f-...",
  "duration_ms": 842,
  "usage": {
    "input_tokens": 41,
    "output_tokens": 12,
    "cost_usd": 0.0008
  }
}
```

### Error responses

The facade owns the schema validation boundary. Status codes:

| Code | Meaning |
|------|---------|
| 200  | Success — response body validated against `spec.outputSchema`. |
| 400  | `input_invalid` — request body failed `spec.inputSchema`. Body includes the validator error. |
| 401  | `unauthorized` — authentication chain (mgmt-plane + data-plane) rejected the request. |
| 404  | `function_not_found` — no function-mode runtime matched `{name}` on this facade. |
| 405  | `method_not_allowed` — only `POST` is allowed. |
| 413  | `payload_too_large` — request body exceeded 1 MiB. |
| 415  | `unsupported_media_type` — `Content-Type` must be `application/json`. |
| 502  | Either `runtime_error` (the runtime sidecar failed) or `output_invalid` (the model output failed `spec.outputSchema`). The body includes `raw_output` in the latter case so you can debug the pack. |

## Authentication

Function routes reuse the same data-plane + mgmt-plane validator chain
as the WebSocket path. By default every request must present a
credential admitted by at least one configured validator. For dev /
CI clusters with no externalAuth and an unreadable mgmt-plane public
key, set `OMNIA_FACADE_ALLOW_UNAUTHENTICATED=true` on the facade pod
to fall through. Production must never set that flag.

See [Configure Authentication](/how-to/configure-authentication/) for
the full validator catalogue.

## Invocation recording

When `spec.invocationRecording.state` is `enabled`, the facade writes
one audit row per call to the `function_invocations` table in
session-api. Each row carries:

- A SHA-256 hash of the input (raw input is not stored — this keeps
  PII surface small).
- The raw output JSON.
- Status (`success` / `input_invalid` / `output_invalid` / `runtime_error`).
- Latency, cost, and the OTel trace id of the producing span (when
  the request context carried a valid span).

Recording is **best-effort**: a session-api outage logs but does not
fail the user-facing call. The dashboard surfaces these rows on the
`/functions/{name}` detail page with 1h / 24h / 7d window presets and
per-window latency + cost sparklines.

To turn recording off, omit `spec.invocationRecording` entirely or set
`state: disabled`.

## Tips

- **Output schemas should be tight.** The facade returns `502
  output_invalid` with the raw model output when the response does
  not match. Tight schemas surface model drift early; loose schemas
  let bad outputs reach the caller.
- **One Function per AgentRuntime.** The facade pod serves exactly
  the function whose `metadata.name` matches its own — there is no
  per-pod multiplexing in Phase 1.
- **Schema changes require a Deployment rollout.** The facade compiles
  schemas once at startup, so changing `spec.inputSchema` /
  `spec.outputSchema` triggers a Deployment update (existing behaviour
  for any CRD-driven config).
- **Use the `examples/echo-function/` directory** in the repository
  as a working starting point — it ships with a PromptPack, Provider,
  and AgentRuntime that you can apply and invoke in under a minute.
