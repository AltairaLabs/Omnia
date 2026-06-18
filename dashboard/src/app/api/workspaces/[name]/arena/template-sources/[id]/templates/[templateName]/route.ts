/**
 * API route for a specific template within a template source.
 *
 * GET /api/workspaces/:name/arena/template-sources/:id/templates/:templateName - Get template details
 *
 * The template index JSON written by the controller is read through the
 * operator's authenticated content API (the dashboard no longer mounts the NFS
 * workspace-content volume directly — see #1462). The operator prepends
 * <workspace>/<namespace>, so the path here is relative to the workspace root.
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
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
import { getContent, isContentFile } from "@/lib/data/content-api-service";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaTemplateSource, TemplateMetadata } from "@/types/arena-template";

const CRD_KIND = "ArenaTemplateSource";
const TEMPLATE_INDEX_DIR = "arena/template-indexes";

interface RouteParams {
  params: Promise<{ name: string; id: string; templateName: string }>;
}

/**
 * Read and parse the controller-written template index for a source, returning
 * an empty list when the index does not exist yet or is unreadable. The path is
 * workspace-relative; the operator prepends <workspace>/<namespace>.
 */
async function readTemplateIndex(
  workspace: string,
  user: User,
  sourceId: string,
): Promise<TemplateMetadata[]> {
  const relpath = `${TEMPLATE_INDEX_DIR}/${sourceId}.json`;
  try {
    const node = await getContent(workspace, user, relpath);
    if (!isContentFile(node)) {
      return [];
    }
    return JSON.parse(node.content);
  } catch {
    // Index file may not exist yet.
    return [];
  }
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

      // Read templates from the controller-written index.
      const templates = await readTemplateIndex(name, user, id);

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
