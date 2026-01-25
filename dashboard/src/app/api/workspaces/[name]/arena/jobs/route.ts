/**
 * API routes for Arena jobs (ArenaJob CRDs).
 *
 * GET /api/workspaces/:name/arena/jobs - List arena jobs
 * POST /api/workspaces/:name/arena/jobs - Create a new arena job
 *
 * Query parameters for GET:
 * - type: Filter by job type (evaluation, loadtest, datagen)
 * - status: Filter by status (Pending, Running, Completed, Failed, Cancelled)
 * - sourceRef: Filter by source reference name
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { listCrd, createCrd } from "@/lib/k8s/crd-operations";
import {
  validateWorkspace,
  serverErrorResponse,
  buildCrdResource,
  CRD_ARENA_JOBS,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaJob, ArenaJobType, ArenaJobPhase } from "@/types/arena";

const CRD_KIND = "ArenaJob";

export const GET = withWorkspaceAccess(
  "viewer",
  async (
    request: NextRequest,
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
        CRD_KIND
      );

      let jobs = await listCrd<ArenaJob>(result.clientOptions, CRD_ARENA_JOBS);

      // Apply filters from query parameters
      const { searchParams } = new URL(request.url);
      const typeFilter = searchParams.get("type") as ArenaJobType | null;
      const statusFilter = searchParams.get("status") as ArenaJobPhase | null;
      const sourceRefFilter = searchParams.get("sourceRef");

      if (typeFilter) {
        jobs = jobs.filter((job) => job.spec.type === typeFilter);
      }
      if (statusFilter) {
        jobs = jobs.filter((job) => job.status?.phase === statusFilter);
      }
      if (sourceRefFilter) {
        jobs = jobs.filter((job) => job.spec.sourceRef.name === sourceRefFilter);
      }

      auditSuccess(auditCtx, "list", undefined, { count: jobs.length });
      return NextResponse.json(jobs);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "list", undefined, error, 500);
      }
      return serverErrorResponse(error, "Failed to list arena jobs");
    }
  }
);

export const POST = withWorkspaceAccess(
  "editor",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;
    let auditCtx;
    let resourceName = "";

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

      const body = await request.json();
      resourceName = body.metadata?.name || body.name || "";

      const job = buildCrdResource(
        CRD_KIND,
        name,
        result.workspace.spec.namespace.name,
        resourceName,
        body.spec,
        body.metadata?.labels,
        body.metadata?.annotations
      );

      const created = await createCrd<ArenaJob>(
        result.clientOptions,
        CRD_ARENA_JOBS,
        job as unknown as ArenaJob
      );

      auditSuccess(auditCtx, "create", resourceName);
      return NextResponse.json(created, { status: 201 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "create", resourceName, error, 500);
      }
      return serverErrorResponse(error, "Failed to create arena job");
    }
  }
);
