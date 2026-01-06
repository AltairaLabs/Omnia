/**
 * User and session types for authentication.
 */

import type { UserRole } from "./config";

// Re-export for convenience
export type { UserRole };

/**
 * Authenticated user information.
 */
export interface User {
  /** Unique identifier (username from proxy header) */
  id: string;
  /** Username */
  username: string;
  /** Email address (optional) */
  email?: string;
  /** Display name (optional) */
  displayName?: string;
  /** User's groups from proxy */
  groups: string[];
  /** Resolved role based on group mapping */
  role: UserRole;
  /** Authentication provider */
  provider: "proxy" | "anonymous";
}

/**
 * Session data stored in the cookie.
 */
export interface SessionData {
  /** User information */
  user?: User;
  /** Session creation timestamp */
  createdAt?: number;
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
