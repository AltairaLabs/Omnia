/**
 * API routes for a specific Arena template source.
 *
 * GET /api/workspaces/:name/arena/template-sources/:id - Get template source details
 * PUT /api/workspaces/:name/arena/template-sources/:id - Update template source
 * DELETE /api/workspaces/:name/arena/template-sources/:id - Delete template source
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess } from "@/lib/auth/workspace-guard";
import { getCrd, updateCrd, deleteCrd } from "@/lib/k8s/crd-operations";
import {
  validateWorkspace,
  serverErrorResponse,
  notFoundResponse,
  CRD_ARENA_TEMPLATE_SOURCES,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaTemplateSource } from "@/types/arena-template";

const CRD_KIND = "ArenaTemplateSource";

interface RouteParams {
  params: Promise<{ name: string; id: string }>;
}

export const GET = withWorkspaceAccess<{ name: string; id: string }>(
  "viewer",
  async (
    _request: NextRequest,
    context: RouteParams,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, id } = await context.params;
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

      const source = await getCrd<ArenaTemplateSource>(
        result.clientOptions,
        CRD_ARENA_TEMPLATE_SOURCES,
        id
      );

      if (!source) {
        return notFoundResponse(`Template source not found: ${id}`);
      }

      auditSuccess(auditCtx, "get", id);
      return NextResponse.json(source);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", id, error, 500);
      }
      return serverErrorResponse(error, "Failed to get template source");
    }
  }
);

export const PUT = withWorkspaceAccess<{ name: string; id: string }>(
  "editor",
  async (
    request: NextRequest,
    context: RouteParams,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, id } = await context.params;
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

      // Get existing source
      const existing = await getCrd<ArenaTemplateSource>(
        result.clientOptions,
        CRD_ARENA_TEMPLATE_SOURCES,
        id
      );

      if (!existing) {
        return notFoundResponse(`Template source not found: ${id}`);
      }

      const body = await request.json();

      // Merge spec updates
      const updated: ArenaTemplateSource = {
        ...existing,
        spec: {
          ...existing.spec,
          ...body.spec,
        },
      };

      const savedSource = await updateCrd<ArenaTemplateSource>(
        result.clientOptions,
        CRD_ARENA_TEMPLATE_SOURCES,
        id,
        updated
      );

      auditSuccess(auditCtx, "update", id);
      return NextResponse.json(savedSource);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "update", id, error, 500);
      }
      return serverErrorResponse(error, "Failed to update template source");
    }
  }
);

export const DELETE = withWorkspaceAccess<{ name: string; id: string }>(
  "editor",
  async (
    _request: NextRequest,
    context: RouteParams,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, id } = await context.params;
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

      await deleteCrd(result.clientOptions, CRD_ARENA_TEMPLATE_SOURCES, id);

      auditSuccess(auditCtx, "delete", id);
      return NextResponse.json({ success: true });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "delete", id, error, 500);
      }
      return serverErrorResponse(error, "Failed to delete template source");
    }
  }
);
