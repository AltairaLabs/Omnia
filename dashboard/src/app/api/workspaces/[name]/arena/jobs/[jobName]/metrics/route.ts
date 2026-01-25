/**
 * API route for getting Arena job metrics.
 *
 * GET /api/workspaces/:name/arena/jobs/:jobName/metrics - Get job metrics
 *
 * Returns real-time metrics for running jobs:
 * - Progress percentage
 * - Current RPS (for load tests)
 * - Latency percentiles
 * - Error rates
 * - Active workers
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
import type { ArenaJob, ArenaJobMetrics } from "@/types/arena";

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

      // Build metrics from job status
      const status = result.resource.status;
      const totalTasks = status?.progress?.total || 0;
      const completedTasks = status?.progress?.completed || 0;
      const progressPct = totalTasks > 0 ? Math.round((completedTasks / totalTasks) * 100) : 0;

      const metrics: ArenaJobMetrics = {
        progress: progressPct,
        activeWorkers: status?.activeWorkers,
        completedScenarios: completedTasks,
        totalScenarios: totalTasks,
      };

      auditSuccess(auditCtx, "get", jobName, { subresource: "metrics" });
      return NextResponse.json(metrics);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", jobName, error, 500);
      }
      return handleK8sError(error, "get metrics for this arena job");
    }
  }
);
