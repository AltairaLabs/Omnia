/**
 * API route for listing workspaces.
 *
 * GET /api/workspaces - List all workspaces the user has access to
 *
 * Query parameters:
 *   - minRole: Filter to workspaces where user has at least this role (viewer|editor|owner)
 *
 * Returns workspaces with the user's role and permissions in each.
 */

import { NextRequest, NextResponse } from "next/server";
import { getUser } from "@/lib/auth";
import { getAccessibleWorkspaces } from "@/lib/auth/workspace-authz";
import type { WorkspaceRole } from "@/types/workspace";

/**
 * Validate role query parameter.
 */
function isValidRole(role: string | null): role is WorkspaceRole {
  return role === "viewer" || role === "editor" || role === "owner";
}

/**
 * GET /api/workspaces
 *
 * List all workspaces the current user has access to.
 * Filters based on group membership (roleBindings) and individual grants (directGrants).
 */
export async function GET(request: NextRequest): Promise<NextResponse> {
  try {
    const user = await getUser();

    // Check authentication
    if (user.provider === "anonymous") {
      return NextResponse.json(
        {
          error: "Unauthorized",
          message: "Authentication required",
        },
        { status: 401 }
      );
    }

    // Parse optional minRole filter
    const minRoleParam = request.nextUrl.searchParams.get("minRole");
    let minRole: WorkspaceRole | undefined;

    if (minRoleParam) {
      if (!isValidRole(minRoleParam)) {
        return NextResponse.json(
          {
            error: "Bad Request",
            message: "Invalid minRole parameter. Must be: viewer, editor, or owner",
          },
          { status: 400 }
        );
      }
      minRole = minRoleParam;
    }

    // Get accessible workspaces
    const accessible = await getAccessibleWorkspaces(minRole);

    // Transform to API response format
    const workspaces = accessible.map(({ workspace, access }) => ({
      name: workspace.metadata.name,
      displayName: workspace.spec.displayName,
      description: workspace.spec.description,
      environment: workspace.spec.environment,
      namespace: workspace.spec.namespace.name,
      role: access.role,
      permissions: access.permissions,
      createdAt: workspace.metadata.creationTimestamp,
    }));

    return NextResponse.json({
      workspaces,
      count: workspaces.length,
    });
  } catch (error) {
    console.error("Failed to list workspaces:", error);
    return NextResponse.json(
      {
        error: "Internal Server Error",
        message: error instanceof Error ? error.message : "Failed to list workspaces",
      },
      { status: 500 }
    );
  }
}
