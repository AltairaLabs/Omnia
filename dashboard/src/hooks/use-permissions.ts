"use client";

/**
 * Hook for checking permissions in client components.
 *
 * Usage:
 *   const { can, canAll, canAny } = usePermissions();
 *
 *   if (can(Permission.AGENTS_DEPLOY)) {
 *     // show deploy button
 *   }
 */

import { useCallback, useMemo } from "react";
import { useAuth } from "./use-auth";
import {
  Permission,
  type PermissionType,
  roleHasPermission,
  getPermissionsForRole,
} from "@/lib/auth/permissions";

// Re-export Permission for convenience
export { Permission };

interface UsePermissionsReturn {
  /** Check if user has a specific permission */
  can: (permission: PermissionType) => boolean;
  /** Check if user has all specified permissions */
  canAll: (permissions: PermissionType[]) => boolean;
  /** Check if user has any of the specified permissions */
  canAny: (permissions: PermissionType[]) => boolean;
  /** All permissions the user has */
  permissions: Set<PermissionType>;
}

/**
 * Hook to check user permissions.
 */
export function usePermissions(): UsePermissionsReturn {
  const { role } = useAuth();

  const permissions = useMemo(() => getPermissionsForRole(role), [role]);

  const can = useCallback(
    (permission: PermissionType) => roleHasPermission(role, permission),
    [role]
  );

  const canAll = useCallback(
    (perms: PermissionType[]) => perms.every((p) => roleHasPermission(role, p)),
    [role]
  );

  const canAny = useCallback(
    (perms: PermissionType[]) => perms.some((p) => roleHasPermission(role, p)),
    [role]
  );

  return {
    can,
    canAll,
    canAny,
    permissions,
  };
}

/**
 * Shorthand permissions for common checks.
 */
export const Permissions = {
  // Agent operations
  canViewAgents: Permission.AGENTS_VIEW,
  canScaleAgents: Permission.AGENTS_SCALE,
  canDeployAgents: Permission.AGENTS_DEPLOY,
  canDeleteAgents: Permission.AGENTS_DELETE,

  // PromptPack operations
  canViewPromptPacks: Permission.PROMPTPACKS_VIEW,
  canCreatePromptPacks: Permission.PROMPTPACKS_CREATE,
  canEditPromptPacks: Permission.PROMPTPACKS_EDIT,
  canDeletePromptPacks: Permission.PROMPTPACKS_DELETE,

  // Tool operations
  canViewTools: Permission.TOOLS_VIEW,
  canCreateTools: Permission.TOOLS_CREATE,
  canEditTools: Permission.TOOLS_EDIT,
  canDeleteTools: Permission.TOOLS_DELETE,

  // Admin operations
  canManageUsers: Permission.USERS_MANAGE,
  canEditSettings: Permission.SETTINGS_EDIT,
} as const;
