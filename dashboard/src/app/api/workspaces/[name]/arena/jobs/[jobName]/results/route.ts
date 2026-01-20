/**
 * API route for getting Arena job results.
 *
 * GET /api/workspaces/:name/arena/jobs/:jobName/results - Get job results
 *
 * Returns the results from the job's status, including:
 * - Evaluation results (scores, details per scenario)
 * - Load test results (latency metrics, throughput)
 * - Data generation results (generated samples)
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import {
  getWorkspaceResource,
  handleK8sError,
  CRD_ARENA_JOBS,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaJob } from "@/types/arena";

type RouteParams = { name: string; jobName: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

const CRD_KIND = "ArenaJob";

export const GET = withWorkspaceAccess<RouteParams>(
  "viewer",
  async (
    _request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, jobName } = await context.params;
    let auditCtx;

    try {
      const result = await getWorkspaceResource<ArenaJob>(
        name,
        access.role!,
        CRD_ARENA_JOBS,
        jobName,
        "Arena job"
      );
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        name,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        CRD_KIND
      );

      // Results are stored at an external URL (status.resultsUrl)
      // Return the URL for the client to fetch, or null if not yet available
      const resultsUrl = result.resource.status?.resultsUrl || null;
      const phase = result.resource.status?.phase;

      auditSuccess(auditCtx, "get", jobName, { subresource: "results" });
      return NextResponse.json({
        jobName,
        phase,
        resultsUrl,
        completedTasks: result.resource.status?.completedTasks || 0,
        totalTasks: result.resource.status?.totalTasks || 0,
        failedTasks: result.resource.status?.failedTasks || 0,
      });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", jobName, error, 500);
      }
      return handleK8sError(error, "get results for this arena job");
    }
  }
);
