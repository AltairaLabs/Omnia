---
title: "Expose Functions as MCP Tools"
description: "Enable the MCP server on a function-mode AgentRuntime and consume it from other agents or external MCP clients"
sidebar:
  order: 8
---

Function-mode AgentRuntimes can expose themselves as **MCP (Model
Context Protocol) tools** alongside the existing `POST /functions/{name}`
HTTP route. When MCP is enabled, the pod runs a Streamable HTTP MCP
server on a separate port (default 9998), advertising the function's
input/output schemas natively.

This lets other Omnia agents — and any MCP-aware external client
(Claude Desktop, LangChain agents, etc.) — discover and call the
function as a typed tool.

If you haven't deployed a function yet, start with
[Define Functions](/how-to/define-functions/).

## Why MCP and not A2A?

Functions are structurally typed tools: single-shot, structured input,
structured output, schema-validated. That's the shape MCP is designed
for — `Tool { name, description, inputSchema }` is in the spec.

A2A is designed for agent-to-agent multi-turn dialog. Its `AgentSkill`
carries `inputModes` / `outputModes` (media-type strings) but no JSON
schemas, so exposing a function over A2A loses the typed contract.

## Enable MCP on a function

Add `facade.mcp.enabled: true` to your function-mode `AgentRuntime`:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: weather-lookup
spec:
  mode: function
  promptPackRef:
    name: weather-pack
  facade:
    type: rest
    port: 8080
    mcp:
      enabled: true
      # port defaults to 9998
  inputSchema:
    type: object
    required: ["latitude", "longitude"]
    properties:
      latitude: { type: "number" }
      longitude: { type: "number" }
  outputSchema:
    type: object
    required: ["temperature_celsius", "conditions"]
    properties:
      temperature_celsius: { type: "number" }
      conditions: { type: "string" }
```

After applying, the pod's Service has two ports:

- `8080` (`facade.port`) — HTTP `POST /functions/{name}` for non-agent
  consumers.
- `9998` (`facade.mcp.port`) — Streamable HTTP MCP for agent /
  typed-tool consumers.

The dashboard's `/functions` catalog shows an **MCP** badge next to
function names that opt in.

## Consume from an Omnia agent

Reference the function in the consuming agent's PromptPack as an MCP
tool source:

```yaml
mcp_servers:
  - name: weather
    url: http://weather-lookup.default.svc.cluster.local:9998/mcp
    headers:
      Authorization: Bearer ${WEATHER_TOKEN}
```

The agent's LLM sees the function as a typed tool with the exact
`inputSchema` declared on the function's AgentRuntime CRD. No
intermediate translation — the model receives the same JSON Schema
your function validates against.

## Consume from Claude Desktop or other external clients

Expose the function externally via your ingress, then point the client
at the public URL:

```json
{
  "mcpServers": {
    "weather-lookup": {
      "url": "https://omnia.example.com/functions/weather-lookup/mcp",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

Any MCP 2025-03-26-compatible client works — the wire protocol is the
standard `POST /mcp` Streamable HTTP transport with JSON-RPC 2.0
envelopes.

## Authentication

MCP shares Omnia's existing five-validator auth chain (SharedToken,
APIKey, OIDC, EdgeTrust, mgmt-plane). The same Bearer token that works
for the HTTP function route works for `POST /mcp`. See
[Configure Authentication](/how-to/configure-authentication/) for the
underlying mechanism.

Unauthenticated requests return `401` with a `WWW-Authenticate` header
pointing at the protected-resource metadata endpoint:

```http
HTTP/1.1 401 Unauthorized
WWW-Authenticate: Bearer realm="omnia",
                  resource_metadata="https://<host>/.well-known/oauth-protected-resource"
```

Spec-compliant MCP clients dereference that URL to discover how to
obtain a token. The metadata endpoint advertises an empty
`authorization_servers` array in v1 (Omnia accepts pre-issued tokens;
there is no Omnia-side OAuth issuer to direct clients to).

## Observability

Each `tools/call` opens a session row identical to a `POST
/functions/{name}` invocation — same Loki / dashboard / session-api
correlation. MCP-specific metrics:

- `omnia_mcp_requests_total{agent, namespace, method, status}` — counter.
- `omnia_mcp_request_duration_seconds{agent, namespace, method}` — histogram.
- `omnia_mcp_tool_invocations_total{agent, namespace, function, outcome}` —
  counter broken out by `outcome` (`ok`, `input_invalid`, `output_invalid`,
  `runtime_error`).

OpenTelemetry spans propagate from the MCP request through the runtime
gRPC call so distributed-trace consumers see the full path.

## v1 limitations

- **Streamable HTTP transport only.** The legacy HTTP+SSE transport
  (older Claude Desktop builds) is not supported. File an issue if you
  need it.
- **One pod = one function = one tool.** A multi-function MCP gateway
  is planned but not yet shipped; until then, point clients at each
  function's MCP endpoint individually.
- **No `prompts/*` or `resources/*` methods.** Only `initialize`,
  `tools/list`, and `tools/call` are advertised in the InitializeResult
  capabilities.
- **No customer-side OAuth discovery.** The protected-resource metadata
  endpoint exists but advertises an empty `authorization_servers`
  array. Bring your own pre-issued token.

## See also

- [Define Functions](/how-to/define-functions/) — author a function in the first place
- [Configure Authentication](/how-to/configure-authentication/) — the auth chain that MCP shares
- [MCP spec (2025-03-26)](https://modelcontextprotocol.io/specification/2025-03-26)
