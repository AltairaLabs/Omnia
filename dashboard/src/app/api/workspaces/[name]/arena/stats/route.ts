/**
 * API route for getting Arena statistics for a workspace.
 *
 * GET /api/workspaces/:name/arena/stats - Get arena stats
 *
 * Returns aggregated statistics:
 * - Sources: total, ready, failed, active
 * - Configs: total, ready, failed
 * - Jobs: total, running, completed, failed
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { listCrd } from "@/lib/k8s/crd-operations";
import {
  validateWorkspace,
  serverErrorResponse,
  CRD_ARENA_SOURCES,
  CRD_ARENA_CONFIGS,
  CRD_ARENA_JOBS,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaSource, ArenaConfig, ArenaJob, ArenaStats } from "@/types/arena";

export const GET = withWorkspaceAccess(
  "viewer",
  async (
    _request: NextRequest,
    context: WorkspaceRouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;
    let auditCtx;

    try {
      const result = await validateWorkspace(name, access.role!);
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        name,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        "ArenaStats"
      );

      // Fetch all arena resources in parallel
      const [sources, configs, jobs] = await Promise.all([
        listCrd<ArenaSource>(result.clientOptions, CRD_ARENA_SOURCES),
        listCrd<ArenaConfig>(result.clientOptions, CRD_ARENA_CONFIGS),
        listCrd<ArenaJob>(result.clientOptions, CRD_ARENA_JOBS),
      ]);

      // Calculate source stats
      const sourceStats = {
        total: sources.length,
        ready: sources.filter((s) => s.status?.phase === "Ready").length,
        failed: sources.filter((s) => s.status?.phase === "Failed").length,
        active: sources.filter((s) => s.status?.phase === "Ready").length, // Active = Ready sources
      };

      // Calculate config stats with total scenario count
      const totalScenarios = configs.reduce((sum, c) => sum + (c.status?.scenarioCount || 0), 0);
      const configStats = {
        total: configs.length,
        ready: configs.filter((c) => c.status?.phase === "Ready").length,
        scenarios: totalScenarios,
      };

      // Calculate job stats
      const completedJobs = jobs.filter((j) => j.status?.phase === "Completed").length;
      const failedJobs = jobs.filter((j) => j.status?.phase === "Failed" || j.status?.phase === "Cancelled").length;
      const totalFinished = completedJobs + failedJobs;
      const successRate = totalFinished > 0 ? completedJobs / totalFinished : 0;

      const jobStats = {
        total: jobs.length,
        running: jobs.filter((j) => j.status?.phase === "Running").length,
        queued: jobs.filter((j) => j.status?.phase === "Pending").length,
        completed: completedJobs,
        failed: failedJobs,
        successRate,
      };

      const stats: ArenaStats = {
        sources: sourceStats,
        configs: configStats,
        jobs: jobStats,
      };

      auditSuccess(auditCtx, "get", undefined, { subresource: "stats" });
      return NextResponse.json(stats);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", undefined, error, 500);
      }
      return serverErrorResponse(error, "Failed to get arena stats");
    }
  }
);
