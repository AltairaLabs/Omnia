/**
 * API routes for Arena dev sessions (ArenaDevSession CRDs).
 *
 * GET /api/workspaces/:name/arena/dev-sessions - List dev sessions
 * POST /api/workspaces/:name/arena/dev-sessions - Create a new dev session
 *
 * Query parameters for GET:
 * - projectId: Filter by project ID
 * - phase: Filter by phase (Pending, Starting, Ready, Stopping, Stopped, Failed)
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { listCrd, createCrd } from "@/lib/k8s/crd-operations";
import {
  validateWorkspace,
  handleK8sError,
  buildCrdResource,
  CRD_ARENA_DEV_SESSIONS,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaDevSession, ArenaDevSessionPhase } from "@/types/arena";

const CRD_KIND = "ArenaDevSession";

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

      let sessions = await listCrd<ArenaDevSession>(result.clientOptions, CRD_ARENA_DEV_SESSIONS);

      // Apply filters from query parameters
      const { searchParams } = new URL(request.url);
      const projectIdFilter = searchParams.get("projectId");
      const phaseFilter = searchParams.get("phase") as ArenaDevSessionPhase | null;

      if (projectIdFilter) {
        sessions = sessions.filter((session) => session.spec.projectId === projectIdFilter);
      }
      if (phaseFilter) {
        sessions = sessions.filter((session) => session.status?.phase === phaseFilter);
      }

      auditSuccess(auditCtx, "list", undefined, { count: sessions.length });
      return NextResponse.json(sessions);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "list", undefined, error, 500);
      }
      return handleK8sError(error, "list arena dev sessions");
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

      // Validate required fields
      if (!body.projectId) {
        return NextResponse.json(
          { error: "Bad Request", message: "projectId is required" },
          { status: 400 }
        );
      }

      // Generate a unique name if not provided
      resourceName = body.name || `dev-session-${body.projectId}-${Date.now().toString(36)}`;

      // Build the spec with workspace
      const spec = {
        projectId: body.projectId,
        workspace: name,
        idleTimeout: body.idleTimeout || "30m",
        image: body.image,
        resources: body.resources,
      };

      const session = buildCrdResource(
        CRD_KIND,
        name,
        result.workspace.spec.namespace.name,
        resourceName,
        spec,
        body.metadata?.labels,
        body.metadata?.annotations
      );

      const created = await createCrd<ArenaDevSession>(
        result.clientOptions,
        CRD_ARENA_DEV_SESSIONS,
        session as unknown as ArenaDevSession
      );

      auditSuccess(auditCtx, "create", resourceName);
      return NextResponse.json(created, { status: 201 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "create", resourceName, error, 500);
      }
      return handleK8sError(error, "create arena dev session");
    }
  }
);
