---
title: "Authenticate tools"
description: "Attach bearer/basic credentials or projected ServiceAccount tokens to tool backends with the handler-level auth stanza"
sidebar:
  order: 1
---

Tool backends usually require a credential. Omnia configures this with the
**handler-level `auth` stanza** on a ToolRegistry handler. This stanza is not
exposed in the dashboard UI — it is CRD-only.

`auth` is a sibling of `httpConfig`/`openAPIConfig`/`grpcConfig`/`mcpConfig`, so
the same shape applies across handler types. The runtime attaches the resolved
credential as an HTTP `Authorization` header, gRPC `authorization` metadata, or
an MCP transport header.

## Prerequisites

- A ToolRegistry with at least one `http`, `openapi`, `grpc`, or `mcp` handler
- For `bearer`/`basic`: a Kubernetes Secret in the **same namespace** as the AgentRuntime holding the credential
- An AgentRuntime that references the ToolRegistry via `toolRegistryRef`

## Auth types

| `auth.type` | Credential | Sent as |
|-------------|-----------|---------|
| `none` (default) | — | no credential |
| `bearer` | `secretRef` value | `Authorization: Bearer <token>` |
| `basic` | `secretRef` value `user:password` | `Authorization: Basic <base64>` |
| `serviceAccount` | projected, audience-bound SA token | `Authorization: Bearer <token>` |
| `workloadIdentity` | hosted same-cloud identity | *(not yet available — rejected at reconcile)* |

## Bearer / basic token

Create the credential Secret, then reference it from the handler:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: weather-credentials
  namespace: agents        # same namespace as the AgentRuntime
type: Opaque
stringData:
  token: "sk-live-abc123"  # for basic auth, use "username:password"
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ToolRegistry
metadata:
  name: weather-tools
  namespace: agents
spec:
  handlers:
    - name: weather
      type: http
      httpConfig:
        endpoint: https://api.weather.example/v1/forecast
        method: GET
      auth:
        type: bearer
        secretRef:
          name: weather-credentials
          key: token
      tool:
        name: get_weather
        description: "Get the weather forecast for a city"
        inputSchema:
          type: object
          properties:
            city: { type: string }
          required: [city]
```

### How the credential is handled

When an AgentRuntime references this registry, the operator resolves each
handler's `secretRef` into a single operator-managed Secret named
`<agentruntime-name>-tool-secrets`, with **one key per authenticated handler**
(the key is the handler name). That Secret is mounted read-only into the runtime
container at `/etc/omnia/tool-secrets`.

The token value **never enters the tools ConfigMap** — the ConfigMap references
it only by path. For the example above, the rendered tool config contains:

```yaml
authType: bearer
authTokenPath: /etc/omnia/tool-secrets/weather
endpoint: https://api.weather.example/v1/forecast
```

This keeps the secret out of the (non-secret) ConfigMap while still making it
available to the runtime at call time.

A **missing Secret or key fails the AgentRuntime reconcile** — the runtime is not
started with a broken auth config, and it never silently sends an
unauthenticated request.

### Verify

```bash
# The operator-managed companion Secret exists, keyed by handler name:
kubectl get secret <agentruntime>-tool-secrets -n agents -o jsonpath='{.data}'

# The tools ConfigMap references the token by PATH, not value:
kubectl get configmap <agentruntime>-tools -n agents -o yaml | grep authTokenPath

# The runtime Deployment mounts the companion Secret:
kubectl get deployment <agentruntime> -n agents \
  -o jsonpath='{.spec.template.spec.volumes[*].secret.secretName}'
```

## ServiceAccount token (`serviceAccount`)

For in-cluster backends that can validate a Kubernetes token, use an
audience-bound **projected ServiceAccount token**. The operator projects the
token into the runtime; the backend validates it via the TokenReview API. No
long-lived secret is stored.

```yaml
- name: internal-api
  type: http
  httpConfig:
    endpoint: http://internal.svc.cluster.local/api
  auth:
    type: serviceAccount
    serviceAccount:
      audience: internal-api      # the backend validates the token against this audience
  tool:
    name: internal_action
    description: "Call the internal API"
    inputSchema:
      type: object
```

The token is sent as `Authorization: Bearer <token>`. `serviceAccount.audience`
is required.

## workloadIdentity (not yet available)

`auth.type: workloadIdentity` is accepted by the schema but **rejected at
reconcile** — its credential resolver is the Enterprise policy broker, which is a
later release. It is also rejected when the pod's Provider itself uses workload
identity, because tool egress must not reuse the runtime's ambient cloud
identity.

## Transport constraints

- Auth works on `http`, `openapi`, `grpc`, and MCP `sse`/`streamable-http` transports.
- Auth on an MCP **`stdio`** transport is **rejected** — a subprocess has no header channel.

## Migrating off the deprecated fields

The per-config `httpConfig.authType` / `httpConfig.authSecretRef` (and the
`openAPIConfig` equivalents) are deprecated. They still work and are normalized
into the `auth` stanza, but setting **both** an `auth` stanza and a legacy
`authType`/`authSecretRef` on the same handler is **rejected**. Prefer the `auth`
stanza for new registries.

```yaml
# before (deprecated)
httpConfig:
  endpoint: https://api.example/v1
  authType: bearer
  authSecretRef:
    name: creds
    key: token

# after
httpConfig:
  endpoint: https://api.example/v1
auth:
  type: bearer
  secretRef:
    name: creds
    key: token
```

## See also

- [ToolRegistry CRD reference](/reference/core/toolregistry/)
- [Advanced HTTP tools](/how-to/tools/advanced-http-tools/)
- [Configure tool policies](/how-to/security/configure-tool-policies/) — CEL allow/deny and header injection on top of authenticated tools
