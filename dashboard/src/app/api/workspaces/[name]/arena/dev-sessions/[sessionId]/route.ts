/**
 * API routes for individual Arena dev sessions.
 *
 * GET /api/workspaces/:name/arena/dev-sessions/:sessionId - Get a dev session
 * PATCH /api/workspaces/:name/arena/dev-sessions/:sessionId - Update session (e.g., heartbeat)
 * DELETE /api/workspaces/:name/arena/dev-sessions/:sessionId - Delete a dev session
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { getCrd, patchCrd, deleteCrd } from "@/lib/k8s/crd-operations";
import {
  validateWorkspace,
  handleK8sError,
  notFoundResponse,
  CRD_ARENA_DEV_SESSIONS,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaDevSession } from "@/types/arena";

type RouteParams = { name: string; sessionId: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

const CRD_KIND = "ArenaDevSession";

export const GET = withWorkspaceAccess<RouteParams>(
  "viewer",
  async (
    _request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, sessionId } = await context.params;
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

      const session = await getCrd<ArenaDevSession>(
        result.clientOptions,
        CRD_ARENA_DEV_SESSIONS,
        sessionId
      );

      if (!session) {
        auditError(auditCtx, "get", sessionId, "Not found", 404);
        return notFoundResponse(`Dev session not found: ${sessionId}`);
      }

      auditSuccess(auditCtx, "get", sessionId);
      return NextResponse.json(session);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", sessionId, error, 500);
      }
      return handleK8sError(error, "get arena dev session");
    }
  }
);

export const PATCH = withWorkspaceAccess<RouteParams>(
  "editor",
  async (
    request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, sessionId } = await context.params;
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

      const body = await request.json();

      // Support updating lastActivityAt for heartbeat
      const patch: Record<string, unknown> = {};
      if (body.status) {
        patch.status = body.status;
      }
      if (body.spec) {
        patch.spec = body.spec;
      }

      const updated = await patchCrd<ArenaDevSession>(
        result.clientOptions,
        CRD_ARENA_DEV_SESSIONS,
        sessionId,
        patch
      );

      auditSuccess(auditCtx, "update", sessionId);
      return NextResponse.json(updated);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "update", sessionId, error, 500);
      }
      return handleK8sError(error, "update arena dev session");
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
    const { name, sessionId } = await context.params;
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

      await deleteCrd(result.clientOptions, CRD_ARENA_DEV_SESSIONS, sessionId);

      auditSuccess(auditCtx, "delete", sessionId);
      return new NextResponse(null, { status: 204 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "delete", sessionId, error, 500);
      }
      return handleK8sError(error, "delete arena dev session");
    }
  }
);
