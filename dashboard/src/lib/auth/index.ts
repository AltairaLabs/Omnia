/**
 * Authentication module for Omnia Dashboard.
 *
 * Supports:
 * - OAuth/OIDC authentication (direct integration with identity providers)
 * - Proxy authentication (header-based, for OAuth2 Proxy, Authelia, etc.)
 * - API key authentication (for programmatic access)
 * - Anonymous access (for development or public dashboards)
 *
 * Usage:
 *   import { getUser, canWrite } from "@/lib/auth";
 *
 *   const user = await getUser();
 *   if (canWrite(user)) {
 *     // Allow write operation
 *   }
 */

export * from "./config";
export * from "./types";
export * from "./session";
export * from "./proxy";
export * from "./permissions";
export * from "./api-guard";

import { getAuthConfig, type AuthConfig } from "./config";
import { createAnonymousUser, type User } from "./types";
import { getCurrentUser, saveUserToSession, getSessionRecord, updateSessionOAuth } from "./session";
import { getUserFromProxyHeaders } from "./proxy";
import { userHasPermission, type PermissionType } from "./permissions";

/**
 * Handle proxy mode authentication.
 */
async function handleProxyAuth(): Promise<User> {
  // Try to get user from proxy headers
  const proxyUser = await getUserFromProxyHeaders();

  if (proxyUser) {
    // Update session with latest user info
    await saveUserToSession(proxyUser);
    return proxyUser;
  }

  // Check existing session (for cases where headers aren't forwarded on every request)
  const sessionUser = await getCurrentUser();
  if (sessionUser) {
    return sessionUser;
  }

  // No authentication - return anonymous with viewer role
  // This allows the proxy to handle the redirect to login
  return createAnonymousUser("viewer");
}

/**
 * Handle builtin mode authentication.
 */
async function handleBuiltinAuth(): Promise<User> {
  const sessionUser = await getCurrentUser();

  if (sessionUser?.provider === "builtin") {
    return sessionUser;
  }

  // No authenticated user — return anonymous. The real gate lives in
  // dashboard/src/middleware.ts, which redirects unauthenticated
  // requests to /login before they reach any page or data-fetching
  // route. This branch still exists for server-side code paths that
  // run outside the middleware (tests, cron jobs, direct lib imports)
  // and expect a User object.
  return createAnonymousUser("viewer");
}

/**
 * Handle OAuth mode authentication.
 */
async function handleOAuthAuth(config: AuthConfig): Promise<User> {
  const sessionUser = await getCurrentUser();

  if (sessionUser?.provider === "oauth") {
    // Check if token needs refresh
    const record = await getSessionRecord();
    if (record?.oauth && shouldRefreshToken(record.oauth.expiresAt)) {
      await tryRefreshToken(record.oauth, config);
    }
    return sessionUser;
  }

  // No authenticated user — return anonymous. The real gate lives in
  // dashboard/src/middleware.ts, which redirects unauthenticated
  // requests to /login before they reach any page or data-fetching
  // route. This branch still exists for server-side code paths that
  // run outside the middleware (tests, cron jobs, direct lib imports)
  // and expect a User object.
  return createAnonymousUser("viewer");
}

/**
 * Get the current authenticated user.
 *
 * This is the main entry point for authentication. It checks in order:
 * 1. API key in headers (if enabled)
 * 2. Proxy headers (if proxy mode)
 * 3. Existing session
 * 4. Falls back to anonymous if configured
 *
 * @returns The current user (never null - returns anonymous user if allowed)
 * @throws Error if authentication is required but not present
 */
export async function getUser(): Promise<User> {
  const config = getAuthConfig();

  // Lazy-load API key module only when enabled
  const { isApiKeyAuthEnabled, authenticateApiKey } = await import("./api-keys");
  if (isApiKeyAuthEnabled()) {
    const apiKeyUser = await authenticateApiKey();
    if (apiKeyUser) {
      return apiKeyUser;
    }
  }

  // Handle authentication based on mode
  switch (config.mode) {
    case "anonymous":
      return createAnonymousUser(config.anonymous.role);
    case "proxy":
      return handleProxyAuth();
    case "oauth":
      return handleOAuthAuth(config);
    case "builtin":
      return handleBuiltinAuth();
    default:
      return createAnonymousUser(config.anonymous.role);
  }
}

/**
 * Check if token should be refreshed.
 * Returns true if token expires in less than 5 minutes.
 */
function shouldRefreshToken(expiresAt?: number): boolean {
  if (!expiresAt) return false;
  const now = Math.floor(Date.now() / 1000);
  return expiresAt - now < 300; // 5 minutes
}

/**
 * Try to refresh the access token.
 * Updates session on success, ignores errors (will re-auth on API call failure).
 */
async function tryRefreshToken(
  currentOAuth: import("./oauth/types").OAuthTokens,
  config: ReturnType<typeof getAuthConfig>
): Promise<void> {
  if (!currentOAuth.refreshToken) return;

  try {
    // Lazy-load OAuth module only when token refresh is needed
    const { refreshAccessToken, extractClaims, mapClaimsToUserAsync, validateClaims } = await import("./oauth");
    const tokens = await refreshAccessToken(currentOAuth.refreshToken);

    // Build updated OAuth tokens. Access token is intentionally not
    // persisted — see OAuthTokens jsdoc (cookie size limit).
    const updatedOAuth: import("./oauth/types").OAuthTokens = {
      ...currentOAuth,
      refreshToken: tokens.refresh_token || currentOAuth.refreshToken,
      idToken: tokens.id_token || currentOAuth.idToken,
      expiresAt: typeof tokens.expires_at === "number" ? tokens.expires_at : currentOAuth.expiresAt,
    };

    // Update user from new claims if available. Use the async path
    // to pick up Entra groups-overage on refresh (issue #855).
    let updatedUser: User | undefined;
    if (tokens.id_token) {
      const claims = extractClaims(tokens);
      if (validateClaims(claims)) {
        updatedUser = await mapClaimsToUserAsync(claims, config, tokens.access_token);
      }
    }

    await updateSessionOAuth(updatedOAuth, updatedUser);
  } catch (error) {
    // Log but don't throw - let the API call fail and trigger re-auth
    console.warn("Token refresh failed:", error);
  }
}

/**
 * Check if the current request is authenticated.
 */
export async function isUserAuthenticated(): Promise<boolean> {
  const user = await getUser();
  return user.provider !== "anonymous";
}

/**
 * Require authentication - throws if not authenticated.
 */
export async function requireAuth(): Promise<User> {
  const user = await getUser();
  if (user.provider === "anonymous") {
    throw new Error("Authentication required");
  }
  return user;
}

/**
 * Require specific role - throws if insufficient permissions.
 */
export async function requireRole(role: "admin" | "editor" | "viewer"): Promise<User> {
  const user = await requireAuth();
  const roleHierarchy = { admin: 3, editor: 2, viewer: 1 };
  if (roleHierarchy[user.role] < roleHierarchy[role]) {
    throw new Error(`Insufficient permissions: requires ${role}`);
  }
  return user;
}

/**
 * Require specific permission - throws if user lacks permission.
 */
export async function requirePermission(permission: PermissionType): Promise<User> {
  const user = await getUser();
  if (!userHasPermission(user, permission)) {
    throw new Error(`Insufficient permissions: requires ${permission}`);
  }
  return user;
}

/**
 * Require all specified permissions - throws if user lacks any.
 */
export async function requireAllPermissions(permissions: PermissionType[]): Promise<User> {
  const user = await getUser();
  for (const permission of permissions) {
    if (!userHasPermission(user, permission)) {
      throw new Error(`Insufficient permissions: requires ${permission}`);
    }
  }
  return user;
}
