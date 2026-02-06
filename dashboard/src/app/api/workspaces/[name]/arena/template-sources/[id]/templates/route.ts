/**
 * API route for listing templates within a template source.
 *
 * GET /api/workspaces/:name/arena/template-sources/:id/templates - List templates
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
import type { ArenaTemplateSource } from "@/types/arena-template";

const CRD_KIND = "ArenaTemplateSource";
const TEMPLATE_INDEX_DIR = "arena/template-indexes";
const WORKSPACE_CONTENT_BASE =
  process.env.WORKSPACE_CONTENT_PATH || "/workspace-content";

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

      const sourcePhase = source.status?.phase || "Pending";

      // If source is not ready, return empty templates
      if (sourcePhase !== "Ready") {
        auditSuccess(auditCtx, "list", `${id}/templates`, { count: 0 });
        return NextResponse.json({
          templates: [],
          sourcePhase,
          sourceName: source.metadata.name,
        });
      }

      // Read templates from the index file at:
      // {workspace}/{namespace}/arena/template-indexes/{source}.json
      const indexPath = path.join(
        WORKSPACE_CONTENT_BASE,
        name,
        namespace,
        TEMPLATE_INDEX_DIR,
        `${id}.json`
      );

      let templates: unknown[] = [];
      try {
        const indexContent = await fs.readFile(indexPath, "utf-8");
        templates = JSON.parse(indexContent);
      } catch (err) {
        // Index file may not exist yet or be unreadable
        console.warn(`Failed to read template index at ${indexPath}:`, err);
      }

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
      return handleK8sError(error, "list templates");
    }
  }
);
