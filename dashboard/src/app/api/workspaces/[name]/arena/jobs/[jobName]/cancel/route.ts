/**
 * API route for cancelling an Arena job.
 *
 * POST /api/workspaces/:name/arena/jobs/:jobName/cancel - Cancel job
 *
 * Sets the cancelled flag on the job spec to stop execution.
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { patchCrd } from "@/lib/k8s/crd-operations";
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

export const POST = withWorkspaceAccess<RouteParams>(
  "editor",
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

      // Check if job is already succeeded, failed, or cancelled
      const phase = result.resource.status?.phase;
      if (phase === "Succeeded" || phase === "Failed" || phase === "Cancelled") {
        return NextResponse.json(
          { error: "Bad Request", message: `Job is already ${phase?.toLowerCase()}` },
          { status: 400 }
        );
      }

      // Patch the job to set cancelled: true
      const patch = {
        spec: {
          cancelled: true,
        },
      };

      await patchCrd(result.clientOptions, CRD_ARENA_JOBS, jobName, patch);

      auditSuccess(auditCtx, "patch", jobName, { action: "cancel" });
      return NextResponse.json({ message: "Job cancellation requested", jobName });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "patch", jobName, error, 500);
      }
      return handleK8sError(error, "cancel this arena job");
    }
  }
);
