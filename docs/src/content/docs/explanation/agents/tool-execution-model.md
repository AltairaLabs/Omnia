---
title: "Tool execution model"
description: "Where each tool type actually runs, and what that means for debugging"
sidebar:
  order: 7
---

Tools in Omnia don't all execute in the same place. Where a tool runs
determines whether you can see its calls in the WebSocket stream — and
getting this wrong is a common source of confusion when debugging "why isn't
my tool call showing up?"

## Client tools run in the browser

A handler with `type: client` (endpoint `client://browser`) is **forwarded**
to the connected browser over the WebSocket facade as a `tool_call` message.
The browser executes the tool locally and sends back a `tool_result` message.

**Client tools are the only tools visible in the WebSocket stream.** If
you're inspecting WS traffic to debug a tool call and you don't see it, the
tool most likely isn't a client tool.

## Server and platform tools run in the runtime

Everything else executes **in the runtime container**, server-side, and is
**never** forwarded over the WebSocket — the browser never sees these calls:

- **Server tools**: `http`, `grpc`, `mcp`, and `openapi` handlers
- **Platform tools**: capabilities registered via the PromptKit SDK, such as
  memory and skills

## Debugging: use the right signal

All tool calls — client and server — are recorded by the runtime to
session-api and are readable via the session's `/tool-calls` endpoint. So:

- To debug a **client** tool, the WebSocket stream is a valid signal.
- To debug a **server or platform** tool, use runtime logs or the session's
  `/tool-calls` endpoint — it will never appear on the WebSocket.

## WebSocket message shapes

From `api/websocket/asyncapi.yaml`:

- `tool_call` carries a `ToolCallInfo`: `id`, `name`, `arguments`,
  `consent_message`, `categories`.
- `tool_result` carries a `ClientToolResultInfo`: `call_id` (matches the
  originating `ToolCallInfo.id`), and either `result` or `error`.

Notably, `ToolCallInfo` has **no `execution` field** — there's nothing to
discriminate on, because every tool call that appears on the WebSocket is
client-side by definition.

## See also

- [Build a tool backend](/how-to/tools/build-a-tool-backend/)
- [Test a tool before wiring it to an agent](/how-to/tools/test-tools/)
- [Client-side tools](/how-to/tools/client-tools/)
