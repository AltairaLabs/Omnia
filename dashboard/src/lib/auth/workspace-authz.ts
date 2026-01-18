/**
 * Workspace authorization logic.
 *
 * Provides functions to check user access to workspaces based on:
 * - Group membership (roleBindings)
 * - Individual user grants (directGrants)
 *
 * Implements role hierarchy: owner > editor > viewer
 */

import { getUser } from "./index";
import { getCachedAccess, setCachedAccess } from "./authz-cache";
import { getWorkspace, listWorkspaces } from "@/lib/k8s/workspace-client";
import {
  ROLE_HIERARCHY,
  ROLE_PERMISSIONS,
  NO_PERMISSIONS,
  type Workspace,
  type WorkspaceRole,
  type WorkspaceAccess,
  type RoleBinding,
  type DirectGrant,
} from "@/types/workspace";

/** Access result for denied requests */
const DENIED_ACCESS: WorkspaceAccess = {
  granted: false,
  role: null,
  permissions: NO_PERMISSIONS,
};

/**
 * Compare two roles and return the higher-privilege one.
 * Returns null if both are null.
 */
function maxRole(
  role1: WorkspaceRole | null,
  role2: WorkspaceRole | null
): WorkspaceRole | null {
  if (!role1) return role2;
  if (!role2) return role1;
  return ROLE_HIERARCHY[role1] >= ROLE_HIERARCHY[role2]
    ? role1
    : role2;
}

/**
 * Check if a role meets the required minimum role.
 */
function meetsRoleRequirement(
  grantedRole: WorkspaceRole,
  requiredRole: WorkspaceRole
): boolean {
  return ROLE_HIERARCHY[grantedRole] >= ROLE_HIERARCHY[requiredRole];
}

/**
 * Find the highest role granted to a user through group membership.
 *
 * @param roleBindings - The workspace's role bindings
 * @param userGroups - The user's OIDC groups
 * @returns The highest role found, or null if no match
 */
function findRoleFromBindings(
  roleBindings: RoleBinding[],
  userGroups: string[]
): WorkspaceRole | null {
  let highestRole: WorkspaceRole | null = null;

  for (const binding of roleBindings) {
    if (!binding.groups) continue;

    // Check if user is in any of the bound groups
    const hasMatchingGroup = binding.groups.some((group) =>
      userGroups.includes(group)
    );

    if (hasMatchingGroup) {
      highestRole = maxRole(highestRole, binding.role);
    }
  }

  return highestRole;
}

/**
 * Find a direct grant for a specific user by email.
 * Checks expiration and ignores expired grants.
 *
 * @param directGrants - The workspace's direct grants
 * @param userEmail - The user's email address
 * @returns The granted role, or null if no valid grant found
 */
function findDirectGrant(
  directGrants: DirectGrant[] | undefined,
  userEmail: string
): WorkspaceRole | null {
  if (!directGrants || !userEmail) return null;

  for (const grant of directGrants) {
    if (grant.user.toLowerCase() !== userEmail.toLowerCase()) continue;

    // Check expiration
    if (grant.expires) {
      const expiresAt = new Date(grant.expires).getTime();
      if (Date.now() > expiresAt) {
        // Grant has expired
        continue;
      }
    }

    return grant.role;
  }

  return null;
}

/**
 * Build a WorkspaceAccess result from the determined role.
 *
 * @param grantedRole - The role granted to the user
 * @param requiredRole - Optional minimum required role
 * @returns WorkspaceAccess with granted status and permissions
 */
function buildAccess(
  grantedRole: WorkspaceRole | null,
  requiredRole?: WorkspaceRole
): WorkspaceAccess {
  if (!grantedRole) {
    return DENIED_ACCESS;
  }

  // If a required role is specified, check if granted role meets it
  if (requiredRole && !meetsRoleRequirement(grantedRole, requiredRole)) {
    return {
      granted: false,
      role: grantedRole,
      permissions: ROLE_PERMISSIONS[grantedRole],
    };
  }

  return {
    granted: true,
    role: grantedRole,
    permissions: ROLE_PERMISSIONS[grantedRole],
  };
}

/**
 * Check if the current user has access to a workspace.
 *
 * Authorization flow:
 * 1. Get current user from session/proxy headers
 * 2. Check cache for existing authorization decision
 * 3. Fetch workspace from K8s API
 * 4. Check roleBindings for group membership
 * 5. Check directGrants for individual user access
 * 6. Use highest-privilege role found
 * 7. Cache and return result
 *
 * @param workspaceName - The workspace to check access for
 * @param requiredRole - Optional minimum role required (access denied if user has lower role)
 * @returns WorkspaceAccess with authorization decision and permissions
 */
export async function checkWorkspaceAccess(
  workspaceName: string,
  requiredRole?: WorkspaceRole
): Promise<WorkspaceAccess> {
  // Get current user
  const user = await getUser();

  // Anonymous users cannot access workspaces
  if (user.provider === "anonymous") {
    return DENIED_ACCESS;
  }

  // Email is required for workspace authorization
  if (!user.email) {
    console.warn(
      `User ${user.username} has no email - cannot authorize workspace access`
    );
    return DENIED_ACCESS;
  }

  // Check cache first (before K8s API call)
  const cached = getCachedAccess(user.email, workspaceName);
  if (cached) {
    // If cached result exists, apply required role check
    if (requiredRole && cached.role && !meetsRoleRequirement(cached.role, requiredRole)) {
      return {
        granted: false,
        role: cached.role,
        permissions: cached.permissions,
      };
    }
    return cached;
  }

  // Fetch workspace from K8s
  const workspace = await getWorkspace(workspaceName);
  if (!workspace) {
    // Workspace doesn't exist - deny access
    return DENIED_ACCESS;
  }

  // Check roleBindings for group-based access
  let role = findRoleFromBindings(
    workspace.spec.roleBindings || [],
    user.groups
  );

  // Check directGrants for individual user access
  const directRole = findDirectGrant(workspace.spec.directGrants, user.email);
  role = maxRole(role, directRole);

  // Build access result
  const access = buildAccess(role, requiredRole);

  // Cache the base access (without required role applied)
  const baseAccess = buildAccess(role);
  setCachedAccess(user.email, workspaceName, baseAccess);

  return access;
}

/**
 * Get all workspaces the current user has access to.
 *
 * @param minimumRole - Optional minimum role to filter by
 * @returns Array of workspaces with access information
 */
export async function getAccessibleWorkspaces(
  minimumRole?: WorkspaceRole
): Promise<Array<{ workspace: Workspace; access: WorkspaceAccess }>> {
  const user = await getUser();

  // Anonymous users cannot access workspaces
  if (user.provider === "anonymous" || !user.email) {
    return [];
  }

  // Fetch all workspaces
  const workspaces = await listWorkspaces();
  const accessible: Array<{ workspace: Workspace; access: WorkspaceAccess }> = [];

  for (const workspace of workspaces) {
    // Check roleBindings
    let role = findRoleFromBindings(
      workspace.spec.roleBindings || [],
      user.groups
    );

    // Check directGrants
    const directRole = findDirectGrant(workspace.spec.directGrants, user.email);
    role = maxRole(role, directRole);

    if (!role) continue;

    // Apply minimum role filter
    if (minimumRole && !meetsRoleRequirement(role, minimumRole)) {
      continue;
    }

    const access = buildAccess(role);

    // Cache each workspace access
    setCachedAccess(user.email, workspace.metadata.name, access);

    accessible.push({ workspace, access });
  }

  return accessible;
}

/**
 * Check if user has at least the specified role in a workspace.
 * Convenience wrapper around checkWorkspaceAccess.
 *
 * @param workspaceName - The workspace to check
 * @param requiredRole - The minimum required role
 * @returns true if user has sufficient access
 */
export async function hasWorkspaceRole(
  workspaceName: string,
  requiredRole: WorkspaceRole
): Promise<boolean> {
  const access = await checkWorkspaceAccess(workspaceName, requiredRole);
  return access.granted;
}

/**
 * Require access to a workspace - throws if denied.
 *
 * @param workspaceName - The workspace to check
 * @param requiredRole - Optional minimum required role
 * @throws Error if access is denied
 */
export async function requireWorkspaceAccess(
  workspaceName: string,
  requiredRole?: WorkspaceRole
): Promise<WorkspaceAccess> {
  const access = await checkWorkspaceAccess(workspaceName, requiredRole);

  if (!access.granted) {
    if (access.role && requiredRole) {
      throw new Error(
        `Insufficient workspace permissions: requires ${requiredRole}, have ${access.role}`
      );
    }
    throw new Error(`Access denied to workspace: ${workspaceName}`);
  }

  return access;
}
