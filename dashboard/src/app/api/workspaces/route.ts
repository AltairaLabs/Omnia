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
import { getAccessibleWorkspaces } from "@/lib/auth/workspace-authz";
import { ROLE_PERMISSIONS, type WorkspaceRole } from "@/types/workspace";

/**
 * Check if demo mode is enabled.
 */
function isDemoMode(): boolean {
  return process.env.NEXT_PUBLIC_DEMO_MODE === "true";
}

/**
 * Mock workspaces for demo mode.
 */
const MOCK_WORKSPACES = [
  {
    name: "default",
    displayName: "Default Workspace",
    description: "Default development workspace for demo",
    environment: "development" as const,
    namespace: "default",
    role: "owner" as WorkspaceRole,
    permissions: ROLE_PERMISSIONS.owner,
    createdAt: new Date(Date.now() - 30 * 24 * 60 * 60 * 1000).toISOString(),
  },
  {
    name: "production",
    displayName: "Production",
    description: "Production environment workspace",
    environment: "production" as const,
    namespace: "production",
    role: "editor" as WorkspaceRole,
    permissions: { view: true, create: true, edit: true, delete: false, scale: true },
    createdAt: new Date(Date.now() - 60 * 24 * 60 * 60 * 1000).toISOString(),
  },
  {
    name: "staging",
    displayName: "Staging",
    description: "Staging environment for testing",
    environment: "staging" as const,
    namespace: "staging",
    role: "viewer" as WorkspaceRole,
    permissions: { view: true, create: false, edit: false, delete: false, scale: false },
    createdAt: new Date(Date.now() - 45 * 24 * 60 * 60 * 1000).toISOString(),
  },
];

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
 * In demo mode, returns mock workspaces.
 */
export async function GET(request: NextRequest): Promise<NextResponse> {
  try {
    // In demo mode, return mock workspaces (no K8s connection needed)
    if (isDemoMode()) {
      return NextResponse.json({
        workspaces: MOCK_WORKSPACES,
        count: MOCK_WORKSPACES.length,
      });
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
