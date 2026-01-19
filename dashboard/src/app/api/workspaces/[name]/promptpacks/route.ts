/**
 * API routes for workspace prompt packs (PromptPack CRDs).
 *
 * GET /api/workspaces/:name/promptpacks - List prompt packs in workspace
 * POST /api/workspaces/:name/promptpacks - Create a new prompt pack
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
  CRD_PROMPTPACKS,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { PromptPack } from "@/lib/data/types";

const CRD_KIND = "PromptPack";

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
        CRD_KIND
      );

      const promptPacks = await listCrd<PromptPack>(result.clientOptions, CRD_PROMPTPACKS);

      auditSuccess(auditCtx, "list", undefined, { count: promptPacks.length });
      return NextResponse.json(promptPacks);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "list", undefined, error, 500);
      }
      return serverErrorResponse(error, "Failed to list prompt packs");
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

      const promptPack = buildCrdResource(
        CRD_KIND,
        name,
        result.workspace.spec.namespace.name,
        resourceName,
        body.spec,
        body.metadata?.labels,
        body.metadata?.annotations
      );

      const created = await createCrd<PromptPack>(result.clientOptions, CRD_PROMPTPACKS, promptPack as unknown as PromptPack);

      auditSuccess(auditCtx, "create", resourceName);
      return NextResponse.json(created, { status: 201 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "create", resourceName, error, 500);
      }
      return serverErrorResponse(error, "Failed to create prompt pack");
    }
  }
);
