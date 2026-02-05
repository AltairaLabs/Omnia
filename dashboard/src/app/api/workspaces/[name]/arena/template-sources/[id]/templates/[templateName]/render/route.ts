/**
 * API route for rendering a template and creating a project.
 *
 * POST /api/workspaces/:name/arena/template-sources/:id/templates/:templateName/render
 *   - Validates variables
 *   - Calls arena-controller to render template using PromptKit
 *   - Creates a new Arena project
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
  type TemplateRenderInput,
  type TemplateMetadata,
} from "@/types/arena-template";
import crypto from "crypto";
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
 * Render a template using the arena-controller API (which uses PromptKit).
 */
async function renderTemplateViaController(
  templatePath: string,
  outputPath: string,
  projectName: string,
  variables: Record<string, unknown>
): Promise<{ success: boolean; filesCreated: string[]; errors: string[]; warnings: string[] }> {
  const response = await fetch(`${ARENA_CONTROLLER_URL}/api/render-template`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      templatePath,
      outputPath,
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

/**
 * Generate a unique project ID using cryptographically secure randomness.
 */
function generateProjectId(): string {
  return crypto.randomUUID();
}

export const POST = withWorkspaceAccess<{ name: string; id: string; templateName: string }>(
  "editor",
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
      const body: TemplateRenderInput = await request.json();

      // Validate required fields
      if (!body.projectName) {
        return NextResponse.json(
          { error: "projectName is required" },
          { status: 400 }
        );
      }

      // Apply defaults and validate variables
      const variablesWithDefaults = {
        ...getDefaultVariableValues(template.variables || []),
        ...body.variables,
        projectName: body.projectName, // Always include projectName as a variable
      };

      const validationErrors = validateVariables(
        template.variables || [],
        variablesWithDefaults
      );

      if (validationErrors.length > 0) {
        return NextResponse.json(
          {
            error: "Variable validation failed",
            errors: validationErrors,
          },
          { status: 400 }
        );
      }

      // Construct paths
      const templateSourcePath = path.join(
        WORKSPACE_CONTENT_PATH,
        name,
        workspaceNamespace,
        source.status?.artifact?.contentPath || ""
      );
      const templatePath = path.join(templateSourcePath, template.path);

      // Generate project ID and create project directory path
      const projectId = generateProjectId();
      const projectsPath = path.join(
        WORKSPACE_CONTENT_PATH,
        name,
        workspaceNamespace,
        "arena",
        "projects"
      );
      const projectPath = path.join(projectsPath, projectId);

      // Call arena-controller to render the template using PromptKit
      const renderResult = await renderTemplateViaController(
        templatePath,
        projectsPath, // PromptKit will create projectName subdirectory
        projectId,    // Use projectId as the project name for the directory
        variablesWithDefaults
      );

      if (!renderResult.success) {
        return NextResponse.json(
          {
            error: "Template rendering failed",
            errors: renderResult.errors,
            warnings: renderResult.warnings,
          },
          { status: 500 }
        );
      }

      // Create project metadata file
      const projectMeta = {
        id: projectId,
        name: body.projectName,
        description: body.projectDescription || template.description,
        tags: body.projectTags || [],
        template: {
          source: source.metadata.name,
          name: template.name,
          version: template.version,
        },
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
        createdBy: user.email || user.username || "unknown",
      };

      await fs.writeFile(
        path.join(projectPath, ".project.json"),
        JSON.stringify(projectMeta, null, 2),
        "utf-8"
      );

      auditSuccess(auditCtx, "create", `projects/${projectId}`, {
        templateSource: id,
        templateName,
        projectName: body.projectName,
      });

      return NextResponse.json({
        projectId,
        projectName: body.projectName,
        files: renderResult.filesCreated,
        warnings: renderResult.warnings,
        template: {
          source: source.metadata.name,
          name: template.name,
        },
      }, { status: 201 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "create", `${id}/templates/${templateName}/render`, error, 500);
      }
      return handleK8sError(error, "render template");
    }
  }
);
