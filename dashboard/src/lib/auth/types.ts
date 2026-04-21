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
 * Session data persisted in the server-side store under `sess:<sid>`.
 * Must not go into the cookie — it exceeds the 4KB browser limit for
 * any non-trivial IDP token. Kept as an alias for now so callers that
 * previously imported SessionData still type-check; later tasks will
 * migrate consumers to SessionRecord from session-store/types.
 */
export interface SessionData {
  user?: User;
  createdAt?: number;
  oauth?: OAuthTokens;
  /**
   * @deprecated PKCE data now lives in the server-side store keyed by state.
   * Retained here so callers in login/callback routes still type-check while
   * Tasks 8 and 9 migrate those files to the new session-store PKCE API.
   */
  pkce?: PKCEData;
}

/**
 * Payload sealed into the session cookie. Kept intentionally tiny
 * (~60 bytes after iron-session sealing) so it is safe across every IDP.
 */
export interface SessionCookieData {
  sid?: string;
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
