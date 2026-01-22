/**
 * API route for getting Arena job worker logs.
 *
 * GET /api/workspaces/:name/arena/jobs/:jobName/logs - Get logs for job worker pods
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { getPodLogs } from "@/lib/k8s/crd-operations";
import { getWorkspaceResource, handleK8sError, CRD_ARENA_JOBS } from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaJob } from "@/types/arena";

type RouteParams = { name: string; jobName: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

export const GET = withWorkspaceAccess<RouteParams>(
  "viewer",
  async (
    request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    try {
      const { name, jobName } = await context.params;

      // Verify job exists
      const result = await getWorkspaceResource<ArenaJob>(
        name,
        access.role!,
        CRD_ARENA_JOBS,
        jobName,
        "Arena job"
      );
      if (!result.ok) return result.response;

      const { searchParams } = new URL(request.url);
      const tailLines = Number.parseInt(searchParams.get("tailLines") || searchParams.get("lines") || "100", 10);
      const sinceSeconds = searchParams.get("sinceSeconds")
        ? Number.parseInt(searchParams.get("sinceSeconds")!, 10)
        : undefined;

      // Worker pods are labeled with omnia.altairalabs.ai/job={jobName}
      const logs = await getPodLogs(
        result.clientOptions,
        `omnia.altairalabs.ai/job=${jobName}`,
        tailLines,
        sinceSeconds,
        undefined // No container filter - arena workers have single container
      );

      return NextResponse.json(logs);
    } catch (error) {
      return handleK8sError(error, "access arena job logs");
    }
  }
);
