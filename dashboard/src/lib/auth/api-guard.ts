/**
 * API route guards for permission checking.
 *
 * Usage in API routes:
 *   import { withPermission, withRole } from "@/lib/auth/api-guard";
 *   import { Permission } from "@/lib/auth/permissions";
 *
 *   export const POST = withPermission(Permission.AGENTS_DEPLOY, async (req, user) => {
 *     // user is guaranteed to have the permission
 *     return NextResponse.json({ success: true });
 *   });
 */

import { NextRequest, NextResponse } from "next/server";
import { getUser } from "./index";
import { userHasPermission, userHasAllPermissions, type PermissionType } from "./permissions";
import type { User, UserRole } from "./types";

type ApiHandler = (
  request: NextRequest,
  user: User
) => Promise<NextResponse> | NextResponse;

type ApiHandlerWithContext<T> = (
  request: NextRequest,
  context: T,
  user: User
) => Promise<NextResponse> | NextResponse;

/**
 * Wrap an API handler with permission checking.
 * Returns 403 if user lacks the required permission.
 */
export function withPermission(
  permission: PermissionType,
  handler: ApiHandler
): (request: NextRequest) => Promise<NextResponse> {
  return async (request: NextRequest) => {
    const user = await getUser();

    if (!userHasPermission(user, permission)) {
      return NextResponse.json(
        {
          error: "Forbidden",
          message: `Insufficient permissions: requires ${permission}`,
          required: permission,
        },
        { status: 403 }
      );
    }

    return handler(request, user);
  };
}

/**
 * Wrap an API handler with permission checking (with route context).
 * Use this for dynamic routes like /api/agents/[name].
 */
export function withPermissionAndContext<T>(
  permission: PermissionType,
  handler: ApiHandlerWithContext<T>
): (request: NextRequest, context: T) => Promise<NextResponse> {
  return async (request: NextRequest, context: T) => {
    const user = await getUser();

    if (!userHasPermission(user, permission)) {
      return NextResponse.json(
        {
          error: "Forbidden",
          message: `Insufficient permissions: requires ${permission}`,
          required: permission,
        },
        { status: 403 }
      );
    }

    return handler(request, context, user);
  };
}

/**
 * Wrap an API handler requiring all specified permissions.
 */
export function withAllPermissions(
  permissions: PermissionType[],
  handler: ApiHandler
): (request: NextRequest) => Promise<NextResponse> {
  return async (request: NextRequest) => {
    const user = await getUser();

    if (!userHasAllPermissions(user, permissions)) {
      const missing = permissions.filter((p) => !userHasPermission(user, p));
      return NextResponse.json(
        {
          error: "Forbidden",
          message: `Insufficient permissions: requires ${missing.join(", ")}`,
          required: permissions,
          missing,
        },
        { status: 403 }
      );
    }

    return handler(request, user);
  };
}

/**
 * Wrap an API handler with role checking.
 * Returns 403 if user doesn't have at least the required role.
 */
export function withRole(
  requiredRole: UserRole,
  handler: ApiHandler
): (request: NextRequest) => Promise<NextResponse> {
  return async (request: NextRequest) => {
    const user = await getUser();
    const roleHierarchy: Record<UserRole, number> = {
      admin: 3,
      editor: 2,
      viewer: 1,
    };

    if (roleHierarchy[user.role] < roleHierarchy[requiredRole]) {
      return NextResponse.json(
        {
          error: "Forbidden",
          message: `Insufficient permissions: requires ${requiredRole} role`,
          required: requiredRole,
          current: user.role,
        },
        { status: 403 }
      );
    }

    return handler(request, user);
  };
}

/**
 * Wrap an API handler requiring authentication (any role).
 * Returns 401 if user is anonymous.
 */
export function withAuth(
  handler: ApiHandler
): (request: NextRequest) => Promise<NextResponse> {
  return async (request: NextRequest) => {
    const user = await getUser();

    if (user.provider === "anonymous") {
      return NextResponse.json(
        {
          error: "Unauthorized",
          message: "Authentication required",
        },
        { status: 401 }
      );
    }

    return handler(request, user);
  };
}

/**
 * Check permissions for the current user without wrapping.
 * Useful for conditional logic within handlers.
 */
export async function checkPermission(
  permission: PermissionType
): Promise<{ allowed: boolean; user: User }> {
  const user = await getUser();
  return {
    allowed: userHasPermission(user, permission),
    user,
  };
}
