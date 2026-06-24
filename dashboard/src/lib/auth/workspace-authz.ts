/**
 * Workspace authorization logic.
 *
 * Provides functions to check user access to workspaces based on:
 * - Group membership (roleBindings)
 * - Individual user grants (directGrants)
 * - Anonymous access configuration (anonymousAccess)
 *
 * Implements role hierarchy: owner > editor > viewer
 */

import { getUser } from "./index";
import { canAdmin, type User } from "./types";
import { getCachedAccess, setCachedAccess } from "./authz-cache";
import { computeWorkspaceRole } from "./compute-workspace-role";
import { getWorkspace, listWorkspaces } from "@/lib/k8s/workspace-client";
import {
  ROLE_HIERARCHY,
  ROLE_PERMISSIONS,
  NO_PERMISSIONS,
  MANAGE_ONLY_PERMISSIONS,
  type Workspace,
  type WorkspaceRole,
  type WorkspaceAccess,
  type AnonymousAccessConfig,
} from "@/types/workspace";

/** Access result for denied requests */
const DENIED_ACCESS: WorkspaceAccess = {
  granted: false,
  role: null,
  permissions: NO_PERMISSIONS,
};

/**
 * Access result when the workspace does not exist. Distinct from DENIED_ACCESS
 * (which means "exists but no role") so API guards can return 404 vs 403.
 */
const NOT_FOUND_ACCESS: WorkspaceAccess = {
  granted: false,
  role: null,
  notFound: true,
  permissions: NO_PERMISSIONS,
};

/**
 * A platform admin may see every workspace and manage its access bindings (so
 * they can self-grant a data role). Requires the global admin role AND an
 * authenticated session — an anonymous user must never get manage-all-
 * workspaces, even where dev sets anonymousRole=admin.
 */
export function isPlatformAdmin(user: User): boolean {
  return user.provider !== "anonymous" && canAdmin(user);
}

/**
 * Stable identity for authorization decisions (cache key, directGrant
 * matching, audit logging).
 *
 * Prefers the email claim — the conventional IdP identity — but falls back
 * to the username/UPN when the IdP doesn't emit an email claim. Microsoft
 * Entra, for example, only populates the `email` claim from the user's `mail`
 * attribute; a member with `mail` unset (no mailbox) authenticates fine and
 * carries a UPN but no email. Without this fallback such a user has an empty
 * `email` and gets dropped into the anonymous branch below — denied workspace
 * access despite being authenticated and correctly group-mapped.
 *
 * Returns undefined only for genuinely identity-less principals.
 */
function userIdentity(user: { email?: string; username?: string }): string | undefined {
  return user.email || user.username;
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
 * Get anonymous access configuration for a workspace.
 *
 * Logs warnings for elevated permissions (editor/owner) as these
 * grant anonymous users write access to resources.
 *
 * @param workspace - The workspace to check
 * @param requiredRole - Optional minimum role required
 * @returns WorkspaceAccess for anonymous users, or null if anonymous access disabled
 */
function getAnonymousAccess(
  workspace: Workspace,
  requiredRole?: WorkspaceRole
): WorkspaceAccess | null {
  const config: AnonymousAccessConfig | undefined = workspace.spec.anonymousAccess;

  // If no config or not enabled, anonymous users have no access
  if (!config?.enabled) {
    return null;
  }

  // Default to viewer if role not specified
  const role: WorkspaceRole = config.role ?? "viewer";

  // Log warnings for elevated anonymous permissions
  if (role === "owner") {
    console.warn(
      `[SECURITY WARNING] Workspace "${workspace.metadata.name}" grants OWNER access to anonymous users. ` +
      `Anonymous users can manage resources and workspace membership. ` +
      `This should only be used in isolated development environments.`
    );
  } else if (role === "editor") {
    console.warn(
      `[SECURITY WARNING] Workspace "${workspace.metadata.name}" grants EDITOR access to anonymous users. ` +
      `Anonymous users can create, modify, and delete resources. ` +
      `This should only be used in isolated development environments.`
    );
  }

  // Check if minimum role requirement is met
  if (requiredRole && !meetsRoleRequirement(role, requiredRole)) {
    return {
      granted: false,
      role,
      permissions: ROLE_PERMISSIONS[role],
    };
  }

  return {
    granted: true,
    role,
    permissions: ROLE_PERMISSIONS[role],
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
  const identity = userIdentity(user);

  // Workspace-scoped API keys: a non-empty allowlist confines the key. A
  // request for a workspace outside the allowlist is denied up front (#1561).
  const scopeWorkspaces = user.apiKeyScope?.workspaces;
  if (scopeWorkspaces && scopeWorkspaces.length > 0 && !scopeWorkspaces.includes(workspaceName)) {
    return DENIED_ACCESS;
  }

  // For anonymous users, check the workspace's anonymousAccess configuration
  if (user.provider === "anonymous" || !identity) {
    // Check if workspace exists
    const workspace = await getWorkspace(workspaceName);
    if (!workspace) {
      return NOT_FOUND_ACCESS;
    }

    // Check workspace's anonymous access configuration
    const anonymousAccess = getAnonymousAccess(workspace, requiredRole);
    if (anonymousAccess) {
      return anonymousAccess;
    }

    // No anonymous access configured for this workspace
    return DENIED_ACCESS;
  }

  // Check cache first (before K8s API call)
  const cached = getCachedAccess(identity, workspaceName);
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
    // Workspace doesn't exist - 404, not a permission denial
    return NOT_FOUND_ACCESS;
  }

  // Compute the highest role from roleBindings + directGrants
  const role = computeWorkspaceRole(
    {
      roleBindings: workspace.spec.roleBindings,
      directGrants: workspace.spec.directGrants,
      userGroups: user.groups,
      userIdentity: identity,
      isAnonymous: false,
    },
    Date.now()
  );

  // Platform admins may manage access on every workspace (so they can
  // self-grant a role), but hold NO data role until they do — a data
  // requiredRole therefore still denies them. Not cached: it derives from the
  // global role, not workspace state, and is cheap to recompute.
  // A scoped key (non-empty allowlist) must never gain platform-admin — that
  // would span all workspaces and defeat least-privilege (#1561).
  const isScopedKey = !!(scopeWorkspaces && scopeWorkspaces.length > 0);
  if (!role && isPlatformAdmin(user) && !isScopedKey) {
    return {
      granted: !requiredRole,
      role: null,
      permissions: MANAGE_ONLY_PERMISSIONS,
    };
  }

  // Build access result
  const access = buildAccess(role, requiredRole);

  // Cache the base access (without required role applied)
  const baseAccess = buildAccess(role);
  setCachedAccess(identity, workspaceName, baseAccess);

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
  const identity = userIdentity(user);

  // For anonymous users, check each workspace's anonymousAccess configuration
  if (user.provider === "anonymous" || !identity) {
    const workspaces = await listWorkspaces();
    const accessible: Array<{ workspace: Workspace; access: WorkspaceAccess }> = [];

    for (const workspace of workspaces) {
      const anonymousAccess = getAnonymousAccess(workspace, minimumRole);
      if (anonymousAccess?.granted) {
        accessible.push({ workspace, access: anonymousAccess });
      }
    }

    return accessible;
  }

  // Fetch all workspaces
  const workspaces = await listWorkspaces();
  const accessible: Array<{ workspace: Workspace; access: WorkspaceAccess }> = [];

  for (const workspace of workspaces) {
    // Compute the highest role from roleBindings + directGrants
    const role = computeWorkspaceRole(
      {
        roleBindings: workspace.spec.roleBindings,
        directGrants: workspace.spec.directGrants,
        userGroups: user.groups,
        userIdentity: identity,
        isAnonymous: false,
      },
      Date.now()
    );

    if (!role) {
      // Platform admins see every workspace (manage-only) so they can
      // self-grant. Only for the unfiltered listing — a data minimumRole
      // excludes manage-only, since the admin holds no data role yet.
      if (!minimumRole && isPlatformAdmin(user)) {
        accessible.push({
          workspace,
          access: { granted: true, role: null, permissions: MANAGE_ONLY_PERMISSIONS },
        });
      }
      continue;
    }

    // Apply minimum role filter
    if (minimumRole && !meetsRoleRequirement(role, minimumRole)) {
      continue;
    }

    const access = buildAccess(role);

    // Cache each workspace access
    setCachedAccess(identity, workspace.metadata.name, access);

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
