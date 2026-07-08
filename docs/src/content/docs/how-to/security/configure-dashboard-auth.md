---
title: "Configure dashboard authentication"
description: "Set up user authentication for the Omnia dashboard"
sidebar:
  order: 6
---


This guide covers configuring authentication for the Omnia dashboard. Choose the mode that fits your deployment:

| Mode | Best For | Complexity |
|------|----------|------------|
| [Anonymous](#anonymous-mode) | Development, demos | Minimal |
| [Proxy](#proxy-mode) | Existing OAuth2 Proxy setup | Low |
| [OAuth](#oauth-mode) | Standalone deployments with IdP | Medium |
| [Builtin](#builtin-mode) | Small teams without IdP | Medium |

For agent endpoint authentication (JWT/Istio), see [Configure Agent Authentication](/how-to/security/configure-authentication/).

## Prerequisites

- Omnia dashboard deployed
- Access to environment configuration
- (For OAuth) Identity provider credentials

## Where these settings go

The examples below show the `OMNIA_*` **environment variables** the dashboard container
reads. You don't set these by hand — the **Helm chart renders them** from `dashboard.auth.*`
and `dashboard.oauth.*` values into the dashboard's ConfigMap (non-secret settings) and wires
credentials in from a Kubernetes Secret. So you configure auth at **install/upgrade time** in
your values file:

```bash
helm upgrade --install omnia altaira/omnia -f values.yaml
```

Each env var maps to a value:

| Environment variable | Helm value |
|----------------------|-----------|
| `OMNIA_AUTH_MODE` | `dashboard.auth.mode` |
| `OMNIA_AUTH_ROLE_ADMIN_GROUPS` | `dashboard.auth.roleMapping.adminGroups` (list) |
| `OMNIA_AUTH_ROLE_EDITOR_GROUPS` | `dashboard.auth.roleMapping.editorGroups` (list) |
| `OMNIA_OAUTH_PROVIDER` | `dashboard.oauth.provider` |
| `OMNIA_OAUTH_ISSUER_URL` | `dashboard.oauth.issuerUrl` |
| `OMNIA_OAUTH_SCOPES` | `dashboard.oauth.scopes` |
| `OMNIA_OAUTH_AZURE_TENANT_ID` | `dashboard.oauth.azureTenantId` |
| `OMNIA_OAUTH_CLAIM_USERNAME` / `_EMAIL` / `_GROUPS` | `dashboard.oauth.claims.username` / `.email` / `.groups` |

**Credentials come from a Secret, never values.yaml.** Create a Kubernetes Secret and
reference it — the chart mounts its keys as env:

- `dashboard.oauth.existingSecret` → a Secret with keys `OMNIA_OAUTH_CLIENT_ID` and
  `OMNIA_OAUTH_CLIENT_SECRET`
- `dashboard.auth.existingSessionSecret` → a Secret with key `OMNIA_SESSION_SECRET` (the
  session cookie signing key)

A complete OAuth values block (Cognito) looks like:

```yaml
dashboard:
  auth:
    mode: oauth
    existingSessionSecret: omnia-dashboard-oauth   # holds OMNIA_SESSION_SECRET
    roleMapping:
      adminGroups: [omnia-admins]                  # platform admins
  oauth:
    provider: generic
    existingSecret: omnia-dashboard-oauth          # holds client id + secret
    issuerUrl: https://cognito-idp.<region>.amazonaws.com/<userPoolId>
    scopes: "openid,email,profile"
    claims:
      username: cognito:username
      groups: cognito:groups
```

For Entra ID, swap `oauth.provider: azure` + `oauth.azureTenantId`, and use the default
`groups` claim (object-ID GUIDs). The mode-by-mode sections below show the individual
settings; combine them under `dashboard.auth` / `dashboard.oauth` as above.

:::note
For a non-Helm deployment (running the dashboard container directly), set the `OMNIA_*`
environment variables shown below on the container yourself — the names are identical.
:::

## Anonymous mode

The default mode with no authentication. All users can access the dashboard as viewers.

### Enable anonymous mode

Set the auth mode environment variable:

```bash
OMNIA_AUTH_MODE=anonymous
```

### Configure anonymous role

By default, anonymous users get the `viewer` role. To change this:

```bash
OMNIA_AUTH_ANONYMOUS_ROLE=editor  # or admin
```

:::caution
Anonymous mode provides no security. Use only for development or when network-level security is in place.
:::

## Proxy mode

Delegates authentication to a reverse proxy that handles OAuth/OIDC.

### How it works

1. Reverse proxy (OAuth2 Proxy, Authelia, etc.) handles authentication
2. Proxy forwards user info in HTTP headers
3. Dashboard reads headers and creates session

### Enable proxy mode

```bash
OMNIA_AUTH_MODE=proxy
```

### Configure header names

Match your proxy's header configuration:

```bash
# Default header names (OAuth2 Proxy compatible)
OMNIA_AUTH_PROXY_HEADER_USER=X-Forwarded-User
OMNIA_AUTH_PROXY_HEADER_EMAIL=X-Forwarded-Email
OMNIA_AUTH_PROXY_HEADER_GROUPS=X-Forwarded-Groups
OMNIA_AUTH_PROXY_HEADER_DISPLAY_NAME=X-Forwarded-Preferred-Username
```

### Configure role mapping

Map identity provider groups to dashboard roles:

```bash
# Comma-separated list of groups that get admin role
OMNIA_AUTH_ROLE_ADMIN_GROUPS=omnia-admins,platform-admins

# Comma-separated list of groups that get editor role
OMNIA_AUTH_ROLE_EDITOR_GROUPS=omnia-editors,developers

# Users not in any group get viewer role
```

### OAuth2 proxy example

Example OAuth2 Proxy configuration for Google:

```yaml
# oauth2-proxy values.yaml
config:
  clientID: "YOUR_CLIENT_ID"
  clientSecret: "YOUR_CLIENT_SECRET"
  cookieSecret: "YOUR_COOKIE_SECRET"

extraArgs:
  provider: google
  email-domain: "*"
  pass-user-headers: true
  set-xauthrequest: true

ingress:
  enabled: true
  hosts:
    - dashboard.example.com
```

Nginx ingress annotation:

```yaml
nginx.ingress.kubernetes.io/auth-url: "http://oauth2-proxy.auth.svc.cluster.local/oauth2/auth"
nginx.ingress.kubernetes.io/auth-signin: "https://dashboard.example.com/oauth2/start?rd=$escaped_request_uri"
nginx.ingress.kubernetes.io/auth-response-headers: "X-Auth-Request-User,X-Auth-Request-Email,X-Auth-Request-Groups"
```

### Authelia example

Authelia configuration:

```yaml
# authelia configuration.yml
access_control:
  default_policy: deny
  rules:
    - domain: dashboard.example.com
      policy: two_factor
      subject:
        - "group:omnia-users"
```

Traefik middleware:

```yaml
apiVersion: traefik.containo.us/v1alpha1
kind: Middleware
metadata:
  name: authelia
spec:
  forwardAuth:
    address: http://authelia.auth.svc.cluster.local/api/verify?rd=https://auth.example.com
    authResponseHeaders:
      - Remote-User
      - Remote-Email
      - Remote-Groups
```

Update dashboard headers to match:

```bash
OMNIA_AUTH_PROXY_HEADER_USER=Remote-User
OMNIA_AUTH_PROXY_HEADER_EMAIL=Remote-Email
OMNIA_AUTH_PROXY_HEADER_GROUPS=Remote-Groups
```

## OAuth mode

Direct OAuth 2.0 / OpenID Connect integration with identity providers.

### How it works

1. User clicks "Sign in" on login page
2. Dashboard redirects to identity provider with PKCE
3. User authenticates with IdP
4. IdP redirects back with authorization code
5. Dashboard exchanges code for tokens
6. Dashboard creates session from ID token claims

### Enable OAuth mode

```bash
OMNIA_AUTH_MODE=oauth
```

### Required configuration

All OAuth deployments need:

```bash
# Base URL for callbacks (no trailing slash)
OMNIA_BASE_URL=https://dashboard.example.com

# OAuth client credentials
OMNIA_OAUTH_CLIENT_ID=your-client-id
OMNIA_OAUTH_CLIENT_SECRET=your-client-secret

# Or mount secret from file (K8s Secret)
# OMNIA_OAUTH_CLIENT_SECRET_FILE=/etc/omnia/oauth/client-secret
```

### Provider configuration

#### Generic OIDC

For any OIDC-compliant provider:

```bash
OMNIA_OAUTH_PROVIDER=generic
OMNIA_OAUTH_ISSUER_URL=https://auth.example.com
OMNIA_OAUTH_SCOPES=openid,profile,email,groups
```

#### Google

```bash
OMNIA_OAUTH_PROVIDER=google
OMNIA_OAUTH_CLIENT_ID=xxx.apps.googleusercontent.com
OMNIA_OAUTH_CLIENT_SECRET=GOCSPX-xxx
```

Create credentials at [Google Cloud Console](https://console.cloud.google.com/apis/credentials):

1. Create OAuth 2.0 Client ID
2. Set authorized redirect URI: `https://dashboard.example.com/api/auth/callback`
3. Copy client ID and secret

#### Azure AD / entra ID

```bash
OMNIA_OAUTH_PROVIDER=azure
OMNIA_OAUTH_AZURE_TENANT_ID=your-tenant-id
OMNIA_OAUTH_CLIENT_ID=your-application-id
OMNIA_OAUTH_CLIENT_SECRET=your-client-secret
```

Configure in Azure Portal:

1. Register application in App registrations
2. Add redirect URI: `https://dashboard.example.com/api/auth/callback`
3. Create client secret
4. Configure API permissions: `openid`, `profile`, `email`
5. (Optional) Add `GroupMember.Read.All` for group claims

#### Okta

```bash
OMNIA_OAUTH_PROVIDER=okta
OMNIA_OAUTH_OKTA_DOMAIN=your-domain.okta.com
OMNIA_OAUTH_CLIENT_ID=your-client-id
OMNIA_OAUTH_CLIENT_SECRET=your-client-secret
```

Configure in Okta Admin:

1. Create OIDC Web Application
2. Set sign-in redirect URI: `https://dashboard.example.com/api/auth/callback`
3. Set sign-out redirect URI: `https://dashboard.example.com/login`
4. Assign users/groups to application

#### GitHub

```bash
OMNIA_OAUTH_PROVIDER=github
OMNIA_OAUTH_CLIENT_ID=your-client-id
OMNIA_OAUTH_CLIENT_SECRET=your-client-secret
```

:::note
GitHub uses OAuth 2.0, not OIDC. User info is fetched from the GitHub API. Groups are not supported; all GitHub users get the default role.
:::

#### AWS Cognito

Cognito is OIDC-compliant, so use the `generic` provider pointed at your user pool's issuer URL:

```bash
OMNIA_OAUTH_PROVIDER=generic
OMNIA_OAUTH_ISSUER_URL=https://cognito-idp.<region>.amazonaws.com/<userPoolId>
OMNIA_OAUTH_CLIENT_ID=your-app-client-id
OMNIA_OAUTH_CLIENT_SECRET=your-app-client-secret

# Cognito uses non-standard claim names — map them explicitly:
OMNIA_OAUTH_CLAIM_USERNAME=cognito:username
OMNIA_OAUTH_CLAIM_GROUPS=cognito:groups

# Cognito has no "groups" scope; the generic provider requests one by default,
# which Cognito rejects. Override the scope list:
OMNIA_OAUTH_SCOPES=openid,email,profile
```

Configure the user pool (console or Terraform):

1. Create an app client whose callback URL is `https://dashboard.example.com/api/auth/callback`.
2. Create groups (e.g. `omnia-admins`) and add users: `aws cognito-idp admin-add-user-to-group --user-pool-id <id> --username <user> --group-name omnia-admins`.
3. Cognito emits the group **name** in the `cognito:groups` claim — that's the value you reference in role mappings and workspace `roleBindings`.

:::caution
Two Cognito specifics silently break group-based access: the group claim is `cognito:groups` (not `groups`), and the app client must not request a `groups` scope (Cognito doesn't define one). The settings above handle both.
:::

### Claim mapping

Map OIDC claims to user fields:

```bash
# Which claim contains the username
OMNIA_OAUTH_CLAIM_USERNAME=preferred_username

# Which claim contains the email
OMNIA_OAUTH_CLAIM_EMAIL=email

# Which claim contains the display name
OMNIA_OAUTH_CLAIM_DISPLAY_NAME=name

# Which claim contains groups (supports nested paths)
OMNIA_OAUTH_CLAIM_GROUPS=groups
```

For Entra ID / Azure AD, the default `groups` claim carries group **object IDs (GUIDs)**, not names:

```bash
OMNIA_OAUTH_CLAIM_GROUPS=groups   # values are group object-ID GUIDs
```

Enable the claim in the app registration's **Token configuration → Add groups claim**. See
[Use IdP groups for workspace access](#use-idp-groups-for-workspace-access) for how those
GUIDs map to workspace roles.

### Role mapping

Same as proxy mode:

```bash
OMNIA_AUTH_ROLE_ADMIN_GROUPS=omnia-admins
OMNIA_AUTH_ROLE_EDITOR_GROUPS=omnia-editors
```

## Use IdP groups for workspace access

Authenticating a user is only half the story — you also decide what they can see and do.
Omnia uses IdP groups at **two independent layers**, and getting a user productively into a
workspace usually means setting **both**.

### Two layers

| Layer | Configured by | Grants |
|-------|---------------|--------|
| **Platform role** (dashboard-wide) | `OMNIA_AUTH_ROLE_ADMIN_GROUPS` / `OMNIA_AUTH_ROLE_EDITOR_GROUPS` | `admin` / `editor` / `viewer` across the whole dashboard. |
| **Workspace role** (per-workspace) | the Workspace's [`spec.roleBindings`](/reference/core/workspace/#rolebindings) (and `directGrants`) | `owner` / `editor` / `viewer` on that specific workspace — the role that actually lets a user work with its agents, sessions, and data. |

:::caution[A platform admin still sees an empty workspace]
Being in an `OMNIA_AUTH_ROLE_ADMIN_GROUPS` group is **not** the same as having a role on a
workspace. A platform admin can see every workspace and open **Settings → Access** to manage
its bindings (including self-granting), but until a `roleBinding` or `directGrant` gives them
`owner`/`editor`/`viewer`, the workspace reads as **empty**. Grant workspace access
explicitly — don't rely on the platform-admin role for data access.
:::

### Match the group identifier your IdP actually emits

`roleBindings[].groups` (and the `OMNIA_AUTH_ROLE_*` lists) are compared against the user's
group claim by **exact, case-sensitive string equality**. The value you must list is whatever
your IdP puts in the claim — which differs by provider:

| IdP | Group claim | Value to list in `roleBindings.groups` |
|-----|-------------|-----------------------------------------|
| **AWS Cognito** | `cognito:groups` | the group **name**, e.g. `omnia-admins` |
| **Entra ID / Azure AD** | `groups` | the group **object ID (GUID)**, e.g. `c16e8ed8-544e-489c-bbb6-9b027822ddc7` |
| **Okta / generic OIDC** | `groups` (or your `OMNIA_OAUTH_CLAIM_GROUPS`) | whatever the claim carries (usually the name) |

:::tip[Entra emits GUIDs, not names]
By default Entra ID puts group **object IDs** in the `groups` claim, so your `roleBindings`
must list the GUID — not the friendly group name. (Entra can be configured to emit names via
the app registration's token configuration, but object IDs are the default.)
:::

### End-to-end example

Map a Cognito group `omnia-admins` to both a dashboard admin and `owner` on a workspace:

```yaml
# Dashboard (helm values / env): OMNIA_AUTH_ROLE_ADMIN_GROUPS=omnia-admins
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Workspace
metadata:
  name: customer-support
spec:
  displayName: "Customer Support"
  namespace:
    name: omnia-customer-support
    create: true
  roleBindings:
    - groups: ["omnia-admins"]   # Cognito group NAME (use the object-ID GUID for Entra)
      role: owner
```

For the full workspace access model — `editor`/`viewer`, `directGrants`, and ServiceAccount
bindings — see
[Manage workspaces → Configure access control](/how-to/workspaces/manage-workspaces/#configure-access-control).

## Builtin mode

Self-contained authentication with a local user database. No external identity provider required.

### How it works

1. Users register or are seeded by admin
2. Credentials stored in SQLite (default) or PostgreSQL
3. Passwords hashed with bcrypt (12 rounds)
4. Sessions managed via encrypted cookies

### Enable builtin mode

```bash
OMNIA_AUTH_MODE=builtin
```

### Storage backend

#### SQLite (default)

Zero-configuration; suited to single-instance deployments:

```bash
OMNIA_BUILTIN_STORE_TYPE=sqlite
OMNIA_BUILTIN_SQLITE_PATH=./data/omnia-users.db
```

:::tip
SQLite is the default. No additional configuration needed for basic setups.
:::

#### PostgreSQL

For multi-instance production deployments:

```bash
OMNIA_BUILTIN_STORE_TYPE=postgresql
OMNIA_BUILTIN_POSTGRES_URL=postgresql://user:password@localhost:5432/omnia
```

### Initial admin user

On first run, an admin user is automatically created:

```bash
OMNIA_BUILTIN_ADMIN_USERNAME=admin
OMNIA_BUILTIN_ADMIN_EMAIL=admin@example.com
OMNIA_BUILTIN_ADMIN_PASSWORD=changeme123
```

:::caution
Change the default admin password immediately after first login.
:::

### User registration

Control whether new users can sign up:

```bash
# Allow public signup (default: false)
OMNIA_BUILTIN_ALLOW_SIGNUP=true

# Require email verification (default: false)
OMNIA_BUILTIN_VERIFY_EMAIL=true
```

### Password policy

Configure password requirements:

```bash
# Minimum password length (default: 8)
OMNIA_BUILTIN_MIN_PASSWORD_LENGTH=12
```

### Account security

Protect against brute force attacks:

```bash
# Failed attempts before lockout (default: 5)
OMNIA_BUILTIN_MAX_FAILED_ATTEMPTS=5

# Lockout duration in seconds (default: 900 = 15 minutes)
OMNIA_BUILTIN_LOCKOUT_DURATION=900
```

### Password reset

Configure password reset tokens:

```bash
# Token expiration in seconds (default: 3600 = 1 hour)
OMNIA_BUILTIN_RESET_TOKEN_EXPIRATION=3600
```

:::note
Password reset emails require an email service to be configured. Without it, reset tokens are logged to the console (development only).
:::

### Email verification

For deployments requiring verified emails:

```bash
OMNIA_BUILTIN_VERIFY_EMAIL=true

# Token expiration in seconds (default: 86400 = 24 hours)
OMNIA_BUILTIN_VERIFICATION_TOKEN_EXPIRATION=86400
```

### Kubernetes example

```yaml
# values.yaml
dashboard:
  env:
    OMNIA_AUTH_MODE: builtin
    OMNIA_BUILTIN_STORE_TYPE: postgresql
    OMNIA_BUILTIN_ALLOW_SIGNUP: "false"

  envFrom:
    - secretRef:
        name: dashboard-builtin-secret
```

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: dashboard-builtin-secret
type: Opaque
stringData:
  OMNIA_SESSION_SECRET: "your-32-char-secret-here"
  OMNIA_BUILTIN_POSTGRES_URL: "postgresql://user:pass@postgres:5432/omnia"
  OMNIA_BUILTIN_ADMIN_PASSWORD: "initial-admin-password"
```

## Session configuration

Configure session behavior for all modes:

```bash
# Session encryption secret (required in production!)
# Generate with: openssl rand -base64 32
OMNIA_SESSION_SECRET=your-32-character-secret-here

# Cookie name
OMNIA_SESSION_COOKIE_NAME=omnia_session

# Session lifetime in seconds (default: 24 hours)
OMNIA_SESSION_TTL=86400
```

:::danger
Always set `OMNIA_SESSION_SECRET` in production. Without it, sessions won't persist across restarts and security is compromised.
:::

### Session cookie attributes

The dashboard issues its session cookie with `HttpOnly` + `SameSite=Lax`, and `Secure` when `NODE_ENV=production`. The clearing `Set-Cookie` issued on logout / invalid-session paths carries the same attributes, so a transient MITM observing the clear cannot downgrade or leak the cookie value.

## Security response headers

Every dashboard response carries a defence-in-depth header baseline:

| Header | Default | Override |
|---|---|---|
| `Strict-Transport-Security` | `max-age=63072000; includeSubDomains; preload` | — |
| `Content-Security-Policy` | `default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https:; font-src 'self' data:; connect-src 'self' ws: wss:; media-src 'self' blob: data:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'` | `OMNIA_CSP_POLICY` env var |
| `X-Frame-Options` | `DENY` | — |
| `X-Content-Type-Options` | `nosniff` | — |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | — |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=(), payment=()` | — |

The default CSP includes `'unsafe-inline'` and `'unsafe-eval'` because Next.js uses inline scripts for hydration and runtime-evaluated chunks. Operators with a strict policy can override by setting `OMNIA_CSP_POLICY` in the dashboard environment — the value replaces the default wholesale.

The `x-powered-by: Next.js` header is disabled via `poweredByHeader: false` in `next.config.ts`, so the runtime + framework version are not advertised in responses.

## API keys

Enable programmatic access for scripts and CI/CD:

```bash
# Enable API key authentication
OMNIA_AUTH_API_KEYS_ENABLED=true

# Maximum keys per user (default: 10)
OMNIA_AUTH_API_KEYS_MAX_PER_USER=10

# Default expiration in days (default: 90, 0 = never)
OMNIA_AUTH_API_KEYS_DEFAULT_EXPIRATION=90
```

### Generate API key

Users can generate keys from Settings > API Keys, or use the CLI:

```bash
cd dashboard
node scripts/generate-api-key.mjs \
  --name "CI Pipeline" \
  --user-id "user-123" \
  --role "editor" \
  --expires-in-days 30
```

### Use API key

Include the key in requests:

```bash
# Using Authorization header
curl -H "Authorization: Bearer omnia_sk_abc123..." \
  https://dashboard.example.com/api/agents

# Or using X-API-Key header
curl -H "X-API-Key: omnia_sk_abc123..." \
  https://dashboard.example.com/api/agents
```

## Kubernetes deployment

### Using Helm values

```yaml
# values.yaml
dashboard:
  env:
    OMNIA_AUTH_MODE: oauth
    OMNIA_OAUTH_PROVIDER: google
    OMNIA_BASE_URL: https://dashboard.example.com

  envFrom:
    - secretRef:
        name: dashboard-oauth-secret
```

### OAuth secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: dashboard-oauth-secret
type: Opaque
stringData:
  OMNIA_SESSION_SECRET: "your-32-char-secret-here"
  OMNIA_OAUTH_CLIENT_ID: "your-client-id"
  OMNIA_OAUTH_CLIENT_SECRET: "your-client-secret"
```

### File-mounted secret

For client secret rotation without pod restart:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: oauth-client-secret
type: Opaque
stringData:
  client-secret: "your-client-secret"
---
# In deployment
volumes:
  - name: oauth-secret
    secret:
      secretName: oauth-client-secret
volumeMounts:
  - name: oauth-secret
    mountPath: /etc/omnia/oauth
    readOnly: true
env:
  - name: OMNIA_OAUTH_CLIENT_SECRET_FILE
    value: /etc/omnia/oauth/client-secret
```

## Troubleshooting

### OAuth login fails

**Symptom:** Redirect to IdP works, but callback fails

**Check:**
1. Callback URL matches exactly: `https://dashboard.example.com/api/auth/callback`
2. `OMNIA_BASE_URL` has no trailing slash
3. Client secret is correct
4. Required scopes are configured in IdP

### Session not persisting

**Symptom:** User logged out after page refresh

**Check:**
1. `OMNIA_SESSION_SECRET` is set
2. Cookie is being set (check browser dev tools)
3. No proxy stripping cookies

### Groups not working

**Symptom:** User authenticated but has wrong role

**Check:**
1. Groups claim is present in ID token (use jwt.io to decode)
2. `OMNIA_OAUTH_CLAIM_GROUPS` matches your IdP's claim name
3. Group names match `OMNIA_AUTH_ROLE_*_GROUPS` exactly

### Proxy headers not received

**Symptom:** Proxy mode shows anonymous user

**Check:**
1. Proxy is configured to forward headers
2. Header names match configuration
3. No intermediate proxy stripping headers
4. Test with: `curl -v` to see headers

### Builtin login fails

**Symptom:** Correct credentials rejected

**Check:**
1. User exists in database
2. Account not locked (check failed login count)
3. Email verified (if `OMNIA_BUILTIN_VERIFY_EMAIL=true`)
4. Database file/connection accessible

### Account locked out

**Symptom:** Login fails with "Account locked"

**Fix:**
1. Wait for lockout duration (default: 15 minutes)
2. Or manually reset in database:
   ```sql
   UPDATE users SET failed_login_attempts = 0, locked_until = NULL WHERE email = 'user@example.com';
   ```

### Password reset not working

**Symptom:** No reset email received

**Check:**
1. Email service configured (or check console logs for token)
2. Token not expired (`OMNIA_BUILTIN_RESET_TOKEN_EXPIRATION`)
3. User exists with that email

## Next steps

- [Authentication Architecture](/explanation/security/authentication/) - Understand the full auth model
- [Configure Agent Authentication](/how-to/security/configure-authentication/) - Secure agent endpoints
- [Dashboard Auth Reference](/reference/platform/dashboard-auth/) - Complete configuration reference
- [Manage Workspaces](/how-to/workspaces/manage-workspaces/) - Set up team isolation and access control
