/**
 * API routes for individual Arena source operations.
 *
 * GET /api/workspaces/:name/arena/sources/:sourceName - Get source details
 * PUT /api/workspaces/:name/arena/sources/:sourceName - Update source
 * DELETE /api/workspaces/:name/arena/sources/:sourceName - Delete source
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { updateCrd, deleteCrd } from "@/lib/k8s/crd-operations";
import {
  validateWorkspace,
  getWorkspaceResource,
  handleK8sError,
  WORKSPACE_LABEL,
  CRD_ARENA_SOURCES,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaSource } from "@/types/arena";

type RouteParams = { name: string; sourceName: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

const CRD_KIND = "ArenaSource";

export const GET = withWorkspaceAccess<RouteParams>(
  "viewer",
  async (
    _request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, sourceName } = await context.params;
    let auditCtx;

    try {
      const result = await getWorkspaceResource<ArenaSource>(
        name,
        access.role!,
        CRD_ARENA_SOURCES,
        sourceName,
        "Arena source"
      );
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        name,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        CRD_KIND
      );

      auditSuccess(auditCtx, "get", sourceName);
      return NextResponse.json(result.resource);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", sourceName, error, 500);
      }
      return handleK8sError(error, "access this arena source");
    }
  }
);

export const PUT = withWorkspaceAccess<RouteParams>(
  "editor",
  async (
    request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, sourceName } = await context.params;
    let auditCtx;

    try {
      const result = await getWorkspaceResource<ArenaSource>(
        name,
        access.role!,
        CRD_ARENA_SOURCES,
        sourceName,
        "Arena source"
      );
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        name,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        CRD_KIND
      );

      const body = await request.json();
      const updated: ArenaSource = {
        ...result.resource,
        metadata: {
          ...result.resource.metadata,
          labels: {
            ...result.resource.metadata?.labels,
            ...body.metadata?.labels,
            [WORKSPACE_LABEL]: name,
          },
          annotations: {
            ...result.resource.metadata?.annotations,
            ...body.metadata?.annotations,
          },
        },
        spec: body.spec || result.resource.spec,
      };

      const saved = await updateCrd<ArenaSource>(
        result.clientOptions,
        CRD_ARENA_SOURCES,
        sourceName,
        updated
      );

      auditSuccess(auditCtx, "update", sourceName);
      return NextResponse.json(saved);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "update", sourceName, error, 500);
      }
      return handleK8sError(error, "update this arena source");
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
    const { name, sourceName } = await context.params;
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

      await deleteCrd(result.clientOptions, CRD_ARENA_SOURCES, sourceName);

      auditSuccess(auditCtx, "delete", sourceName);
      return new NextResponse(null, { status: 204 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "delete", sourceName, error, 500);
      }
      return handleK8sError(error, "delete this arena source");
    }
  }
);
