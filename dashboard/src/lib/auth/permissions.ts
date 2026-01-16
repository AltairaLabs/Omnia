/**
 * RBAC permissions and role mappings.
 *
 * Defines fine-grained permissions and maps them to roles.
 * Use these to check specific actions rather than just role levels.
 */

import type { UserRole } from "./config";
import type { User } from "./types";

/**
 * All available permissions in the system.
 */
export const Permission = {
  // Agent permissions
  AGENTS_VIEW: "agents:view",
  AGENTS_SCALE: "agents:scale",
  AGENTS_DEPLOY: "agents:deploy",
  AGENTS_DELETE: "agents:delete",

  // PromptPack permissions
  PROMPTPACKS_VIEW: "promptpacks:view",
  PROMPTPACKS_CREATE: "promptpacks:create",
  PROMPTPACKS_EDIT: "promptpacks:edit",
  PROMPTPACKS_DELETE: "promptpacks:delete",

  // ToolRegistry permissions
  TOOLS_VIEW: "tools:view",
  TOOLS_CREATE: "tools:create",
  TOOLS_EDIT: "tools:edit",
  TOOLS_DELETE: "tools:delete",

  // Logs and metrics
  LOGS_VIEW: "logs:view",
  METRICS_VIEW: "metrics:view",

  // Sessions
  SESSIONS_VIEW: "sessions:view",
  SESSIONS_DELETE: "sessions:delete",

  // Admin permissions
  USERS_VIEW: "users:view",
  USERS_MANAGE: "users:manage",
  SETTINGS_VIEW: "settings:view",
  SETTINGS_EDIT: "settings:edit",

  // API keys (own = user's own keys, all = any user's keys)
  API_KEYS_VIEW_OWN: "apikeys:view:own",
  API_KEYS_MANAGE_OWN: "apikeys:manage:own",
  API_KEYS_VIEW_ALL: "apikeys:view:all",
  API_KEYS_MANAGE_ALL: "apikeys:manage:all",

  // Provider credentials (K8s secrets for API keys)
  CREDENTIALS_VIEW: "credentials:view",
  CREDENTIALS_CREATE: "credentials:create",
  CREDENTIALS_EDIT: "credentials:edit",
  CREDENTIALS_DELETE: "credentials:delete",
} as const;

export type PermissionType = (typeof Permission)[keyof typeof Permission];

/**
 * Permissions granted to each role.
 * Higher roles inherit all permissions from lower roles.
 */
const rolePermissions: Record<UserRole, PermissionType[]> = {
  viewer: [
    // Read-only access
    Permission.AGENTS_VIEW,
    Permission.PROMPTPACKS_VIEW,
    Permission.TOOLS_VIEW,
    Permission.LOGS_VIEW,
    Permission.METRICS_VIEW,
    Permission.SESSIONS_VIEW,
    // Own API keys only
    Permission.API_KEYS_VIEW_OWN,
    Permission.API_KEYS_MANAGE_OWN,
    // View provider credentials (secrets metadata only)
    Permission.CREDENTIALS_VIEW,
  ],

  editor: [
    // Inherits viewer permissions (handled by getPermissionsForRole)
    // Plus write access
    Permission.AGENTS_SCALE,
    Permission.AGENTS_DEPLOY,
    Permission.AGENTS_DELETE,
    Permission.PROMPTPACKS_CREATE,
    Permission.PROMPTPACKS_EDIT,
    Permission.PROMPTPACKS_DELETE,
    Permission.TOOLS_CREATE,
    Permission.TOOLS_EDIT,
    Permission.TOOLS_DELETE,
    Permission.SESSIONS_DELETE,
    // Manage provider credentials
    Permission.CREDENTIALS_CREATE,
    Permission.CREDENTIALS_EDIT,
  ],

  admin: [
    // Inherits editor permissions (handled by getPermissionsForRole)
    // Plus admin access
    Permission.USERS_VIEW,
    Permission.USERS_MANAGE,
    Permission.SETTINGS_VIEW,
    Permission.SETTINGS_EDIT,
    Permission.API_KEYS_VIEW_ALL,
    Permission.API_KEYS_MANAGE_ALL,
    // Delete provider credentials
    Permission.CREDENTIALS_DELETE,
  ],
};

/**
 * Role hierarchy for inheritance.
 */
const roleHierarchy: UserRole[] = ["viewer", "editor", "admin"];

/**
 * Get all permissions for a role, including inherited permissions.
 */
export function getPermissionsForRole(role: UserRole): Set<PermissionType> {
  const permissions = new Set<PermissionType>();
  const roleIndex = roleHierarchy.indexOf(role);

  // Add permissions from this role and all lower roles
  for (let i = 0; i <= roleIndex; i++) {
    const currentRole = roleHierarchy[i];
    for (const permission of rolePermissions[currentRole]) {
      permissions.add(permission);
    }
  }

  return permissions;
}

/**
 * Check if a role has a specific permission.
 */
export function roleHasPermission(
  role: UserRole,
  permission: PermissionType
): boolean {
  const permissions = getPermissionsForRole(role);
  return permissions.has(permission);
}

/**
 * Check if a user has a specific permission.
 */
export function userHasPermission(
  user: User,
  permission: PermissionType
): boolean {
  return roleHasPermission(user.role, permission);
}

/**
 * Check if a user has all of the specified permissions.
 */
export function userHasAllPermissions(
  user: User,
  permissions: PermissionType[]
): boolean {
  return permissions.every((p) => userHasPermission(user, p));
}

/**
 * Check if a user has any of the specified permissions.
 */
export function userHasAnyPermission(
  user: User,
  permissions: PermissionType[]
): boolean {
  return permissions.some((p) => userHasPermission(user, p));
}

/**
 * Permission groups for common use cases.
 */
export const PermissionGroups = {
  /** Can modify agents (scale, deploy, delete) */
  AGENT_WRITE: [
    Permission.AGENTS_SCALE,
    Permission.AGENTS_DEPLOY,
    Permission.AGENTS_DELETE,
  ],

  /** Can modify prompt packs */
  PROMPTPACK_WRITE: [
    Permission.PROMPTPACKS_CREATE,
    Permission.PROMPTPACKS_EDIT,
    Permission.PROMPTPACKS_DELETE,
  ],

  /** Can modify tool registries */
  TOOLS_WRITE: [
    Permission.TOOLS_CREATE,
    Permission.TOOLS_EDIT,
    Permission.TOOLS_DELETE,
  ],

  /** Admin-only operations */
  ADMIN_OPS: [
    Permission.USERS_MANAGE,
    Permission.SETTINGS_EDIT,
  ],

  /** Can modify provider credentials */
  CREDENTIALS_WRITE: [
    Permission.CREDENTIALS_CREATE,
    Permission.CREDENTIALS_EDIT,
    Permission.CREDENTIALS_DELETE,
  ],
} as const;
