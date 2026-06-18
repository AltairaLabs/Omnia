/**
 * Pure workspace role computation.
 *
 * This is the framework-free core of the workspace authorization logic in
 * workspace-authz.ts: given the workspace's roleBindings / directGrants /
 * anonymousAccess and the principal's groups + identity, it returns the
 * highest-privilege role granted (or null for no access).
 *
 * It is the TS half of the Go↔TS parity contract — it MUST stay semantically
 * identical to pkg/workspaceauth ComputeRole (enforced by parity.test.ts which
 * runs both sides against the shared fixture).
 *
 * `now` is injected (ms epoch) for deterministic direct-grant expiry.
 */

import {
  ROLE_HIERARCHY,
  type WorkspaceRole,
  type RoleBinding,
  type DirectGrant,
  type AnonymousAccessConfig,
} from "@/types/workspace";

/** Decoupled input for computeWorkspaceRole — mirrors Go workspaceauth.Inputs. */
export interface ComputeRoleInput {
  roleBindings?: RoleBinding[];
  directGrants?: DirectGrant[];
  anonymousAccess?: AnonymousAccessConfig;
  userGroups: string[];
  /** email-or-username; undefined/empty = identity-less principal */
  userIdentity?: string;
  isAnonymous: boolean;
}

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
  return ROLE_HIERARCHY[role1] >= ROLE_HIERARCHY[role2] ? role1 : role2;
}

/**
 * Find the highest role granted to a user through group membership.
 */
function findRoleFromBindings(
  roleBindings: RoleBinding[],
  userGroups: string[]
): WorkspaceRole | null {
  let highestRole: WorkspaceRole | null = null;

  for (const binding of roleBindings) {
    if (!binding.groups) continue;

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
 * Find a direct grant for a specific user by identity (case-insensitive).
 * Ignores grants whose expiry is before `now`.
 */
function findDirectGrant(
  directGrants: DirectGrant[] | undefined,
  userIdentity: string,
  now: number
): WorkspaceRole | null {
  if (!directGrants || !userIdentity) return null;

  for (const grant of directGrants) {
    if (grant.user.toLowerCase() !== userIdentity.toLowerCase()) continue;

    if (grant.expires) {
      const expiresAt = new Date(grant.expires).getTime();
      if (now > expiresAt) {
        continue;
      }
    }

    return grant.role;
  }

  return null;
}

/**
 * Compute the workspace role granted to a principal.
 *
 * @param input - workspace config + principal identity/groups
 * @param now - ms epoch used for direct-grant expiry comparison
 * @returns the highest role granted, or null for no access
 */
export function computeWorkspaceRole(
  input: ComputeRoleInput,
  now: number
): WorkspaceRole | null {
  if (input.isAnonymous || !input.userIdentity) {
    if (input.anonymousAccess?.enabled) {
      return input.anonymousAccess.role ?? "viewer";
    }
    return null;
  }

  let role = findRoleFromBindings(input.roleBindings || [], input.userGroups);
  const directRole = findDirectGrant(
    input.directGrants,
    input.userIdentity,
    now
  );
  role = maxRole(role, directRole);

  return role;
}
