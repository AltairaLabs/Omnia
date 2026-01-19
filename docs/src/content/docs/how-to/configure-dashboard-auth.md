---
title: "Configure Dashboard Authentication"
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

For agent endpoint authentication (JWT/Istio), see [Configure Agent Authentication](/how-to/configure-authentication/).

## Prerequisites

- Omnia dashboard deployed
- Access to environment configuration
- (For OAuth) Identity provider credentials

## Anonymous Mode

The default mode with no authentication. All users can access the dashboard as viewers.

### Enable Anonymous Mode

Set the auth mode environment variable:

```bash
OMNIA_AUTH_MODE=anonymous
```

### Configure Anonymous Role

By default, anonymous users get the `viewer` role. To change this:

```bash
OMNIA_AUTH_ANONYMOUS_ROLE=editor  # or admin
```

:::caution
Anonymous mode provides no security. Use only for development or when network-level security is in place.
:::

## Proxy Mode

Delegates authentication to a reverse proxy that handles OAuth/OIDC.

### How It Works

1. Reverse proxy (OAuth2 Proxy, Authelia, etc.) handles authentication
2. Proxy forwards user info in HTTP headers
3. Dashboard reads headers and creates session

### Enable Proxy Mode

```bash
OMNIA_AUTH_MODE=proxy
```

### Configure Header Names

Match your proxy's header configuration:

```bash
# Default header names (OAuth2 Proxy compatible)
OMNIA_AUTH_PROXY_HEADER_USER=X-Forwarded-User
OMNIA_AUTH_PROXY_HEADER_EMAIL=X-Forwarded-Email
OMNIA_AUTH_PROXY_HEADER_GROUPS=X-Forwarded-Groups
OMNIA_AUTH_PROXY_HEADER_DISPLAY_NAME=X-Forwarded-Preferred-Username
```

### Configure Role Mapping

Map identity provider groups to dashboard roles:

```bash
# Comma-separated list of groups that get admin role
OMNIA_AUTH_ROLE_ADMIN_GROUPS=omnia-admins,platform-admins

# Comma-separated list of groups that get editor role
OMNIA_AUTH_ROLE_EDITOR_GROUPS=omnia-editors,developers

# Users not in any group get viewer role
```

### OAuth2 Proxy Example

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

### Authelia Example

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

## OAuth Mode

Direct OAuth 2.0 / OpenID Connect integration with identity providers.

### How It Works

1. User clicks "Sign in" on login page
2. Dashboard redirects to identity provider with PKCE
3. User authenticates with IdP
4. IdP redirects back with authorization code
5. Dashboard exchanges code for tokens
6. Dashboard creates session from ID token claims

### Enable OAuth Mode

```bash
OMNIA_AUTH_MODE=oauth
```

### Required Configuration

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

### Provider Configuration

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

#### Azure AD / Entra ID

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

### Claim Mapping

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

For Azure AD with nested claims:

```bash
OMNIA_OAUTH_CLAIM_GROUPS=wids  # Uses role IDs
# Or configure group claims in Azure AD token configuration
```

### Role Mapping

Same as proxy mode:

```bash
OMNIA_AUTH_ROLE_ADMIN_GROUPS=omnia-admins
OMNIA_AUTH_ROLE_EDITOR_GROUPS=omnia-editors
```

## Builtin Mode

Self-contained authentication with a local user database. No external identity provider required.

### How It Works

1. Users register or are seeded by admin
2. Credentials stored in SQLite (default) or PostgreSQL
3. Passwords hashed with bcrypt (12 rounds)
4. Sessions managed via encrypted cookies

### Enable Builtin Mode

```bash
OMNIA_AUTH_MODE=builtin
```

### Storage Backend

#### SQLite (Default)

Zero-configuration, perfect for single-instance deployments:

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

### Initial Admin User

On first run, an admin user is automatically created:

```bash
OMNIA_BUILTIN_ADMIN_USERNAME=admin
OMNIA_BUILTIN_ADMIN_EMAIL=admin@example.com
OMNIA_BUILTIN_ADMIN_PASSWORD=changeme123
```

:::caution
Change the default admin password immediately after first login!
:::

### User Registration

Control whether new users can sign up:

```bash
# Allow public signup (default: false)
OMNIA_BUILTIN_ALLOW_SIGNUP=true

# Require email verification (default: false)
OMNIA_BUILTIN_VERIFY_EMAIL=true
```

### Password Policy

Configure password requirements:

```bash
# Minimum password length (default: 8)
OMNIA_BUILTIN_MIN_PASSWORD_LENGTH=12
```

### Account Security

Protect against brute force attacks:

```bash
# Failed attempts before lockout (default: 5)
OMNIA_BUILTIN_MAX_FAILED_ATTEMPTS=5

# Lockout duration in seconds (default: 900 = 15 minutes)
OMNIA_BUILTIN_LOCKOUT_DURATION=900
```

### Password Reset

Configure password reset tokens:

```bash
# Token expiration in seconds (default: 3600 = 1 hour)
OMNIA_BUILTIN_RESET_TOKEN_EXPIRATION=3600
```

:::note
Password reset emails require an email service to be configured. Without it, reset tokens are logged to the console (development only).
:::

### Email Verification

For deployments requiring verified emails:

```bash
OMNIA_BUILTIN_VERIFY_EMAIL=true

# Token expiration in seconds (default: 86400 = 24 hours)
OMNIA_BUILTIN_VERIFICATION_TOKEN_EXPIRATION=86400
```

### Kubernetes Example

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

## Session Configuration

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

## API Keys

Enable programmatic access for scripts and CI/CD:

```bash
# Enable API key authentication
OMNIA_AUTH_API_KEYS_ENABLED=true

# Maximum keys per user (default: 10)
OMNIA_AUTH_API_KEYS_MAX_PER_USER=10

# Default expiration in days (default: 90, 0 = never)
OMNIA_AUTH_API_KEYS_DEFAULT_EXPIRATION=90
```

### Generate API Key

Users can generate keys from Settings > API Keys, or use the CLI:

```bash
cd dashboard
node scripts/generate-api-key.mjs \
  --name "CI Pipeline" \
  --user-id "user-123" \
  --role "editor" \
  --expires-in-days 30
```

### Use API Key

Include the key in requests:

```bash
# Using Authorization header
curl -H "Authorization: Bearer omnia_sk_abc123..." \
  https://dashboard.example.com/api/agents

# Or using X-API-Key header
curl -H "X-API-Key: omnia_sk_abc123..." \
  https://dashboard.example.com/api/agents
```

## Kubernetes Deployment

### Using Helm Values

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

### OAuth Secret

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

### File-Mounted Secret

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

### OAuth Login Fails

**Symptom:** Redirect to IdP works, but callback fails

**Check:**
1. Callback URL matches exactly: `https://dashboard.example.com/api/auth/callback`
2. `OMNIA_BASE_URL` has no trailing slash
3. Client secret is correct
4. Required scopes are configured in IdP

### Session Not Persisting

**Symptom:** User logged out after page refresh

**Check:**
1. `OMNIA_SESSION_SECRET` is set
2. Cookie is being set (check browser dev tools)
3. No proxy stripping cookies

### Groups Not Working

**Symptom:** User authenticated but has wrong role

**Check:**
1. Groups claim is present in ID token (use jwt.io to decode)
2. `OMNIA_OAUTH_CLAIM_GROUPS` matches your IdP's claim name
3. Group names match `OMNIA_AUTH_ROLE_*_GROUPS` exactly

### Proxy Headers Not Received

**Symptom:** Proxy mode shows anonymous user

**Check:**
1. Proxy is configured to forward headers
2. Header names match configuration
3. No intermediate proxy stripping headers
4. Test with: `curl -v` to see headers

### Builtin Login Fails

**Symptom:** Correct credentials rejected

**Check:**
1. User exists in database
2. Account not locked (check failed login count)
3. Email verified (if `OMNIA_BUILTIN_VERIFY_EMAIL=true`)
4. Database file/connection accessible

### Account Locked Out

**Symptom:** Login fails with "Account locked"

**Fix:**
1. Wait for lockout duration (default: 15 minutes)
2. Or manually reset in database:
   ```sql
   UPDATE users SET failed_login_attempts = 0, locked_until = NULL WHERE email = 'user@example.com';
   ```

### Password Reset Not Working

**Symptom:** No reset email received

**Check:**
1. Email service configured (or check console logs for token)
2. Token not expired (`OMNIA_BUILTIN_RESET_TOKEN_EXPIRATION`)
3. User exists with that email

## Next Steps

- [Authentication Architecture](/explanation/authentication/) - Understand the full auth model
- [Configure Agent Authentication](/how-to/configure-authentication/) - Secure agent endpoints
- [Dashboard Auth Reference](/reference/dashboard-auth/) - Complete configuration reference
- [Manage Workspaces](/how-to/manage-workspaces/) - Set up team isolation and access control
