/**
 * API routes for individual Arena config operations.
 *
 * GET /api/workspaces/:name/arena/configs/:configName - Get config details
 * PUT /api/workspaces/:name/arena/configs/:configName - Update config
 * DELETE /api/workspaces/:name/arena/configs/:configName - Delete config
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
  CRD_ARENA_CONFIGS,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaConfig } from "@/types/arena";

type RouteParams = { name: string; configName: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

const CRD_KIND = "ArenaConfig";

export const GET = withWorkspaceAccess<RouteParams>(
  "viewer",
  async (
    _request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, configName } = await context.params;
    let auditCtx;

    try {
      const result = await getWorkspaceResource<ArenaConfig>(
        name,
        access.role!,
        CRD_ARENA_CONFIGS,
        configName,
        "Arena config"
      );
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        name,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        CRD_KIND
      );

      auditSuccess(auditCtx, "get", configName);
      return NextResponse.json(result.resource);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", configName, error, 500);
      }
      return handleK8sError(error, "access this arena config");
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
    const { name, configName } = await context.params;
    let auditCtx;

    try {
      const result = await getWorkspaceResource<ArenaConfig>(
        name,
        access.role!,
        CRD_ARENA_CONFIGS,
        configName,
        "Arena config"
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
      const updated: ArenaConfig = {
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

      const saved = await updateCrd<ArenaConfig>(
        result.clientOptions,
        CRD_ARENA_CONFIGS,
        configName,
        updated
      );

      auditSuccess(auditCtx, "update", configName);
      return NextResponse.json(saved);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "update", configName, error, 500);
      }
      return handleK8sError(error, "update this arena config");
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
    const { name, configName } = await context.params;
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

      await deleteCrd(result.clientOptions, CRD_ARENA_CONFIGS, configName);

      auditSuccess(auditCtx, "delete", configName);
      return new NextResponse(null, { status: 204 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "delete", configName, error, 500);
      }
      return handleK8sError(error, "delete this arena config");
    }
  }
);
