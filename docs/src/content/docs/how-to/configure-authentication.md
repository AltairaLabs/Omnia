---
title: "Configure Authentication"
description: "Secure agent endpoints with JWT authentication"
sidebar:
  order: 5
---


Omnia supports JWT-based authentication for agent endpoints using Istio's RequestAuthentication. This allows you to integrate with any OIDC provider (Auth0, Okta, Keycloak, Google, etc.).

## Prerequisites

- Istio installed in your cluster
- Omnia Helm chart with Istio integration enabled
- An OIDC provider with a JWKS endpoint

## Enable JWT Authentication

Configure authentication in your Helm values:

```yaml
istio:
  enabled: true

authentication:
  enabled: true
  jwt:
    issuer: "https://your-auth-provider.com"
    jwksUri: "https://your-auth-provider.com/.well-known/jwks.json"
    audiences:
      - "your-api-audience"
```

Apply with Helm:

```bash
helm upgrade --install omnia oci://ghcr.io/altairalabs/omnia \
  --namespace omnia-system \
  -f values.yaml
```

## Provider Examples

### Auth0

```yaml
authentication:
  enabled: true
  jwt:
    issuer: "https://your-tenant.auth0.com/"
    jwksUri: "https://your-tenant.auth0.com/.well-known/jwks.json"
    audiences:
      - "https://your-api-identifier"
```

### Okta

```yaml
authentication:
  enabled: true
  jwt:
    issuer: "https://your-org.okta.com/oauth2/default"
    jwksUri: "https://your-org.okta.com/oauth2/default/v1/keys"
    audiences:
      - "api://default"
```

### Google

```yaml
authentication:
  enabled: true
  jwt:
    issuer: "https://accounts.google.com"
    jwksUri: "https://www.googleapis.com/oauth2/v3/certs"
    audiences:
      - "your-client-id.apps.googleusercontent.com"
```

### Keycloak

```yaml
authentication:
  enabled: true
  jwt:
    issuer: "https://keycloak.example.com/realms/your-realm"
    jwksUri: "https://keycloak.example.com/realms/your-realm/protocol/openid-connect/certs"
    audiences:
      - "your-client-id"
```

## Forward Claims to Agents

Extract JWT claims and pass them as headers to your agents:

```yaml
authentication:
  enabled: true
  jwt:
    issuer: "https://your-auth-provider.com"
    forwardOriginalToken: true
    outputClaimToHeaders:
      - header: x-user-id
        claim: sub
      - header: x-user-email
        claim: email
      - header: x-user-roles
        claim: roles
```

Your agent can then read these headers from the WebSocket upgrade request.

## Require Specific Claims

Restrict access to users with specific claims:

```yaml
authentication:
  enabled: true
  jwt:
    issuer: "https://your-auth-provider.com"
  authorization:
    requiredClaims:
      - claim: "scope"
        values: ["agents:access"]
      - claim: "role"
        values: ["user", "admin"]
```

## Exclude Paths from Authentication

Allow unauthenticated access to specific paths:

```yaml
authentication:
  enabled: true
  jwt:
    issuer: "https://your-auth-provider.com"
  authorization:
    excludePaths:
      - /healthz
      - /readyz
      - /metrics
```

## Connect with a Token

### WebSocket Client

Include the JWT in the WebSocket connection:

```javascript
const token = await getAccessToken();
const ws = new WebSocket('wss://agents.example.com/my-agent/ws', {
  headers: {
    'Authorization': `Bearer ${token}`
  }
});
```

### Using wscat

```bash
wscat -H "Authorization: Bearer $TOKEN" \
  -c wss://agents.example.com/my-agent/ws
```

### Using websocat

```bash
websocat -H "Authorization: Bearer $TOKEN" \
  wss://agents.example.com/my-agent/ws
```

## Troubleshooting

### Check RequestAuthentication

Verify the Istio RequestAuthentication was created:

```bash
kubectl get requestauthentication -n omnia-system
kubectl describe requestauthentication omnia-jwt-auth -n omnia-system
```

### Check AuthorizationPolicy

Verify the authorization policy:

```bash
kubectl get authorizationpolicy -n omnia-system
kubectl describe authorizationpolicy omnia-require-jwt -n omnia-system
```

### Debug Token Issues

If connections are rejected, check:

1. **Token expiry**: Ensure the token hasn't expired
2. **Issuer match**: The `iss` claim must exactly match the configured issuer
3. **Audience match**: If audiences are configured, the `aud` claim must match
4. **JWKS accessibility**: Istio must be able to reach the JWKS URI

View Istio proxy logs for auth errors:

```bash
kubectl logs -l app.kubernetes.io/name=omnia-agent -c istio-proxy -n omnia-system
```

## Disable Authentication

To disable authentication (not recommended for production):

```yaml
authentication:
  enabled: false
```
