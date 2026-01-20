/**
 * API routes for individual Arena job operations.
 *
 * GET /api/workspaces/:name/arena/jobs/:jobName - Get job details
 * DELETE /api/workspaces/:name/arena/jobs/:jobName - Delete job
 *
 * Note: Jobs are immutable, so PUT is not supported.
 * Use cancel endpoint to stop a running job.
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { deleteCrd } from "@/lib/k8s/crd-operations";
import {
  validateWorkspace,
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

      auditSuccess(auditCtx, "get", jobName);
      return NextResponse.json(result.resource);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", jobName, error, 500);
      }
      return handleK8sError(error, "access this arena job");
    }
  }
);

export const DELETE = withWorkspaceAccess<RouteParams>(
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
      const result = await validateWorkspace(name, access.role!);
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        name,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        CRD_KIND
      );

      await deleteCrd(result.clientOptions, CRD_ARENA_JOBS, jobName);

      auditSuccess(auditCtx, "delete", jobName);
      return new NextResponse(null, { status: 204 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "delete", jobName, error, 500);
      }
      return handleK8sError(error, "delete this arena job");
    }
  }
);
