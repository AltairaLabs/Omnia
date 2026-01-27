/**
 * API route for a specific template within a template source.
 *
 * GET /api/workspaces/:name/arena/template-sources/:id/templates/:templateName - Get template details
 *
 * Templates are read from the _template-index.json file written by the controller.
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import fs from "node:fs/promises";
import path from "node:path";
import { withWorkspaceAccess } from "@/lib/auth/workspace-guard";
import { getCrd } from "@/lib/k8s/crd-operations";
import {
  validateWorkspace,
  handleK8sError,
  notFoundResponse,
  CRD_ARENA_TEMPLATE_SOURCES,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaTemplateSource, TemplateMetadata } from "@/types/arena-template";

const CRD_KIND = "ArenaTemplateSource";
const TEMPLATE_INDEX_DIR = "arena/template-indexes";
const WORKSPACE_CONTENT_BASE =
  process.env.WORKSPACE_CONTENT_PATH || "/workspace-content";

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

      const namespace = result.workspace.spec.namespace.name;

      auditCtx = createAuditContext(
        name,
        namespace,
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

      // Read templates from the index file
      const indexPath = path.join(
        WORKSPACE_CONTENT_BASE,
        name,
        namespace,
        TEMPLATE_INDEX_DIR,
        `${id}.json`
      );

      let templates: TemplateMetadata[] = [];
      try {
        const indexContent = await fs.readFile(indexPath, "utf-8");
        templates = JSON.parse(indexContent);
      } catch {
        // Index file may not exist yet
      }

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
      return handleK8sError(error, "get template");
    }
  }
);
