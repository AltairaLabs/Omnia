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
import { getCurrentUser, saveUserToSession, getSession } from "./session";
import { getUserFromProxyHeaders } from "./proxy";
import { userHasPermission, type PermissionType } from "./permissions";
import { authenticateApiKey, isApiKeyAuthEnabled } from "./api-keys";
import { refreshAccessToken, extractClaims, mapClaimsToUser, validateClaims } from "./oauth";

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

  // No authenticated user - return anonymous
  // Middleware will handle redirect to login page
  return createAnonymousUser("viewer");
}

/**
 * Handle OAuth mode authentication.
 */
async function handleOAuthAuth(config: AuthConfig): Promise<User> {
  const sessionUser = await getCurrentUser();

  if (sessionUser?.provider === "oauth") {
    // Check if token needs refresh
    const session = await getSession();
    if (session.oauth && shouldRefreshToken(session.oauth.expiresAt)) {
      await tryRefreshToken(session, config);
    }
    return sessionUser;
  }

  // No authenticated user - return anonymous
  // Middleware will handle redirect to login page
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

  // Check for API key authentication first (works in any mode)
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
  session: Awaited<ReturnType<typeof getSession>>,
  config: ReturnType<typeof getAuthConfig>
): Promise<void> {
  if (!session.oauth?.refreshToken) return;

  try {
    const tokens = await refreshAccessToken(session.oauth.refreshToken);

    // Update tokens in session
    session.oauth = {
      ...session.oauth,
      accessToken: tokens.access_token,
      refreshToken: tokens.refresh_token || session.oauth.refreshToken,
      idToken: tokens.id_token || session.oauth.idToken,
      expiresAt: typeof tokens.expires_at === "number" ? tokens.expires_at : session.oauth.expiresAt,
    };

    // Update user from new claims if available
    if (tokens.id_token) {
      const claims = extractClaims(tokens);
      if (validateClaims(claims)) {
        session.user = mapClaimsToUser(claims, config);
      }
    }

    await session.save();
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
