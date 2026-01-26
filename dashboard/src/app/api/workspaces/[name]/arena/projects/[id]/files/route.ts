/**
 * API routes for listing files in an Arena project.
 *
 * GET /api/workspaces/:name/arena/projects/:id/files - List all files in project
 * POST /api/workspaces/:name/arena/projects/:id/files - Create file/folder at root
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess } from "@/lib/auth/workspace-guard";
import {
  validateWorkspace,
  serverErrorResponse,
  notFoundResponse,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import { getFileType, type FileTreeNode, type FileCreateRequest, type FileCreateResponse } from "@/types/arena-project";
import * as fs from "node:fs/promises";
import * as path from "node:path";

// Base path for workspace content (configurable via environment)
const WORKSPACE_CONTENT_BASE =
  process.env.WORKSPACE_CONTENT_PATH || "/workspace-content";

const RESOURCE_TYPE = "ArenaProjectFile";

interface RouteParams {
  params: Promise<{ name: string; id: string }>;
}

/**
 * Get the project directory path
 */
function getProjectPath(workspaceName: string, namespace: string, projectId: string): string {
  return path.join(
    WORKSPACE_CONTENT_BASE,
    workspaceName,
    namespace,
    "arena",
    "projects",
    projectId
  );
}

/**
 * Build file tree recursively from directory
 */
async function buildFileTree(dirPath: string, relativePath: string = ""): Promise<FileTreeNode[]> {
  const entries = await fs.readdir(dirPath, { withFileTypes: true });
  const nodes: FileTreeNode[] = [];

  for (const entry of entries) {
    // Skip hidden files/directories
    if (entry.name.startsWith(".")) continue;

    const entryPath = relativePath ? `${relativePath}/${entry.name}` : entry.name;
    const fullPath = path.join(dirPath, entry.name);
    const stats = await fs.stat(fullPath);

    if (entry.isDirectory()) {
      const children = await buildFileTree(fullPath, entryPath);
      nodes.push({
        name: entry.name,
        path: entryPath,
        isDirectory: true,
        children,
        modifiedAt: stats.mtime.toISOString(),
      });
    } else {
      nodes.push({
        name: entry.name,
        path: entryPath,
        isDirectory: false,
        size: stats.size,
        modifiedAt: stats.mtime.toISOString(),
        type: getFileType(entry.name),
      });
    }
  }

  // Sort: directories first, then alphabetically
  nodes.sort((a, b) => {
    if (a.isDirectory && !b.isDirectory) return -1;
    if (!a.isDirectory && b.isDirectory) return 1;
    return a.name.localeCompare(b.name);
  });

  return nodes;
}

/**
 * Validate filename (no path separators, no .. )
 */
function isValidFilename(name: string): boolean {
  if (!name || name.trim() === "") return false;
  if (name.includes("/") || name.includes("\\")) return false;
  if (name === "." || name === "..") return false;
  if (name.startsWith(".")) return false; // No hidden files
  return true;
}

/**
 * GET /api/workspaces/:name/arena/projects/:id/files
 *
 * List all files in the project as a tree.
 */
export const GET = withWorkspaceAccess<{ name: string; id: string }>(
  "viewer",
  async (
    _request: NextRequest,
    context: RouteParams,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, id: projectId } = await context.params;
    let auditCtx;

    try {
      const result = await validateWorkspace(name, access.role!);
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        name,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        RESOURCE_TYPE
      );

      const projectPath = getProjectPath(name, result.workspace.spec.namespace.name, projectId);

      // Check if project exists
      try {
        await fs.access(projectPath);
      } catch {
        return notFoundResponse(`Project not found: ${projectId}`);
      }

      // Build file tree
      const tree = await buildFileTree(projectPath);

      auditSuccess(auditCtx, "list", projectId);
      return NextResponse.json({ tree });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "list", projectId, error, 500);
      }
      return serverErrorResponse(error, "Failed to list project files");
    }
  }
);

/**
 * POST /api/workspaces/:name/arena/projects/:id/files
 *
 * Create a file or directory at the project root.
 */
export const POST = withWorkspaceAccess<{ name: string; id: string }>(
  "editor",
  async (
    request: NextRequest,
    context: RouteParams,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, id: projectId } = await context.params;
    let auditCtx;

    try {
      const result = await validateWorkspace(name, access.role!);
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        name,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        RESOURCE_TYPE
      );

      const body = (await request.json()) as FileCreateRequest;

      if (!isValidFilename(body.name)) {
        return NextResponse.json(
          { error: "Bad Request", message: "Invalid filename" },
          { status: 400 }
        );
      }

      const projectPath = getProjectPath(name, result.workspace.spec.namespace.name, projectId);

      // Check if project exists
      try {
        await fs.access(projectPath);
      } catch {
        return notFoundResponse(`Project not found: ${projectId}`);
      }

      const targetPath = path.join(projectPath, body.name);

      // Check if already exists
      try {
        await fs.access(targetPath);
        return NextResponse.json(
          { error: "Conflict", message: `File or directory already exists: ${body.name}` },
          { status: 409 }
        );
      } catch {
        // Good, doesn't exist
      }

      const now = new Date().toISOString();

      if (body.isDirectory) {
        await fs.mkdir(targetPath, { recursive: true });
      } else {
        const content = body.content ?? "";
        await fs.writeFile(targetPath, content, "utf-8");
      }

      const stats = await fs.stat(targetPath);

      const response: FileCreateResponse = {
        path: body.name,
        name: body.name,
        isDirectory: body.isDirectory,
        size: body.isDirectory ? undefined : stats.size,
        modifiedAt: now,
      };

      auditSuccess(auditCtx, "create", `${projectId}/${body.name}`);
      return NextResponse.json(response, { status: 201 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "create", projectId, error, 500);
      }
      return serverErrorResponse(error, "Failed to create file");
    }
  }
);
