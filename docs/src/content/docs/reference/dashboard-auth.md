---
title: "Dashboard Authentication Reference"
description: "Complete reference for dashboard authentication configuration"
sidebar:
  order: 6
---


This document provides a complete reference for all dashboard authentication configuration options.

## Environment Variables

### Core Settings

| Variable | Description | Default |
|----------|-------------|---------|
| `OMNIA_AUTH_MODE` | Authentication mode: `anonymous`, `proxy`, or `oauth` | `anonymous` |
| `OMNIA_BASE_URL` | Base URL for OAuth callbacks (required for OAuth mode) | - |

### Session Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `OMNIA_SESSION_SECRET` | 32+ character secret for session encryption | Random (dev only) |
| `OMNIA_SESSION_COOKIE_NAME` | Name of the session cookie | `omnia_session` |
| `OMNIA_SESSION_TTL` | Session lifetime in seconds | `86400` (24 hours) |

:::caution
`OMNIA_SESSION_SECRET` must be set in production. Without it, sessions are encrypted with a random key that changes on restart.
:::

### Role Mapping

| Variable | Description | Default |
|----------|-------------|---------|
| `OMNIA_AUTH_ROLE_ADMIN_GROUPS` | Comma-separated groups that get admin role | - |
| `OMNIA_AUTH_ROLE_EDITOR_GROUPS` | Comma-separated groups that get editor role | - |
| `OMNIA_AUTH_ANONYMOUS_ROLE` | Role for anonymous users | `viewer` |

### Proxy Mode Settings

| Variable | Description | Default |
|----------|-------------|---------|
| `OMNIA_AUTH_PROXY_HEADER_USER` | Header containing username | `X-Forwarded-User` |
| `OMNIA_AUTH_PROXY_HEADER_EMAIL` | Header containing email | `X-Forwarded-Email` |
| `OMNIA_AUTH_PROXY_HEADER_GROUPS` | Header containing groups (comma-separated) | `X-Forwarded-Groups` |
| `OMNIA_AUTH_PROXY_HEADER_DISPLAY_NAME` | Header containing display name | `X-Forwarded-Preferred-Username` |
| `OMNIA_AUTH_PROXY_AUTO_SIGNUP` | Auto-create users on first login | `true` |

### OAuth Mode Settings

| Variable | Description | Default |
|----------|-------------|---------|
| `OMNIA_OAUTH_PROVIDER` | Provider type (see below) | `generic` |
| `OMNIA_OAUTH_CLIENT_ID` | OAuth client ID | - |
| `OMNIA_OAUTH_CLIENT_SECRET` | OAuth client secret | - |
| `OMNIA_OAUTH_CLIENT_SECRET_FILE` | Path to file containing client secret | - |
| `OMNIA_OAUTH_ISSUER_URL` | OIDC issuer URL (required for generic) | - |
| `OMNIA_OAUTH_SCOPES` | Comma-separated OAuth scopes | Provider-specific |

### OAuth Claim Mapping

| Variable | Description | Default |
|----------|-------------|---------|
| `OMNIA_OAUTH_CLAIM_USERNAME` | Claim for username | `preferred_username` |
| `OMNIA_OAUTH_CLAIM_EMAIL` | Claim for email | `email` |
| `OMNIA_OAUTH_CLAIM_DISPLAY_NAME` | Claim for display name | `name` |
| `OMNIA_OAUTH_CLAIM_GROUPS` | Claim for groups | `groups` |

### Provider-Specific Settings

| Variable | Description | Required For |
|----------|-------------|--------------|
| `OMNIA_OAUTH_AZURE_TENANT_ID` | Azure AD tenant ID | Azure |
| `OMNIA_OAUTH_OKTA_DOMAIN` | Okta organization domain | Okta |

### API Keys

| Variable | Description | Default |
|----------|-------------|---------|
| `OMNIA_AUTH_API_KEYS_ENABLED` | Enable API key authentication | `true` |
| `OMNIA_AUTH_API_KEYS_MAX_PER_USER` | Maximum keys per user | `10` |
| `OMNIA_AUTH_API_KEYS_DEFAULT_EXPIRATION` | Default expiration in days (0 = never) | `90` |

## OAuth Providers

### Generic OIDC

For any OpenID Connect compliant provider.

```bash
OMNIA_OAUTH_PROVIDER=generic
OMNIA_OAUTH_ISSUER_URL=https://auth.example.com
```

**Required:** `OMNIA_OAUTH_ISSUER_URL`

**Default scopes:** `openid`, `profile`, `email`

**Discovery:** Automatic via `/.well-known/openid-configuration`

### Google

```bash
OMNIA_OAUTH_PROVIDER=google
```

**Issuer:** `https://accounts.google.com`

**Default scopes:** `openid`, `profile`, `email`

**Callback URL:** `https://your-domain/api/auth/callback`

**Console:** [Google Cloud Console](https://console.cloud.google.com/apis/credentials)

### GitHub

```bash
OMNIA_OAUTH_PROVIDER=github
```

**Note:** GitHub uses OAuth 2.0, not OIDC. User info is fetched from GitHub API.

**Default scopes:** `read:user`, `user:email`

**Callback URL:** `https://your-domain/api/auth/callback`

**Console:** [GitHub Developer Settings](https://github.com/settings/developers)

**Limitations:**
- No groups claim (all users get default role)
- No OIDC discovery
- No SSO logout

### Azure AD / Entra ID

```bash
OMNIA_OAUTH_PROVIDER=azure
OMNIA_OAUTH_AZURE_TENANT_ID=your-tenant-id
```

**Issuer:** `https://login.microsoftonline.com/{tenant}/v2.0`

**Default scopes:** `openid`, `profile`, `email`

**Callback URL:** `https://your-domain/api/auth/callback`

**Console:** [Azure Portal](https://portal.azure.com/#view/Microsoft_AAD_IAM/ActiveDirectoryMenuBlade/~/RegisteredApps)

**Group claims:** Configure in Token Configuration > Add groups claim

### Okta

```bash
OMNIA_OAUTH_PROVIDER=okta
OMNIA_OAUTH_OKTA_DOMAIN=your-domain.okta.com
```

**Issuer:** `https://{domain}/oauth2/default`

**Default scopes:** `openid`, `profile`, `email`, `groups`

**Callback URL:** `https://your-domain/api/auth/callback`

**Console:** Okta Admin Console > Applications

## API Endpoints

### Login

```
POST /api/auth/login
GET /api/auth/login?returnTo=/agents
```

Initiates OAuth flow. Query parameter `returnTo` specifies redirect after login.

### Callback

```
GET /api/auth/callback?code=xxx&state=xxx
```

OAuth callback endpoint. Handles authorization code exchange.

### Logout

```
POST /api/auth/logout
```

Clears session. In OAuth mode, may redirect to IdP for SSO logout.

**Response:**
```json
{
  "success": true,
  "redirectUrl": "https://idp.example.com/logout?..."  // Optional
}
```

### Refresh

```
POST /api/auth/refresh
```

Refreshes OAuth access token using refresh token.

**Response:**
```json
{
  "success": true,
  "expiresAt": 1704067200
}
```

### Current User

```
GET /api/auth/me
```

Returns current user information.

**Response:**
```json
{
  "user": {
    "id": "user-123",
    "username": "jdoe",
    "email": "jdoe@example.com",
    "displayName": "John Doe",
    "groups": ["omnia-admins"],
    "role": "admin",
    "provider": "oauth"
  }
}
```

### API Keys

```
GET /api/settings/api-keys
POST /api/settings/api-keys
DELETE /api/settings/api-keys/:id
```

Manage API keys for the current user.

## Session Data Structure

The session cookie contains encrypted JSON:

```typescript
interface SessionData {
  user?: {
    id: string;
    username: string;
    email?: string;
    displayName?: string;
    groups: string[];
    role: "admin" | "editor" | "viewer";
    provider: "anonymous" | "proxy" | "oauth" | "api-key";
  };
  createdAt?: number;
  oauth?: {
    accessToken: string;
    refreshToken?: string;
    idToken?: string;
    expiresAt?: number;
    provider: string;
  };
  pkce?: {
    codeVerifier: string;
    codeChallenge: string;
    state: string;
    returnTo?: string;
  };
}
```

## User Roles

### Permissions Matrix

| Action | Viewer | Editor | Admin |
|--------|--------|--------|-------|
| View agents | Yes | Yes | Yes |
| View logs | Yes | Yes | Yes |
| View metrics | Yes | Yes | Yes |
| Scale agents | No | Yes | Yes |
| Create agents | No | Yes | Yes |
| Delete agents | No | Yes | Yes |
| Modify prompts | No | Yes | Yes |
| Modify tools | No | Yes | Yes |
| Manage own API keys | Yes | Yes | Yes |
| Manage all API keys | No | No | Yes |
| View all users | No | No | Yes |

### Role Resolution Order

1. Check admin groups
2. Check editor groups
3. Default to viewer

First match wins. Example:

```bash
OMNIA_AUTH_ROLE_ADMIN_GROUPS=admins,super-users
OMNIA_AUTH_ROLE_EDITOR_GROUPS=developers,ops
```

User with groups `["developers", "admins"]` gets **admin** role (checked first).

## API Key Format

API keys follow this format:

```
omnia_sk_[base64-encoded-data]
```

The encoded data contains:
- Key ID
- User ID
- Creation timestamp
- Signature

Example:
```
omnia_sk_eyJpZCI6ImtleS0xMjMiLCJ1c2VyIjoidXNlci00NTYiLCJjcmVhdGVkIjoxNzA0MDY3MjAwfQ.signature
```

## HTTP Headers

### Request Headers

| Header | Description |
|--------|-------------|
| `Authorization` | Bearer token: `Bearer omnia_sk_...` or `Bearer <jwt>` |
| `X-API-Key` | Alternative API key header |
| `Cookie` | Session cookie (browser requests) |

### Response Headers (Proxy Mode)

Headers read from reverse proxy:

| Default Header | Contains |
|----------------|----------|
| `X-Forwarded-User` | Username |
| `X-Forwarded-Email` | Email address |
| `X-Forwarded-Groups` | Comma-separated groups |
| `X-Forwarded-Preferred-Username` | Display name |

## Error Codes

### OAuth Errors

| Error | Description |
|-------|-------------|
| `invalid_state` | CSRF state mismatch |
| `no_code` | No authorization code in callback |
| `callback_failed` | Token exchange failed |
| `access_denied` | User denied consent |
| `invalid_claims` | Missing required claims |
| `config_error` | OAuth misconfiguration |

### API Errors

| Status | Error | Description |
|--------|-------|-------------|
| 401 | `unauthorized` | No valid authentication |
| 403 | `forbidden` | Insufficient permissions |
| 400 | `invalid_request` | Malformed request |

## Security Headers

The dashboard sets these security headers:

```
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
X-XSS-Protection: 1; mode=block
Referrer-Policy: strict-origin-when-cross-origin
```

Session cookies include:

```
HttpOnly; Secure; SameSite=Lax; Path=/
```

## Callback URLs

Configure these URLs in your identity provider:

| Provider | Redirect URI |
|----------|--------------|
| All | `https://your-domain/api/auth/callback` |

For logout (if supported):

| Provider | Post-Logout URI |
|----------|-----------------|
| All | `https://your-domain/login` |
