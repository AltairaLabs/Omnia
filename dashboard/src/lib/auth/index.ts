/**
 * Authentication module for Omnia Dashboard.
 *
 * Supports:
 * - Proxy authentication (header-based, for OAuth2 Proxy, Authelia, etc.)
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

import { getAuthConfig } from "./config";
import { createAnonymousUser, type User } from "./types";
import { getCurrentUser, saveUserToSession } from "./session";
import { getUserFromProxyHeaders } from "./proxy";
import { userHasPermission, type PermissionType } from "./permissions";

/**
 * Get the current authenticated user.
 *
 * This is the main entry point for authentication. It:
 * 1. Checks for existing session
 * 2. If proxy mode, validates/refreshes from headers
 * 3. Falls back to anonymous if configured
 *
 * @returns The current user (never null - returns anonymous user if allowed)
 * @throws Error if authentication is required but not present
 */
export async function getUser(): Promise<User> {
  const config = getAuthConfig();

  // Anonymous mode - return anonymous user
  if (config.mode === "anonymous") {
    return createAnonymousUser(config.anonymous.role);
  }

  // Proxy mode - check headers
  if (config.mode === "proxy") {
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

  // Fallback to anonymous
  return createAnonymousUser(config.anonymous.role);
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
