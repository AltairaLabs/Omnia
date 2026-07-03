/**
 * API route for workspace service health.
 *
 * GET /api/workspaces/:name/services - Get per-service health for a workspace
 * (session-api + memory-api per service group, plus workspace-level
 * privacy-api).
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { validateWorkspace, handleK8sError } from "@/lib/k8s/workspace-route-helpers";
import { getServiceHealth } from "@/lib/k8s/service-health";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";

export const GET = withWorkspaceAccess(
  "viewer",
  async (
    _request: NextRequest,
    context: WorkspaceRouteContext,
    access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    try {
      const { name } = await context.params;
      const result = await validateWorkspace(name, access.role!);
      if (!result.ok) return result.response;

      const health = await getServiceHealth(result.clientOptions, name);
      return NextResponse.json(health);
    } catch (error) {
      return handleK8sError(error, "fetch workspace service health");
    }
  }
);
