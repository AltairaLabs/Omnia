"use client";

/**
 * Permission-aware wrapper component for UI security trimming.
 *
 * Hides or disables children based on workspace permissions.
 *
 * Usage:
 *   // Hide if no permission
 *   <RequirePermission permission="write">
 *     <CreateButton />
 *   </RequirePermission>
 *
 *   // Disable if no permission
 *   <RequirePermission permission="write" fallback="disable">
 *     <ScaleButton />
 *   </RequirePermission>
 *
 *   // Custom fallback
 *   <RequirePermission permission="delete" fallback={<Tooltip content="No permission">...</Tooltip>}>
 *     <DeleteButton />
 *   </RequirePermission>
 */

import { type ReactNode, type ReactElement, cloneElement, isValidElement } from "react";
import {
  useWorkspacePermissions,
  type PermissionType,
} from "@/hooks/use-workspace-permissions";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";

/**
 * Props for RequirePermission component.
 */
interface RequirePermissionProps {
  /** The permission required to show/enable children */
  permission: PermissionType;

  /** Children to render if permission is granted */
  children: ReactNode;

  /**
   * Behavior when permission is denied:
   * - "hide" (default): Don't render children
   * - "disable": Render children with disabled prop
   * - ReactNode: Render the fallback instead
   */
  fallback?: "hide" | "disable" | ReactNode;

  /** Tooltip message shown when hovering over disabled element */
  disabledTooltip?: string;
}

/**
 * Default tooltip messages for disabled permissions.
 */
const DEFAULT_TOOLTIPS: Record<PermissionType, string> = {
  read: "You don't have permission to view this resource",
  write: "You don't have permission to modify this resource",
  delete: "You don't have permission to delete this resource",
  manageMembers: "You don't have permission to manage workspace members",
};

/**
 * Wrapper component that conditionally renders children based on permissions.
 *
 * @example
 * ```tsx
 * // Hide create button for viewers
 * <RequirePermission permission="write">
 *   <Button onClick={onCreate}>Create Agent</Button>
 * </RequirePermission>
 *
 * // Disable scale buttons for viewers with tooltip
 * <RequirePermission permission="write" fallback="disable">
 *   <Button onClick={onScale}>Scale</Button>
 * </RequirePermission>
 * ```
 */
export function RequirePermission({
  permission,
  children,
  fallback = "hide",
  disabledTooltip,
}: RequirePermissionProps) {
  const { permissions } = useWorkspacePermissions();
  const hasPermission = permissions[permission];

  // If permission is granted, render children normally
  if (hasPermission) {
    return <>{children}</>;
  }

  // Handle different fallback behaviors
  if (fallback === "hide") {
    return null;
  }

  if (fallback === "disable") {
    const tooltip = disabledTooltip ?? DEFAULT_TOOLTIPS[permission];

    // Try to add disabled prop to child element
    if (isValidElement(children)) {
      const disabledChild = cloneElement(children as ReactElement<{ disabled?: boolean }>, {
        disabled: true,
      });

      // Wrap in tooltip to explain why it's disabled
      return (
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="inline-block">{disabledChild}</span>
          </TooltipTrigger>
          <TooltipContent>
            <p>{tooltip}</p>
          </TooltipContent>
        </Tooltip>
      );
    }

    // If children isn't a single element, just return it
    return <>{children}</>;
  }

  // Custom fallback element
  return <>{fallback}</>;
}

/**
 * Props for PermissionGate component (alternative API).
 */
interface PermissionGateProps {
  /** The permission required */
  permission: PermissionType;

  /** Render function that receives permission state */
  children: (props: { hasPermission: boolean; disabled: boolean }) => ReactNode;
}

/**
 * Render prop component for more complex permission-based rendering.
 *
 * @example
 * ```tsx
 * <PermissionGate permission="write">
 *   {({ hasPermission, disabled }) => (
 *     <Button
 *       disabled={disabled}
 *       variant={hasPermission ? "default" : "outline"}
 *       onClick={hasPermission ? onCreate : showUpgradeDialog}
 *     >
 *       Create
 *     </Button>
 *   )}
 * </PermissionGate>
 * ```
 */
export function PermissionGate({ permission, children }: PermissionGateProps) {
  const { permissions } = useWorkspacePermissions();
  const hasPermission = permissions[permission];

  return <>{children({ hasPermission, disabled: !hasPermission })}</>;
}
