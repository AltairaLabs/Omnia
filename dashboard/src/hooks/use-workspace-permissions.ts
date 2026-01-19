"use client";

/**
 * Hook for checking workspace permissions.
 *
 * Provides convenient access to the current workspace's permissions
 * for UI security trimming (hiding/disabling UI elements based on permissions).
 *
 * Usage:
 *   const { canWrite, canDelete, isViewer } = useWorkspacePermissions();
 *
 *   if (canWrite) {
 *     // Show create/edit buttons
 *   }
 */

import { useMemo } from "react";
import { useWorkspace } from "@/contexts/workspace-context";
import type { WorkspaceRole, WorkspacePermissions } from "@/types/workspace";

/**
 * Default permissions when no workspace is selected (no access).
 */
const NO_PERMISSIONS: WorkspacePermissions = {
  read: false,
  write: false,
  delete: false,
  manageMembers: false,
};

/**
 * Return type for useWorkspacePermissions hook.
 */
export interface WorkspacePermissionsResult {
  /** Raw permissions object */
  permissions: WorkspacePermissions;

  /** User's role in the workspace (null if no workspace selected) */
  role: WorkspaceRole | null;

  // Permission checks
  /** Can read workspace resources */
  canRead: boolean;
  /** Can create/update workspace resources */
  canWrite: boolean;
  /** Can delete workspace resources */
  canDelete: boolean;
  /** Can manage workspace membership */
  canManageMembers: boolean;

  // Role checks
  /** User has viewer role */
  isViewer: boolean;
  /** User has editor role (or higher) */
  isEditor: boolean;
  /** User has owner role */
  isOwner: boolean;

  /** Whether a workspace is selected */
  hasWorkspace: boolean;
}

/**
 * Hook to access current workspace permissions.
 *
 * Returns permission flags and role information for the current workspace.
 * If no workspace is selected, all permissions are false.
 *
 * @example
 * ```tsx
 * function CreateButton() {
 *   const { canWrite } = useWorkspacePermissions();
 *
 *   if (!canWrite) return null;
 *
 *   return <Button>Create Agent</Button>;
 * }
 * ```
 *
 * @example
 * ```tsx
 * function ScaleControl() {
 *   const { canWrite } = useWorkspacePermissions();
 *
 *   return (
 *     <Button disabled={!canWrite}>
 *       Scale
 *     </Button>
 *   );
 * }
 * ```
 */
export function useWorkspacePermissions(): WorkspacePermissionsResult {
  const { currentWorkspace } = useWorkspace();

  return useMemo(() => {
    const hasWorkspace = currentWorkspace !== null;
    const permissions = currentWorkspace?.permissions ?? NO_PERMISSIONS;
    const role = currentWorkspace?.role ?? null;

    return {
      permissions,
      role,

      // Permission checks
      canRead: permissions.read,
      canWrite: permissions.write,
      canDelete: permissions.delete,
      canManageMembers: permissions.manageMembers,

      // Role checks
      isViewer: role === "viewer",
      isEditor: role === "editor" || role === "owner",
      isOwner: role === "owner",

      hasWorkspace,
    };
  }, [currentWorkspace]);
}

/**
 * Permission type for RequirePermission component.
 */
export type PermissionType = "read" | "write" | "delete" | "manageMembers";

/**
 * Check if a specific permission is granted.
 *
 * @param permissions - The permissions object
 * @param permission - The permission to check
 * @returns Whether the permission is granted
 */
export function hasPermission(
  permissions: WorkspacePermissions,
  permission: PermissionType
): boolean {
  return permissions[permission];
}
