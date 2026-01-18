/**
 * Workspace access guard for API routes.
 *
 * Provides decorator-style middleware for protecting routes
 * that require workspace membership.
 *
 * Usage:
 *   import { withWorkspaceAccess } from "@/lib/auth/workspace-guard";
 *
 *   export const GET = withWorkspaceAccess("viewer", async (req, context, access) => {
 *     // access.role contains the user's role in the workspace
 *     return NextResponse.json({ workspace: context.params.name });
 *   });
 */

import { NextRequest, NextResponse } from "next/server";
import { getUser } from "./index";
import { checkWorkspaceAccess } from "./workspace-authz";
import type { WorkspaceRole, WorkspaceAccess } from "@/types/workspace";
import type { User } from "./types";

/**
 * Base route context with workspace name parameter.
 * Extended contexts can include additional params like agentName, packName, etc.
 */
export interface WorkspaceRouteContext<
  TParams extends { name: string } = { name: string }
> {
  params: Promise<TParams>;
}

/**
 * Handler function for workspace-protected routes.
 * Generic to support routes with additional params beyond the workspace name.
 */
type WorkspaceApiHandler<TParams extends { name: string } = { name: string }> = (
  request: NextRequest,
  context: WorkspaceRouteContext<TParams>,
  access: WorkspaceAccess,
  user: User
) => Promise<NextResponse> | NextResponse;

/**
 * Handler function for workspace-protected routes (simple version without context).
 */
type WorkspaceApiHandlerSimple = (
  request: NextRequest,
  workspaceName: string,
  access: WorkspaceAccess,
  user: User
) => Promise<NextResponse> | NextResponse;

/**
 * Returns 401 Unauthorized response for anonymous users.
 */
function unauthorizedResponse(): NextResponse {
  return NextResponse.json(
    { error: "Unauthorized", message: "Authentication required" },
    { status: 401 }
  );
}

/**
 * Returns 403 Forbidden response for workspace access denial.
 */
function forbiddenResponse(
  workspaceName: string,
  access: WorkspaceAccess,
  requiredRole?: WorkspaceRole
): NextResponse {
  if (access.role === null) {
    return NextResponse.json(
      {
        error: "Forbidden",
        message: `Access denied to workspace: ${workspaceName}`,
        workspace: workspaceName,
      },
      { status: 403 }
    );
  }
  return NextResponse.json(
    {
      error: "Forbidden",
      message: `Insufficient workspace permissions: requires ${requiredRole || "any"}, have ${access.role}`,
      workspace: workspaceName,
      required: requiredRole,
      current: access.role,
    },
    { status: 403 }
  );
}

/**
 * Wrap an API handler with workspace access checking.
 *
 * Extracts the workspace name from route params and checks if the current
 * user has at least the required role in that workspace.
 *
 * @param requiredRole - Minimum role required for access
 * @param handler - The handler to call if access is granted
 * @returns Wrapped handler that enforces workspace access
 *
 * @example
 * // In app/api/workspaces/[name]/route.ts
 * export const GET = withWorkspaceAccess("viewer", async (req, ctx, access, user) => {
 *   const { name } = await ctx.params;
 *   return NextResponse.json({ workspace: name, role: access.role });
 * });
 *
 * @example
 * // In app/api/workspaces/[name]/agents/[agentName]/route.ts
 * export const GET = withWorkspaceAccess<{ name: string; agentName: string }>(
 *   "viewer",
 *   async (req, ctx, access, user) => {
 *     const { name, agentName } = await ctx.params;
 *     return NextResponse.json({ workspace: name, agent: agentName });
 *   }
 * );
 */
export function withWorkspaceAccess<
  TParams extends { name: string } = { name: string }
>(
  requiredRole: WorkspaceRole,
  handler: WorkspaceApiHandler<TParams>
): (request: NextRequest, context: WorkspaceRouteContext<TParams>) => Promise<NextResponse> {
  return async (request: NextRequest, context: WorkspaceRouteContext<TParams>) => {
    const user = await getUser();
    if (user.provider === "anonymous") {
      return unauthorizedResponse();
    }

    const { name: workspaceName } = await context.params;
    const access = await checkWorkspaceAccess(workspaceName, requiredRole);

    if (!access.granted) {
      return forbiddenResponse(workspaceName, access, requiredRole);
    }

    return handler(request, context, access, user);
  };
}

/**
 * Wrap an API handler with workspace access checking (extracts name from query).
 *
 * Use this for routes where workspace is specified as a query parameter
 * instead of a path parameter.
 *
 * @param requiredRole - Minimum role required for access
 * @param handler - The handler to call if access is granted
 * @returns Wrapped handler that enforces workspace access
 *
 * @example
 * // For routes like /api/resources?workspace=my-workspace
 * export const GET = withWorkspaceQuery("viewer", async (req, workspace, access, user) => {
 *   return NextResponse.json({ workspace, role: access.role });
 * });
 */
export function withWorkspaceQuery(
  requiredRole: WorkspaceRole,
  handler: WorkspaceApiHandlerSimple
): (request: NextRequest) => Promise<NextResponse> {
  return async (request: NextRequest) => {
    const user = await getUser();
    if (user.provider === "anonymous") {
      return unauthorizedResponse();
    }

    const workspaceName = request.nextUrl.searchParams.get("workspace");
    if (!workspaceName) {
      return NextResponse.json(
        { error: "Bad Request", message: "Missing required query parameter: workspace" },
        { status: 400 }
      );
    }

    const access = await checkWorkspaceAccess(workspaceName, requiredRole);
    if (!access.granted) {
      return forbiddenResponse(workspaceName, access, requiredRole);
    }

    return handler(request, workspaceName, access, user);
  };
}

/**
 * Check workspace access without wrapping a handler.
 * Useful for conditional logic within handlers.
 *
 * @param workspaceName - The workspace to check
 * @param requiredRole - Optional minimum role required
 * @returns Access check result with user
 */
export async function checkWorkspace(
  workspaceName: string,
  requiredRole?: WorkspaceRole
): Promise<{ access: WorkspaceAccess; user: User }> {
  const user = await getUser();
  const access = await checkWorkspaceAccess(workspaceName, requiredRole);
  return { access, user };
}

/**
 * Build a workspace access denied response.
 * Helper for custom error handling.
 */
export function workspaceAccessDenied(
  workspaceName: string,
  access: WorkspaceAccess,
  requiredRole?: WorkspaceRole
): NextResponse {
  return forbiddenResponse(workspaceName, access, requiredRole);
}
