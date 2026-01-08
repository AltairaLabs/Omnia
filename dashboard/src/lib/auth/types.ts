/**
 * User and session types for authentication.
 */

import type { UserRole } from "./config";
import type { OAuthTokens, PKCEData } from "./oauth/types";

// Re-export for convenience
export type { UserRole } from "./config";

/**
 * Authentication provider types.
 */
export type AuthProvider = "proxy" | "anonymous" | "oauth" | "builtin";

/**
 * Authenticated user information.
 */
export interface User {
  /** Unique identifier (sub claim or username) */
  id: string;
  /** Username */
  username: string;
  /** Email address (optional) */
  email?: string;
  /** Display name (optional) */
  displayName?: string;
  /** User's groups from identity provider */
  groups: string[];
  /** Resolved role based on group mapping */
  role: UserRole;
  /** Authentication provider */
  provider: AuthProvider;
}

/**
 * Session data stored in the cookie.
 */
export interface SessionData {
  /** User information */
  user?: User;
  /** Session creation timestamp */
  createdAt?: number;
  /** OAuth tokens (when using OAuth mode) */
  oauth?: OAuthTokens;
  /** PKCE data during OAuth flow */
  pkce?: PKCEData;
}

/**
 * Anonymous user for unauthenticated access.
 */
export function createAnonymousUser(role: UserRole): User {
  return {
    id: "anonymous",
    username: "anonymous",
    groups: [],
    role,
    provider: "anonymous",
  };
}

/**
 * Check if user has at least the required role.
 */
export function hasRole(user: User, requiredRole: UserRole): boolean {
  const roleHierarchy: Record<UserRole, number> = {
    admin: 3,
    editor: 2,
    viewer: 1,
  };
  return roleHierarchy[user.role] >= roleHierarchy[requiredRole];
}

/**
 * Check if user can perform write operations.
 */
export function canWrite(user: User): boolean {
  return hasRole(user, "editor");
}

/**
 * Check if user can perform admin operations.
 */
export function canAdmin(user: User): boolean {
  return hasRole(user, "admin");
}
