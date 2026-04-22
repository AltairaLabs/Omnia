---
title: "Configure Agent Authentication"
description: "Secure an AgentRuntime's WebSocket and HTTP facades"
sidebar:
  order: 5
---

This guide covers securing **agent endpoints** with the facade's built-in
validator chain. For dashboard authentication, see:

- [Configure Dashboard Authentication](/how-to/configure-dashboard-auth/) — set up user authentication
- [Authentication Architecture](/explanation/authentication/) — the full auth model

## How the facade authenticates a request

Each agent facade runs an **ordered chain of validators**. A request is
admitted as soon as any validator accepts it; otherwise the facade
returns 401.

By default the chain contains:

1. **management-plane** — admits dashboard-minted JWTs used by the "Try
   this agent" debug view.
2. Any data-plane validator configured on the AgentRuntime under
   `spec.externalAuth` (shared token, API keys, OIDC, edge-trust).

With **no** `spec.externalAuth` configured, the agent is reachable only
from the dashboard — there is no unauthenticated external path. This is
the secure default. Add at least one data-plane validator before
exposing an agent to customer traffic.

## Recipes

### 1. Dashboard access only (no external traffic)

Leave `spec.externalAuth` unset:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: internal-agent
spec:
  promptPackRef: { name: internal-pack }
  providerRefs:
    - name: openai
  toolRegistryRef: { name: internal-tools }
```

The dashboard proxy mints a short-lived RS256 token per request and
attaches it to the upstream WebSocket. External callers receive 401.

### 2. Shared bearer token (simplest external access)

Create a Secret holding the token:

```bash
kubectl create secret generic partner-agent-token \
  --namespace=my-workspace \
  --from-literal=token=$(openssl rand -hex 32)
```

Reference it on the AgentRuntime:

```yaml
spec:
  externalAuth:
    sharedToken:
      secretRef:
        name: partner-agent-token
      trustEndUserHeader: false  # flip to true only if the calling app is trusted
```

All callers share one token. Rotate by editing the Secret — the facade
reloads within 30s.

### 3. Per-caller API keys (managed in the dashboard)

Opt the agent in:

```yaml
spec:
  externalAuth:
    apiKeys:
      defaultRole: viewer    # viewer | editor | admin
      trustEndUserHeader: false
```

Then create keys from the dashboard's **Credentials** page — each key is
stored as a Secret keyed by its sha256 hash, with a scope and expiry.
Clients present `Authorization: Bearer <key>`; the facade looks up the
hash and admits the caller with the role stamped on the Secret.

No CRD edit is required when you add or revoke keys.

### 4. OIDC (customer IdP — no Istio required)

Point the facade at your IdP's issuer. The controller auto-fetches the
JWKS from the discovery document and refreshes every 6 hours:

```yaml
spec:
  externalAuth:
    oidc:
      issuer: "https://auth.example.com"
      audience: "my-agent"
      claimMapping:                 # optional; shown with defaults
        subject: sub
        role: omnia.role
        endUser: sub
```

The facade terminates and verifies the JWT in-process — no service mesh
is needed. A per-agent `agent-<name>-oidc-jwks` Secret appears in the
workspace namespace after the first reconcile; status conditions surface
any discovery or fetch failures:

```bash
kubectl get agentruntime my-agent -o yaml | yq '.status.conditions'
# look for type: OIDCJWKSReady
```

Provider-specific issuer values:

| Provider | `issuer` |
|----------|----------|
| Auth0 | `https://<tenant>.auth0.com/` |
| Okta | `https://<org>.okta.com/oauth2/default` |
| Google | `https://accounts.google.com` |
| Keycloak | `https://<host>/realms/<realm>` |
| Azure AD | `https://login.microsoftonline.com/<tenant-id>/v2.0` |

### 5. Edge-trust (Istio or API gateway terminates the JWT)

When an upstream edge (Istio `RequestAuthentication` with
`outputClaimToHeaders`, Envoy, or a commercial API gateway) already
terminates the JWT and injects claim headers, trust those headers
instead of re-verifying:

```yaml
spec:
  externalAuth:
    edgeTrust:
      headerMapping:              # defaults match the chart's Istio layout
        subject: x-user-id
        role: x-user-roles
        endUser: x-user-id
        email: x-user-email
      claimsFromHeaders:
        x-user-groups: groups     # exposed to ToolPolicy as identity.claims.groups
```

:::danger[Security requirement]
The edge **must** strip any inbound headers listed in `headerMapping` or
`claimsFromHeaders` before they reach the facade — otherwise any caller
can inject their own claims. The chart's `authentication.enabled=true`
Istio `AuthorizationPolicy` already does this for the default mapping;
verify it for any custom edge.
:::

## Combining validators

You can configure several at once — they all run:

```yaml
spec:
  externalAuth:
    allowManagementPlane: true   # dashboard debug view still works
    sharedToken: { secretRef: { name: partner-token } }
    apiKeys:    { defaultRole: viewer }
    oidc:       { issuer: "https://auth.example.com", audience: "my-agent" }
```

The facade tries each in order and admits the first that accepts the
request. Setting `allowManagementPlane: false` blocks the dashboard
debug view for this agent — useful for paranoid workloads that want
data-plane-only isolation.

## Connect with a token

### WebSocket (browser or Node)

```javascript
const token = await getAccessToken();
const ws = new WebSocket('wss://agents.example.com/my-agent/ws', {
  headers: { 'Authorization': `Bearer ${token}` },
});
```

### CLI

```bash
wscat -H "Authorization: Bearer $TOKEN" \
  -c wss://agents.example.com/my-agent/ws

websocat -H "Authorization: Bearer $TOKEN" \
  wss://agents.example.com/my-agent/ws
```

## Troubleshooting

### Every request returns 401

Check the facade logs — rejection telemetry is emitted at `V(1)`:

```bash
kubectl logs -l app.kubernetes.io/name=omnia-agent -c facade --tail=50
# look for: "auth middleware rejected request" reason=... path=...
```

Common causes:

- **`reason=no validator admitted`** — no `spec.externalAuth` validator
  is configured, or the caller presented no credential.
- **`reason=invalid credential`** — the credential format/signature is
  wrong (expired JWT, wrong shared-token, unknown API key hash).

### OIDC tokens are rejected

```bash
kubectl get secret agent-my-agent-oidc-jwks -o yaml
kubectl get agentruntime my-agent -o yaml | yq '.status.conditions[] | select(.type=="OIDCJWKSReady")'
```

If the Secret is missing or empty, the controller couldn't reach the
issuer — check reachability from the operator pod and confirm the
issuer URL has no trailing slash.

### Edge-trust headers aren't populated downstream

Tool handlers see edge-trust claims as `X-Omnia-Claim-<name>` headers
and ToolPolicy sees them as `identity.claims.<name>`. If they're
missing:

1. Confirm the edge (Istio/gateway) is injecting the expected headers —
   use a debug container or Envoy access logs.
2. Confirm the edge **strips inbound** versions of those headers so
   clients can't spoof them.
3. Confirm `spec.externalAuth.edgeTrust.claimsFromHeaders` lists the
   inbound header names exactly as the edge emits them (header names
   are case-insensitive).

## Migrating from legacy A2A shared-token

Previously `spec.a2a.authentication.secretRef` set a shared bearer on
the A2A HTTP endpoint only. The controller now projects that value onto
`spec.externalAuth.sharedToken.secretRef` in memory so both the WS and
A2A facades validate against it. Move new work to
`spec.externalAuth.sharedToken` directly — the legacy field will be
removed in a future release.
