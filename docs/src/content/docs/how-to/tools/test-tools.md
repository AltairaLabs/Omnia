---
title: "Test a tool before wiring it to an agent"
description: "Use the dashboard's Test this tool panel to exercise a handler through the real runtime adapters"
sidebar:
  order: 6
---

Before wiring a new handler into an AgentRuntime, you can exercise it
directly from the dashboard's **Test this tool** panel (backed by the
operator's tool-test API). It calls the tool through the **real** runtime
adapters — HTTP, gRPC, MCP, and OpenAPI — and returns the result, so it's a
faithful way to check a backend without deploying an agent first.

## What the response contains

| Field | Description |
|-------|-------------|
| `success` | Whether the tool call succeeded |
| `result` | The tool's JSON response, on success |
| `error` | The error message, on failure |
| `durationMs` | Execution time in milliseconds |
| `handlerType` | The handler type that was used (`http`, `grpc`, `mcp`, `openapi`) |
| `warning` | A non-fatal caveat about the test — set when auth could not be replicated (see below) |
| `validation` | JSON-Schema checks of the request against `inputSchema` and the response against `outputSchema`, when those schemas are set |

## Auth caveats

The tool-test path resolves `bearer`/`basic` secret-backed credentials the same
way the runtime does and attaches them to the test request — for **all** handler
types (`http`, `openapi`, `grpc`, and `mcp`).

Some auth cannot be replicated by the test, and when that happens the response's
`warning` field explains why — the call still goes out, but **without** that
credential, so treat the result with caution (a failure may just be the backend
correctly rejecting an unauthenticated call):

- **`serviceAccount` / `workloadIdentity`** — a running agent gets these from a
  projected ServiceAccount token or the pod's ambient cloud identity, neither of
  which the operator's tool-test process has.
- **Auth on a `stdio` MCP transport** — a subprocess has no header channel, so no
  credential can be sent (the running agent rejects this configuration).

Confirm auth separately for those cases — for example by checking the runtime
logs once the tool is wired to a running agent.

## Reachability

The call originates from the **operator pod**, not from an agent pod. It can
only reach whatever the operator pod's network can reach — a backend that's
only reachable from the agent's namespace or node pool may fail the test even
though it will work fine once wired to a real AgentRuntime.

## See also

- [Build a tool backend](/how-to/tools/build-a-tool-backend/)
- [Authenticate tools](/how-to/tools/authenticate-tools/)
- [ToolRegistry CRD reference](/reference/core/toolregistry/)
