/**
 * API routes for individual workspace prompt pack operations.
 *
 * GET /api/workspaces/:name/promptpacks/:packName - Get prompt pack details
 * PUT /api/workspaces/:name/promptpacks/:packName - Update prompt pack
 * DELETE /api/workspaces/:name/promptpacks/:packName - Delete prompt pack
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
  CRD_PROMPTPACKS,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { PromptPack } from "@/lib/data/types";

type RouteParams = { name: string; packName: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

const CRD_KIND = "PromptPack";

export const GET = withWorkspaceAccess<RouteParams>(
  "viewer",
  async (
    _request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, packName } = await context.params;
    let auditCtx;

    try {
      const result = await getWorkspaceResource<PromptPack>(name, access.role!, CRD_PROMPTPACKS, packName, "Prompt pack");
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        name,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        CRD_KIND
      );

      auditSuccess(auditCtx, "get", packName);
      return NextResponse.json(result.resource);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", packName, error, 500);
      }
      return handleK8sError(error, "access this prompt pack");
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
    const { name, packName } = await context.params;
    let auditCtx;

    try {
      const result = await getWorkspaceResource<PromptPack>(name, access.role!, CRD_PROMPTPACKS, packName, "Prompt pack");
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        name,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        CRD_KIND
      );

      const body = await request.json();
      const updated: PromptPack = {
        ...result.resource,
        metadata: {
          ...result.resource.metadata,
          labels: { ...result.resource.metadata?.labels, ...body.metadata?.labels, [WORKSPACE_LABEL]: name },
          annotations: { ...result.resource.metadata?.annotations, ...body.metadata?.annotations },
        },
        spec: body.spec || result.resource.spec,
      };

      const saved = await updateCrd<PromptPack>(result.clientOptions, CRD_PROMPTPACKS, packName, updated);

      auditSuccess(auditCtx, "update", packName);
      return NextResponse.json(saved);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "update", packName, error, 500);
      }
      return handleK8sError(error, "update this prompt pack");
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
    const { name, packName } = await context.params;
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

      await deleteCrd(result.clientOptions, CRD_PROMPTPACKS, packName);

      auditSuccess(auditCtx, "delete", packName);
      return new NextResponse(null, { status: 204 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "delete", packName, error, 500);
      }
      return handleK8sError(error, "delete this prompt pack");
    }
  }
);
