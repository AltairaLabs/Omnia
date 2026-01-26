/**
 * API route for listing templates within a template source.
 *
 * GET /api/workspaces/:name/arena/template-sources/:id/templates - List templates
 *
 * Templates are discovered from the source status, not fetched separately.
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess } from "@/lib/auth/workspace-guard";
import { getCrd } from "@/lib/k8s/crd-operations";
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

      // Templates are stored in the status after discovery
      const templates = source.status?.templates || [];
      const sourcePhase = source.status?.phase || "Pending";

      auditSuccess(auditCtx, "list", `${id}/templates`, { count: templates.length });
      return NextResponse.json({
        templates,
        sourcePhase,
        sourceName: source.metadata.name,
      });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "list", `${id}/templates`, error, 500);
      }
      return serverErrorResponse(error, "Failed to list templates");
    }
  }
);
