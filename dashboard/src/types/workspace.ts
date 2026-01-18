/**
 * Workspace authorization types for Dashboard (#276)
 *
 * These types define the workspace membership model used for authorization.
 * Workspaces are Kubernetes CRDs that define team/project boundaries.
 */

import { ObjectMeta } from "./common";

/**
 * Role hierarchy: owner > editor > viewer
 * Each higher role includes all permissions of lower roles.
 */
export type WorkspaceRole = "owner" | "editor" | "viewer";

/**
 * Group-based role binding.
 * Maps OIDC groups or service accounts to workspace roles.
 */
export interface RoleBinding {
  /** OIDC groups that receive this role */
  groups?: string[];
  /** Kubernetes service accounts that receive this role */
  serviceAccounts?: { name: string; namespace: string }[];
  /** The role granted to these groups/service accounts */
  role: WorkspaceRole;
}

/**
 * Individual user grant.
 * Grants a specific user access to the workspace, optionally with expiration.
 */
export interface DirectGrant {
  /** User email address (from OIDC) */
  user: string;
  /** The role granted to this user */
  role: WorkspaceRole;
  /** Optional expiration timestamp (ISO 8601) */
  expires?: string;
}

/**
 * Workspace specification from the Workspace CRD.
 */
export interface WorkspaceSpec {
  /** Human-readable display name */
  displayName: string;
  /** Optional description of the workspace */
  description?: string;
  /** Environment tier for this workspace */
  environment: "development" | "staging" | "production";
  /** Kubernetes namespace configuration */
  namespace: {
    /** Namespace name */
    name: string;
    /** Whether to create the namespace if it doesn't exist */
    create: boolean;
  };
  /** Group-based role bindings */
  roleBindings: RoleBinding[];
  /** Individual user grants */
  directGrants?: DirectGrant[];
}

/**
 * Workspace status from the Workspace CRD.
 */
export interface WorkspaceStatus {
  /** Current phase of the workspace */
  phase?: "Active" | "Terminating" | "Pending";
  /** Conditions describing workspace state */
  conditions?: Array<{
    type: string;
    status: "True" | "False" | "Unknown";
    lastTransitionTime?: string;
    reason?: string;
    message?: string;
  }>;
}

/**
 * Full Workspace CRD resource.
 */
export interface Workspace {
  apiVersion: "omnia.altairalabs.ai/v1alpha1";
  kind: "Workspace";
  metadata: ObjectMeta;
  spec: WorkspaceSpec;
  status?: WorkspaceStatus;
}

/**
 * Result of a workspace access check.
 */
export interface WorkspaceAccess {
  /** Whether access was granted */
  granted: boolean;
  /** The role the user has (null if no access) */
  role: WorkspaceRole | null;
  /** Derived permissions based on role */
  permissions: WorkspacePermissions;
}

/**
 * Permissions derived from workspace role.
 */
export interface WorkspacePermissions {
  /** Can read workspace resources */
  read: boolean;
  /** Can create/update workspace resources */
  write: boolean;
  /** Can delete workspace resources */
  delete: boolean;
  /** Can manage workspace membership */
  manageMembers: boolean;
}

/**
 * Role hierarchy values for comparison.
 * Higher number = more permissions.
 */
export const ROLE_HIERARCHY: Record<WorkspaceRole, number> = {
  viewer: 1,
  editor: 2,
  owner: 3,
};

/**
 * Permissions granted by each role.
 */
export const ROLE_PERMISSIONS: Record<WorkspaceRole, WorkspacePermissions> = {
  viewer: {
    read: true,
    write: false,
    delete: false,
    manageMembers: false,
  },
  editor: {
    read: true,
    write: true,
    delete: true,
    manageMembers: false,
  },
  owner: {
    read: true,
    write: true,
    delete: true,
    manageMembers: true,
  },
};

/**
 * No permissions (for denied access).
 */
export const NO_PERMISSIONS: WorkspacePermissions = {
  read: false,
  write: false,
  delete: false,
  manageMembers: false,
};
