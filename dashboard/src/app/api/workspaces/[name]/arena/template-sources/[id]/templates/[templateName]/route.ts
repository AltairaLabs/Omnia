/**
 * API route for a specific template within a template source.
 *
 * GET /api/workspaces/:name/arena/template-sources/:id/templates/:templateName - Get template details
 *
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
  params: Promise<{ name: string; id: string; templateName: string }>;
}

export const GET = withWorkspaceAccess<{ name: string; id: string; templateName: string }>(
  "viewer",
  async (
    _request: NextRequest,
    context: RouteParams,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, id, templateName } = await context.params;
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

      // Find the template in the source's status
      const templates = source.status?.templates || [];
      const template = templates.find((t) => t.name === templateName);

      if (!template) {
        return notFoundResponse(`Template not found: ${templateName}`);
      }

      auditSuccess(auditCtx, "get", `${id}/templates/${templateName}`);
      return NextResponse.json({
        template,
        sourceName: source.metadata.name,
        sourcePhase: source.status?.phase || "Pending",
      });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", `${id}/templates/${templateName}`, error, 500);
      }
      return serverErrorResponse(error, "Failed to get template");
    }
  }
);
