/**
 * API route for previewing rendered template output.
 *
 * POST /api/workspaces/:name/arena/template-sources/:id/templates/:templateName/preview
 *   - Validates variables
 *   - Calls arena-controller to render template using PromptKit
 *   - Returns rendered files without creating a project
 *
 * Templates are read from the index file written by the controller.
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
  getWorkspace,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import {
  validateVariables,
  getDefaultVariableValues,
  type ArenaTemplateSource,
  type TemplateMetadata,
} from "@/types/arena-template";
import * as fs from "fs/promises";
import * as path from "path";

const TEMPLATE_INDEX_DIR = "arena/template-indexes";

const CRD_KIND = "ArenaTemplateSource";
const WORKSPACE_CONTENT_PATH = process.env.WORKSPACE_CONTENT_PATH || "/workspace-content";
// Service domain for K8s cluster DNS
const SERVICE_DOMAIN = process.env.SERVICE_DOMAIN || "svc.cluster.local";
// Arena controller API URL for template rendering
const ARENA_CONTROLLER_URL = process.env.ARENA_CONTROLLER_URL ||
  `http://omnia-arena-controller.omnia-system.${SERVICE_DOMAIN}:8082`;

interface RouteParams {
  params: Promise<{ name: string; id: string; templateName: string }>;
}

/**
 * Preview a template using the arena-controller API (which uses PromptKit).
 */
async function previewTemplateViaController(
  templatePath: string,
  projectName: string,
  variables: Record<string, unknown>
): Promise<{ files: Array<{ path: string; content: string }>; errors?: string[] }> {
  const response = await fetch(`${ARENA_CONTROLLER_URL}/api/preview-template`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      templatePath,
      projectName,
      variables,
    }),
  });

  if (!response.ok) {
    const text = await response.text();
    throw new Error(`Arena controller returned ${response.status}: ${text}`);
  }

  return response.json();
}

export const POST = withWorkspaceAccess<{ name: string; id: string; templateName: string }>(
  "viewer", // Preview only requires viewer access
  async (
    request: NextRequest,
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

      // Get the template source
      const source = await getCrd<ArenaTemplateSource>(
        result.clientOptions,
        CRD_ARENA_TEMPLATE_SOURCES,
        id
      );

      if (!source) {
        return notFoundResponse(`Template source not found: ${id}`);
      }

      if (source.status?.phase !== "Ready") {
        return NextResponse.json(
          { error: "Template source is not ready" },
          { status: 400 }
        );
      }

      // Get workspace info for path construction
      const workspace = await getWorkspace(name);
      if (!workspace) {
        return notFoundResponse(`Workspace not found: ${name}`);
      }
      const workspaceNamespace = workspace.spec.namespace.name;

      // Read templates from the index file
      const indexPath = path.join(
        WORKSPACE_CONTENT_PATH,
        name,
        workspaceNamespace,
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

      const template = templates.find((t: TemplateMetadata) => t.name === templateName);

      if (!template) {
        return notFoundResponse(`Template not found: ${templateName}`);
      }

      // Parse request body
      const body = await request.json();
      const inputVariables = body.variables || {};

      // Apply defaults
      const variablesWithDefaults = {
        ...getDefaultVariableValues(template.variables || []),
        ...inputVariables,
        projectName: body.projectName || "preview-project",
      };

      // Validate variables (return errors but don't block preview)
      const validationErrors = validateVariables(
        template.variables || [],
        variablesWithDefaults
      );

      // Construct template path
      const templateSourcePath = path.join(
        WORKSPACE_CONTENT_PATH,
        name,
        workspaceNamespace,
        source.status?.artifact?.contentPath || ""
      );
      const templatePath = path.join(templateSourcePath, template.path);

      // Call arena-controller to preview the template using PromptKit
      const previewResult = await previewTemplateViaController(
        templatePath,
        body.projectName || "preview-project",
        variablesWithDefaults
      );

      auditSuccess(auditCtx, "get", `${id}/templates/${templateName}/preview`);

      return NextResponse.json({
        files: previewResult.files,
        errors: validationErrors.length > 0 ? validationErrors : previewResult.errors,
      });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", `${id}/templates/${templateName}/preview`, error, 500);
      }
      return handleK8sError(error, "preview template");
    }
  }
);
