---
title: "Advanced HTTP tools"
description: "Shape HTTP tool requests and responses with URL templates, static injection, JMESPath mapping, redaction, and retry policies"
sidebar:
  order: 2
---

The `http` handler's `httpConfig` block has a set of request- and
response-shaping fields that let you adapt an existing HTTP API into a clean tool
surface for the LLM — without writing a wrapper service. None of these fields are
exposed in the dashboard UI; they are CRD-only.

All examples below are fields of a single handler's `httpConfig`. See the
[ToolRegistry reference](/reference/core/toolregistry/) for the full schema.

## Path parameters with `urlTemplate`

`urlTemplate` is a Go `text/template` evaluated against the tool arguments. When
set, `endpoint` is used as the base URL and `urlTemplate` builds the path:

```yaml
httpConfig:
  endpoint: https://api.example.com
  method: GET
  urlTemplate: "/users/{{.user_id}}/orders/{{.order_id}}"
```

Arguments consumed by the template still come from the tool's `inputSchema`.

## Send arguments as query params or headers

By default arguments go into the JSON request body. Redirect specific arguments:

```yaml
httpConfig:
  endpoint: https://api.example.com/search
  method: GET
  # These arg names become URL query parameters instead of body fields:
  queryParams: [query, limit]
  # Map an arg into a request header via a template:
  headerParams:
    X-Customer-ID: "{{.customer_id}}"
```

## Inject fixed values the LLM never sees

`staticQuery` and `staticBody` add fixed values to every request. They are
**invisible to the LLM** — useful for API keys carried in a query string, tenant
scoping, or fixed flags:

```yaml
httpConfig:
  endpoint: https://api.example.com/v1/search
  method: POST
  staticQuery:
    api_version: "2024-01"
  staticBody:
    source: omnia
    include_metadata: true
```

:::note[Prefer the auth stanza for credentials]
Use `staticQuery`/`staticBody` for non-secret constants. For credentials, use the
[auth stanza](/how-to/tools/authenticate-tools/) so the value stays out of the
tools ConfigMap.
:::

## Reshape request and response with JMESPath

`bodyMapping` reshapes the outgoing request body; `responseMapping` filters and
reshapes the response before it is returned to the LLM (smaller, cleaner payloads
mean cheaper, more reliable tool use):

```yaml
httpConfig:
  endpoint: https://api.example.com/v1/products
  method: POST
  # Reshape the request body before sending:
  bodyMapping: "{ q: query, size: limit }"
  # Return only the fields the model needs:
  responseMapping: "results[].{name: name, price: price, inStock: available}"
```

## Redact fields from logs and traces

`redact` lists response field names to exclude from logs and tracing (they are
still returned to the model — this controls observability output, not the tool
result):

```yaml
httpConfig:
  endpoint: https://api.example.com/v1/customer
  method: GET
  redact: [ssn, date_of_birth]
```

## Retry policy

Retries are configured per transport (the handler-level `retries` field was
removed). For HTTP:

```yaml
httpConfig:
  endpoint: https://api.example.com/v1/data
  method: GET
  retryPolicy:
    maxAttempts: 3              # total attempts incl. the first (1–10; 1 = no retries)
    initialBackoff: "200ms"
    backoffMultiplier: "2.0"
    maxBackoff: "10s"
    retryOn: [429, 500, 502, 503, 504]   # default: [408, 429, 500, 502, 503, 504]
    retryOnNetworkError: true            # retry connection/DNS/timeout errors
    respectRetryAfter: true              # honor Retry-After on 429/503
```

| Field | Default | Notes |
|-------|---------|-------|
| `maxAttempts` | — (required) | 1–10; `1` disables retries |
| `initialBackoff` | `100ms` | delay before the first retry |
| `backoffMultiplier` | `2.0` | decimal string; must be ≥ 1.0 |
| `maxBackoff` | `30s` | upper bound; must be ≥ `initialBackoff` |
| `retryOn` | `[408, 429, 500, 502, 503, 504]` | empty list disables status-based retry |
| `retryOnNetworkError` | `true` | retries pre-response failures |
| `respectRetryAfter` | `true` | honors `Retry-After` on 429/503 |

The same `retryPolicy` shape applies to `openAPIConfig`. `grpcConfig` and
`mcpConfig` have their own retry-policy variants (gRPC keys off status-code names
like `UNAVAILABLE`).

## See also

- [ToolRegistry CRD reference](/reference/core/toolregistry/)
- [Authenticate tools](/how-to/tools/authenticate-tools/)
