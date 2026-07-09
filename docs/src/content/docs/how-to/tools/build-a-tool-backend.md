---
title: "Build a tool backend"
description: "The wire contract a tool backend must implement for Omnia's HTTP and gRPC handlers"
sidebar:
  order: 5
---

The [ToolRegistry reference](/reference/core/toolregistry/) explains how to
*configure* `http` and `grpc` handlers — endpoint, auth, retry policy. This
guide covers what the backend service behind that endpoint must actually
implement to be called successfully.

## HTTP tool backend

The runtime sends an HTTP request to `httpConfig.endpoint` using
`httpConfig.method` (default `POST`).

### Request

In the simple case — none of the [advanced shaping fields](/how-to/tools/advanced-http-tools/)
(`urlTemplate`, `queryParams`, `headerParams`, `staticQuery`, `staticBody`,
`bodyMapping`) are set — the tool-call arguments are sent as the JSON request
body verbatim, with `Content-Type: application/json` by default. For `GET` and
`DELETE`, the arguments go in the query string instead, and there is no body.

The runtime also sends context headers (for example `x-omnia-tool-name`,
`x-omnia-agent-name`, `x-omnia-session-id`). A backend may read them for
logging/routing but doesn't need to.

### Response

**Success is HTTP 2xx only.** Any non-2xx status is treated as a hard
failure: the response body is truncated to roughly 512 bytes and returned as
an error string — it is **not** parsed as structured data. Keep error
responses short and human-readable; don't rely on the LLM parsing a
structured error body out of a failure response.

On a 2xx response:

- If the body is **valid JSON**, it is passed through to the LLM (after any
  `redact`/`responseMapping` configured on the handler).
- If the body is **not valid JSON**, it is wrapped as `{"result": "<body as
  string>"}` before the LLM sees it.

### Minimal contract

Accept the arguments JSON at your endpoint and return **2xx + JSON**. There is
no required envelope and no correlation ID to echo back — the runtime matches
the response to the in-flight tool call itself.

## gRPC tool backend — the Omnia Tool protocol

A `grpc` handler talks to your service using a small protocol Omnia defines,
not an arbitrary gRPC API. The authoritative source is
[`api/proto/tools/v1/tools.proto`](https://github.com/AltairaLabs/Omnia/blob/main/api/proto/tools/v1/tools.proto)
(Go package `github.com/altairalabs/omnia/pkg/tools/v1`):

```proto
syntax = "proto3";
package omnia.tools.v1;

service ToolService {
  rpc Execute(ToolRequest) returns (ToolResponse);
  rpc ListTools(ListToolsRequest) returns (ListToolsResponse);
}

message ToolRequest {
  string tool_name = 1;
  string arguments_json = 2;   // tool arguments as a JSON string
  map<string, string> metadata = 3;
}
message ToolResponse {
  string result_json = 1;      // tool result as a JSON string
  bool is_error = 2;
  string error_message = 3;    // set when is_error is true
}
message ListToolsRequest {}
message ListToolsResponse { repeated ToolInfo tools = 1; }
message ToolInfo {
  string name = 1;
  string description = 2;
  string input_schema = 3;     // JSON Schema string
}
```

The runtime calls `/omnia.tools.v1.ToolService/Execute` with
`ToolRequest{tool_name, arguments_json}` — the arguments are passed as an
opaque JSON string, not unpacked into typed fields. Your backend returns
`result_json` (also an opaque JSON string) on success, or sets `is_error =
true` and `error_message` for a tool-level failure.

### What to implement

- **`Execute`** is required — this is the only RPC the runtime calls for a
  handler with an inline `tool` definition.
- **`ListTools`** is optional. It's used when a `grpc` handler omits an inline
  `tool` block and relies on self-discovery instead — the runtime calls
  `ListTools` to learn the tool's name, description, and `input_schema`.

### Wiring and TLS

Endpoint (`host:port`) and TLS options (`tls`, `tlsCertPath`, `tlsKeyPath`,
`tlsCAPath`, `tlsInsecureSkipVerify`) are configured on the handler's
`grpcConfig` — see the [ToolRegistry reference's gRPC handler
section](/reference/core/toolregistry/#grpc-handler). Auth (bearer/basic,
ServiceAccount, workload identity) is layered on top via the handler's `auth`
stanza — see [Authenticate tools](/how-to/tools/authenticate-tools/).

## See also

- [ToolRegistry CRD reference](/reference/core/toolregistry/)
- [Advanced HTTP tools](/how-to/tools/advanced-http-tools/)
- [Authenticate tools](/how-to/tools/authenticate-tools/)
- [Test tools](/how-to/tools/test-tools/) — exercise a backend before wiring it to an agent
