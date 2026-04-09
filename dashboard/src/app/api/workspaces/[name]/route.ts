/**
 * API route for individual workspace operations.
 *
 * GET /api/workspaces/:name - Get workspace details
 * PATCH /api/workspaces/:name - Update workspace access settings
 *
 * Protected by workspace access checks. User must have at least viewer role for GET,
 * owner role for PATCH.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess } from "@/lib/auth/workspace-guard";
import { getWorkspace, patchWorkspace } from "@/lib/k8s/workspace-client";
import type { WorkspaceAccess, WorkspaceSpec } from "@/types/workspace";
import type { User } from "@/lib/auth/types";

interface RouteParams {
  params: Promise<{ name: string }>;
}

const ERR_INTERNAL = "Internal Server Error";

/**
 * PATCH /api/workspaces/:name
 *
 * Update access-related settings for a specific workspace.
 * Requires owner role in the workspace.
 *
 * Only the following fields are updatable:
 * - anonymousAccess
 * - roleBindings
 * - directGrants
 */
export const PATCH = withWorkspaceAccess(
  "owner",
  async (
    request: NextRequest,
    context: RouteParams,
    _access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    try {
      const { name } = await context.params;
      const body = await request.json() as Partial<WorkspaceSpec>;

      const allowed: Partial<WorkspaceSpec> = {};
      if (body.anonymousAccess !== undefined) {
        allowed.anonymousAccess = body.anonymousAccess;
      }
      if (body.roleBindings !== undefined) {
        allowed.roleBindings = body.roleBindings;
      }
      if (body.directGrants !== undefined) {
        allowed.directGrants = body.directGrants;
      }

      if (Object.keys(allowed).length === 0) {
        return NextResponse.json(
          {
            error: "Bad Request",
            message: "No updatable fields provided. Allowed fields: anonymousAccess, roleBindings, directGrants",
          },
          { status: 400 }
        );
      }

      const updated = await patchWorkspace(name, allowed);

      if (!updated) {
        return NextResponse.json(
          {
            error: ERR_INTERNAL,
            message: "Failed to patch workspace",
          },
          { status: 500 }
        );
      }

      return NextResponse.json({ workspace: updated });
    } catch (error) {
      console.error("Failed to patch workspace:", error);
      return NextResponse.json(
        {
          error: ERR_INTERNAL,
          message: error instanceof Error ? error.message : "Failed to patch workspace",
        },
        { status: 500 }
      );
    }
  }
);

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
          error: ERR_INTERNAL,
          message: error instanceof Error ? error.message : "Failed to get workspace",
        },
        { status: 500 }
      );
    }
  }
);
