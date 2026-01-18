/**
 * API route for individual workspace operations.
 *
 * GET /api/workspaces/:name - Get workspace details
 *
 * Protected by workspace access checks. User must have at least viewer role.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess } from "@/lib/auth/workspace-guard";
import { getWorkspace } from "@/lib/k8s/workspace-client";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";

interface RouteParams {
  params: Promise<{ name: string }>;
}

/**
 * GET /api/workspaces/:name
 *
 * Get details for a specific workspace.
 * Requires at least viewer role in the workspace.
 *
 * Response includes:
 * - Full workspace spec
 * - User's role and permissions
 * - Status information
 */
export const GET = withWorkspaceAccess(
  "viewer",
  async (
    _request: NextRequest,
    context: RouteParams,
    access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    try {
      const { name } = await context.params;

      // Fetch workspace (already validated to exist by guard)
      const workspace = await getWorkspace(name);

      if (!workspace) {
        // Should not happen as guard already checked, but handle gracefully
        return NextResponse.json(
          {
            error: "Not Found",
            message: `Workspace not found: ${name}`,
          },
          { status: 404 }
        );
      }

      // Build response with user's access info
      return NextResponse.json({
        workspace: {
          name: workspace.metadata.name,
          displayName: workspace.spec.displayName,
          description: workspace.spec.description,
          environment: workspace.spec.environment,
          namespace: workspace.spec.namespace,
          createdAt: workspace.metadata.creationTimestamp,
          labels: workspace.metadata.labels,
          annotations: workspace.metadata.annotations,
          status: workspace.status,
        },
        access: {
          role: access.role,
          permissions: access.permissions,
        },
        // Include membership info only for owners
        ...(access.permissions.manageMembers && {
          membership: {
            roleBindings: workspace.spec.roleBindings,
            directGrants: workspace.spec.directGrants,
          },
        }),
      });
    } catch (error) {
      console.error("Failed to get workspace:", error);
      return NextResponse.json(
        {
          error: "Internal Server Error",
          message: error instanceof Error ? error.message : "Failed to get workspace",
        },
        { status: 500 }
      );
    }
  }
);
