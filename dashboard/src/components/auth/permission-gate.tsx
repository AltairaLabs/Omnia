"use client";

/**
 * Permission-based conditional rendering components.
 *
 * Usage:
 *   <PermissionGate permission={Permission.AGENTS_DEPLOY}>
 *     <DeployButton />
 *   </PermissionGate>
 *
 *   <RequireRole role="editor" fallback={<ViewOnlyBadge />}>
 *     <EditButton />
 *   </RequireRole>
 */

import { type ReactNode } from "react";
import { useAuth } from "@/hooks/use-auth";
import {
  Permission,
  type PermissionType,
  roleHasPermission,
} from "@/lib/auth/permissions";
import type { UserRole } from "@/lib/auth/types";

// Re-export Permission for convenience
export { Permission };

interface PermissionGateProps {
  /** Required permission(s) */
  permission: PermissionType | PermissionType[];
  /** Require all permissions (default) or any */
  mode?: "all" | "any";
  /** Content to show if permission granted */
  children: ReactNode;
  /** Content to show if permission denied (optional) */
  fallback?: ReactNode;
}

/**
 * Conditionally render content based on user permissions.
 */
export function PermissionGate({
  permission,
  mode = "all",
  children,
  fallback = null,
}: PermissionGateProps) {
  const { role } = useAuth();

  const permissions = Array.isArray(permission) ? permission : [permission];
  const hasPermission =
    mode === "all"
      ? permissions.every((p) => roleHasPermission(role, p))
      : permissions.some((p) => roleHasPermission(role, p));

  if (!hasPermission) {
    return <>{fallback}</>;
  }

  return <>{children}</>;
}

interface RequireRoleProps {
  /** Minimum required role */
  role: UserRole;
  /** Content to show if role satisfied */
  children: ReactNode;
  /** Content to show if role not satisfied (optional) */
  fallback?: ReactNode;
}

/**
 * Conditionally render content based on user role.
 */
export function RequireRole({
  role: requiredRole,
  children,
  fallback = null,
}: RequireRoleProps) {
  const { hasRole } = useAuth();

  if (!hasRole(requiredRole)) {
    return <>{fallback}</>;
  }

  return <>{children}</>;
}

interface RequireAuthProps {
  /** Content to show if authenticated */
  children: ReactNode;
  /** Content to show if not authenticated (optional) */
  fallback?: ReactNode;
}

/**
 * Conditionally render content only for authenticated users.
 */
export function RequireAuth({ children, fallback = null }: RequireAuthProps) {
  const { isAuthenticated } = useAuth();

  if (!isAuthenticated) {
    return <>{fallback}</>;
  }

  return <>{children}</>;
}

interface CanWriteProps {
  /** Content to show if user can write */
  children: ReactNode;
  /** Content to show if user cannot write (optional) */
  fallback?: ReactNode;
}

/**
 * Convenience component for write permission checks.
 */
export function CanWrite({ children, fallback = null }: CanWriteProps) {
  const { canWrite } = useAuth();

  if (!canWrite) {
    return <>{fallback}</>;
  }

  return <>{children}</>;
}

interface CanAdminProps {
  /** Content to show if user is admin */
  children: ReactNode;
  /** Content to show if user is not admin (optional) */
  fallback?: ReactNode;
}

/**
 * Convenience component for admin permission checks.
 */
export function CanAdmin({ children, fallback = null }: CanAdminProps) {
  const { canAdmin } = useAuth();

  if (!canAdmin) {
    return <>{fallback}</>;
  }

  return <>{children}</>;
}
