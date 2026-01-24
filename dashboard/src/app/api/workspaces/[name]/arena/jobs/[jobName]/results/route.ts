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

      // Results are stored at an external URL (status.result.url)
      // Return the URL for the client to fetch, or null if not yet available
      const status = result.resource.status;
      const resultsUrl = status?.result?.url || null;
      const phase = status?.phase;
      const resultSummary = status?.result?.summary;

      auditSuccess(auditCtx, "get", jobName, { subresource: "results" });
      return NextResponse.json({
        jobName,
        phase,
        resultsUrl,
        // Work item progress
        completedTasks: status?.progress?.completed || 0,
        totalTasks: status?.progress?.total || 0,
        failedTasks: status?.progress?.failed || 0,
        // Test result summary (if available)
        summary: resultSummary ? {
          totalItems: parseInt(resultSummary.totalItems || "0", 10),
          passedItems: parseInt(resultSummary.passedItems || "0", 10),
          failedItems: parseInt(resultSummary.failedItems || "0", 10),
          passRate: resultSummary.passRate,
          avgDurationMs: resultSummary.avgDurationMs,
        } : null,
      });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", jobName, error, 500);
      }
      return handleK8sError(error, "get results for this arena job");
    }
  }
);
