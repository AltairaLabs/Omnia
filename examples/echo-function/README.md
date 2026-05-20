# echo-function

A minimum-viable function-mode AgentRuntime that demonstrates the
Functions Phase 1 surface (#1102 / #1103). It accepts a JSON object
with one `message` field and returns it as `{"echo": "<message>"}`.

The goal of this example is not to be useful — it's the smallest
working pattern you can copy and adapt for your own Function.

## What's here

| File | Purpose |
|------|---------|
| `agentruntime.yaml` | Function-mode AgentRuntime with input + output JSON Schemas and recording enabled. |
| `promptpack.yaml`   | Single-prompt PromptPack that instructs the model to copy its input into an `echo` field. |
| `provider.yaml`     | Provider + Secret for the LLM (Anthropic Claude by default; edit to taste). |

## Apply

```bash
# 1. Pick a namespace (or use your existing workspace).
kubectl create namespace echo-demo

# 2. Edit examples/echo-function/provider.yaml — replace REPLACE_ME
#    with your actual API key, or substitute a different provider
#    type per docs/reference/provider/.

# 3. Apply the manifests in dependency order.
kubectl -n echo-demo apply -f examples/echo-function/provider.yaml
kubectl -n echo-demo apply -f examples/echo-function/promptpack.yaml
kubectl -n echo-demo apply -f examples/echo-function/agentruntime.yaml

# 4. Wait for the runtime to come up.
kubectl -n echo-demo wait --for=condition=Ready agentruntime/echo-function --timeout=120s
```

## Invoke

```bash
kubectl -n echo-demo port-forward svc/echo-function 8080:8080 &

curl -X POST http://localhost:8080/functions/echo-function \
  -H "Content-Type: application/json" \
  -d '{"message": "hello functions"}'
```

Expected response shape:

```json
{
  "output": { "echo": "hello functions" },
  "invocation_id": "9c0e2c1f-...",
  "duration_ms": 730,
  "usage": {
    "input_tokens": 92,
    "output_tokens": 14,
    "cost_usd": 0.0001
  }
}
```

## Inspect

Each invocation is recorded as an ordinary session (tagged `function`),
so every existing session-related surface lights up:

- The dashboard's `/functions/echo-function` page shows the resolved
  input + output schemas.
- The Sessions page filters by this AgentRuntime's name and shows
  recent invocations alongside their tool calls, provider calls, and
  eval results.
- Retention follows the workspace's `SessionRetentionPolicy` — same
  rules as agent-mode runtimes.

## Adapt

To turn this into a real Function:

1. **Tighten the input schema.** Add required fields, `enum`s, length
   bounds. The facade rejects mis-shaped input with HTTP 400
   `input_invalid` — *before* the runtime is invoked, so you don't pay
   for bad calls.

2. **Tighten the output schema.** A loose output schema lets model
   drift reach your callers. A tight one surfaces drift as HTTP 502
   `output_invalid` with the raw model output in the response body
   so you can debug.

3. **Edit the PromptPack.** Replace the echo system prompt with the
   real task. See `config/samples/omnia_v1alpha1_promptpack.yaml` for
   the full pack surface (fragments, evals, validators, multi-prompt
   routing, etc.).

4. **Pick a model.** The default in `provider.yaml` is Claude Haiku
   for cheap iteration. Production Functions usually want a deterministic
   model and `temperature: 0` — both already set here.

5. **Configure retention.** Function invocations are sessions, so the
   workspace's `SessionRetentionPolicy` governs how long their rows
   live. Tighten the policy if PII or cost considerations require
   shorter retention for this Function specifically.

See [Define Functions](https://omnia.altairalabs.ai/how-to/define-functions/)
for the full guide.
