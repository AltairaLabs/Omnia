/**
 * API routes for Arena template sources (ArenaTemplateSource CRDs).
 *
 * GET /api/workspaces/:name/arena/template-sources - List template sources
 * POST /api/workspaces/:name/arena/template-sources - Create a new template source
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
  CRD_ARENA_TEMPLATE_SOURCES,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaTemplateSource } from "@/types/arena-template";

const CRD_KIND = "ArenaTemplateSource";

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

      const sources = await listCrd<ArenaTemplateSource>(
        result.clientOptions,
        CRD_ARENA_TEMPLATE_SOURCES
      );

      auditSuccess(auditCtx, "list", undefined, { count: sources.length });
      return NextResponse.json(sources);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "list", undefined, error, 500);
      }
      return serverErrorResponse(error, "Failed to list template sources");
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

      const source = buildCrdResource(
        CRD_KIND,
        name,
        result.workspace.spec.namespace.name,
        resourceName,
        body.spec,
        body.metadata?.labels,
        body.metadata?.annotations
      );

      const created = await createCrd<ArenaTemplateSource>(
        result.clientOptions,
        CRD_ARENA_TEMPLATE_SOURCES,
        source as unknown as ArenaTemplateSource
      );

      auditSuccess(auditCtx, "create", resourceName);
      return NextResponse.json(created, { status: 201 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "create", resourceName, error, 500);
      }
      return serverErrorResponse(error, "Failed to create template source");
    }
  }
);
