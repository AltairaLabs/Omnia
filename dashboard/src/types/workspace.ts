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
 * Anonymous access configuration for a workspace.
 *
 * Controls what access anonymous (unauthenticated) users have to this workspace.
 * Use with caution - granting editor or owner access to anonymous users
 * is a security risk and should only be used in isolated development environments.
 */
export interface AnonymousAccessConfig {
  /**
   * Whether anonymous users can access this workspace.
   * If false or omitted, anonymous users have no access.
   */
  enabled: boolean;
  /**
   * Role granted to anonymous users.
   * Defaults to "viewer" if enabled is true but role is not specified.
   *
   * WARNING: Setting this to "editor" or "owner" grants anonymous users
   * write access to resources. Only use in isolated dev environments.
   */
  role?: WorkspaceRole;
}

/**
 * Action to take when budget is exceeded.
 */
export type BudgetExceededAction = "warn" | "pauseJobs" | "block";

/**
 * Threshold for cost alerts.
 */
export interface CostAlertThreshold {
  /** Percentage of budget at which to trigger the alert (1-100) */
  percent: number;
  /** Email addresses to notify when threshold is reached */
  notify?: string[];
}

/**
 * Cost control settings for a workspace.
 */
export interface CostControls {
  /** Maximum daily spending limit in USD (e.g., "100.00") */
  dailyBudget?: string;
  /** Maximum monthly spending limit in USD (e.g., "2000.00") */
  monthlyBudget?: string;
  /** Action to take when budget is exceeded */
  budgetExceededAction?: BudgetExceededAction;
  /** Thresholds for cost alerts */
  alertThresholds?: CostAlertThreshold[];
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
    create?: boolean;
  };
  /** Group-based role bindings */
  roleBindings?: RoleBinding[];
  /** Individual user grants */
  directGrants?: DirectGrant[];
  /**
   * Anonymous access configuration.
   * If omitted, anonymous users have no access to this workspace.
   */
  anonymousAccess?: AnonymousAccessConfig;
  /** Cost control settings for budget and alerts */
  costControls?: CostControls;
}

/**
 * Current cost usage for a workspace.
 */
export interface CostUsage {
  /** Current day's spending in USD */
  dailySpend?: string;
  /** Configured daily budget in USD */
  dailyBudget?: string;
  /** Current month's spending in USD */
  monthlySpend?: string;
  /** Configured monthly budget in USD */
  monthlyBudget?: string;
  /** Timestamp of the last cost calculation */
  lastUpdated?: string;
}

/**
 * Workspace status from the Workspace CRD.
 */
export interface WorkspaceStatus {
  /** Current phase of the workspace */
  phase?: "Active" | "Terminating" | "Pending";
  /** Cost usage tracking */
  costUsage?: CostUsage;
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
