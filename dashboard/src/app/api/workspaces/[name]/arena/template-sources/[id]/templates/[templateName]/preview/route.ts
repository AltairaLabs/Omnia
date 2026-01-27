/**
 * API route for previewing rendered template output.
 *
 * POST /api/workspaces/:name/arena/template-sources/:id/templates/:templateName/preview
 *   - Validates variables
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

type VariableValueType = string | number | boolean;

const CRD_KIND = "ArenaTemplateSource";
const WORKSPACE_CONTENT_PATH = process.env.WORKSPACE_CONTENT_PATH || "/workspace-content";

interface RouteParams {
  params: Promise<{ name: string; id: string; templateName: string }>;
}

/**
 * Simple Go template rendering (subset of functionality).
 * Replaces {{ .variableName }} with variable values.
 */
function renderTemplate(content: string, variables: Record<string, VariableValueType>): string {
  let result = content;

  // Replace {{ .variableName }} patterns
  for (const [name, value] of Object.entries(variables)) {
    const patterns = [
      new RegExp(`\\{\\{\\s*\\.${name}\\s*\\}\\}`, "g"),
      new RegExp(`\\{\\{-\\s*\\.${name}\\s*-?\\}\\}`, "g"),
    ];

    for (const pattern of patterns) {
      result = result.replace(pattern, String(value));
    }
  }

  return result;
}

/**
 * Determine if a file should be rendered based on extension.
 */
function shouldRenderFile(filename: string): boolean {
  const renderExtensions = [".yaml", ".yml", ".json", ".txt", ".md", ".arena.yaml"];
  return renderExtensions.some((ext) => filename.endsWith(ext));
}

/**
 * Read and render template files for preview.
 */
async function previewTemplateFiles(
  sourcePath: string,
  variables: Record<string, VariableValueType>,
  template: TemplateMetadata
): Promise<Array<{ path: string; content: string }>> {
  const renderedFiles: Array<{ path: string; content: string }> = [];
  const files = template.files || [];

  // If no files specified, preview all files except template.yaml
  if (files.length === 0) {
    await previewDirectory(sourcePath, "", variables, true, renderedFiles);
    return renderedFiles;
  }

  for (const fileSpec of files) {
    const srcPath = path.join(sourcePath, fileSpec.path);

    try {
      const stat = await fs.stat(srcPath);

      if (stat.isDirectory()) {
        await previewDirectory(srcPath, fileSpec.path, variables, fileSpec.render !== false, renderedFiles);
      } else {
        const content = await fs.readFile(srcPath, "utf-8");
        const shouldRender = fileSpec.render !== false && shouldRenderFile(fileSpec.path);
        const renderedContent = shouldRender ? renderTemplate(content, variables) : content;
        renderedFiles.push({ path: fileSpec.path, content: renderedContent });
      }
    } catch {
      // Skip files that don't exist
      continue;
    }
  }

  return renderedFiles;
}

/**
 * Preview files in a directory recursively.
 */
async function previewDirectory(
  srcDir: string,
  relativePath: string,
  variables: Record<string, VariableValueType>,
  render: boolean,
  renderedFiles: Array<{ path: string; content: string }>
): Promise<void> {
  const entries = await fs.readdir(srcDir, { withFileTypes: true });

  for (const entry of entries) {
    // Skip template.yaml
    if (entry.name === "template.yaml") continue;

    const srcPath = path.join(srcDir, entry.name);
    const filePath = relativePath ? path.join(relativePath, entry.name) : entry.name;

    if (entry.isDirectory()) {
      await previewDirectory(srcPath, filePath, variables, render, renderedFiles);
    } else {
      const content = await fs.readFile(srcPath, "utf-8");
      const shouldRender = render && shouldRenderFile(entry.name);
      const renderedContent = shouldRender ? renderTemplate(content, variables) : content;
      renderedFiles.push({ path: filePath, content: renderedContent });
    }
  }
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

      // Render template files for preview
      const renderedFiles = await previewTemplateFiles(
        templatePath,
        variablesWithDefaults,
        template
      );

      auditSuccess(auditCtx, "get", `${id}/templates/${templateName}/preview`);

      return NextResponse.json({
        files: renderedFiles,
        errors: validationErrors.length > 0 ? validationErrors : undefined,
      });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", `${id}/templates/${templateName}/preview`, error, 500);
      }
      return handleK8sError(error, "preview template");
    }
  }
);
