/**
 * API route for rendering a template and creating a project.
 *
 * POST /api/workspaces/:name/arena/template-sources/:id/templates/:templateName/render
 *   - Validates variables
 *   - Renders template files
 *   - Creates a new Arena project
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
    // Handle both {{ .name }} and {{.name}} formats
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
 * Recursively copy and render template files to project directory.
 */
async function renderTemplateFiles(
  sourcePath: string,
  destPath: string,
  variables: Record<string, VariableValueType>,
  template: TemplateMetadata
): Promise<string[]> {
  const renderedFiles: string[] = [];
  const files = template.files || [];

  // If no files specified, copy everything except template.yaml
  if (files.length === 0) {
    await copyAndRenderDirectory(sourcePath, destPath, variables, true, renderedFiles);
    return renderedFiles;
  }

  for (const fileSpec of files) {
    const srcPath = path.join(sourcePath, fileSpec.path);
    const dstPath = path.join(destPath, fileSpec.path);

    try {
      const stat = await fs.stat(srcPath);

      if (stat.isDirectory()) {
        await copyAndRenderDirectory(srcPath, dstPath, variables, fileSpec.render !== false, renderedFiles);
      } else {
        await copyAndRenderFile(srcPath, dstPath, variables, fileSpec.render !== false);
        renderedFiles.push(fileSpec.path);
      }
    } catch {
      // Skip files that don't exist
      continue;
    }
  }

  return renderedFiles;
}

/**
 * Copy and optionally render a directory recursively.
 */
async function copyAndRenderDirectory(
  srcDir: string,
  dstDir: string,
  variables: Record<string, VariableValueType>,
  render: boolean,
  renderedFiles: string[]
): Promise<void> {
  await fs.mkdir(dstDir, { recursive: true });

  const entries = await fs.readdir(srcDir, { withFileTypes: true });

  for (const entry of entries) {
    // Skip template.yaml
    if (entry.name === "template.yaml") continue;

    const srcPath = path.join(srcDir, entry.name);
    const dstPath = path.join(dstDir, entry.name);

    if (entry.isDirectory()) {
      await copyAndRenderDirectory(srcPath, dstPath, variables, render, renderedFiles);
    } else {
      const shouldRender = render && shouldRenderFile(entry.name);
      await copyAndRenderFile(srcPath, dstPath, variables, shouldRender);
      renderedFiles.push(entry.name);
    }
  }
}

/**
 * Copy and optionally render a single file.
 */
async function copyAndRenderFile(
  srcPath: string,
  dstPath: string,
  variables: Record<string, VariableValueType>,
  render: boolean
): Promise<void> {
  await fs.mkdir(path.dirname(dstPath), { recursive: true });

  const content = await fs.readFile(srcPath, "utf-8");
  const output = render ? renderTemplate(content, variables) : content;
  await fs.writeFile(dstPath, output, "utf-8");
}

/**
 * Determine if a file should be rendered based on extension.
 */
function shouldRenderFile(filename: string): boolean {
  const renderExtensions = [".yaml", ".yml", ".json", ".txt", ".md", ".arena.yaml"];
  return renderExtensions.some((ext) => filename.endsWith(ext));
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

      // Find the template
      const templates = source.status?.templates || [];
      const template = templates.find((t) => t.name === templateName);

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

      // Get workspace info for path construction
      const workspace = await getWorkspace(name);
      if (!workspace) {
        return notFoundResponse(`Workspace not found: ${name}`);
      }

      // Construct paths
      const workspaceNamespace = workspace.spec.namespace.name;
      const templateSourcePath = path.join(
        WORKSPACE_CONTENT_PATH,
        name,
        workspaceNamespace,
        source.status?.artifact?.contentPath || ""
      );
      const templatePath = path.join(templateSourcePath, template.path);

      // Generate project ID and create project directory
      const projectId = generateProjectId();
      const projectsPath = path.join(
        WORKSPACE_CONTENT_PATH,
        name,
        workspaceNamespace,
        "arena",
        "projects"
      );
      const projectPath = path.join(projectsPath, projectId);

      // Render template files to project directory
      const renderedFiles = await renderTemplateFiles(
        templatePath,
        projectPath,
        variablesWithDefaults,
        template
      );

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
        files: renderedFiles,
        template: {
          source: source.metadata.name,
          name: template.name,
        },
      }, { status: 201 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "create", `${id}/templates/${templateName}/render`, error, 500);
      }
      return serverErrorResponse(error, "Failed to render template");
    }
  }
);
