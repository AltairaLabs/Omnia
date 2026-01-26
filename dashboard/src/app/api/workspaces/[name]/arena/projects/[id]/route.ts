/**
 * API routes for a single Arena project.
 *
 * GET /api/workspaces/:name/arena/projects/:id - Get project metadata and file tree
 * DELETE /api/workspaces/:name/arena/projects/:id - Delete a project
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
import { getFileType, type ArenaProjectWithTree, type FileTreeNode } from "@/types/arena-project";
import * as fs from "node:fs/promises";
import * as path from "node:path";
import * as yaml from "js-yaml";

// Base path for workspace content (configurable via environment)
const WORKSPACE_CONTENT_BASE =
  process.env.WORKSPACE_CONTENT_PATH || "/workspace-content";

const RESOURCE_TYPE = "ArenaProject";

interface RouteParams {
  params: Promise<{ name: string; id: string }>;
}

interface ProjectConfig {
  name: string;
  description?: string;
  tags?: string[];
  createdAt?: string;
  updatedAt?: string;
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
 * Delete directory recursively
 */
async function deleteRecursive(dirPath: string): Promise<void> {
  const entries = await fs.readdir(dirPath, { withFileTypes: true });

  for (const entry of entries) {
    const fullPath = path.join(dirPath, entry.name);
    if (entry.isDirectory()) {
      await deleteRecursive(fullPath);
    } else {
      await fs.unlink(fullPath);
    }
  }

  await fs.rmdir(dirPath);
}

/**
 * GET /api/workspaces/:name/arena/projects/:id
 *
 * Get project metadata and file tree.
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

      // Read project config
      const configPath = path.join(projectPath, "config.arena.yaml");
      let config: ProjectConfig = { name: projectId };
      let configStats;

      try {
        const content = await fs.readFile(configPath, "utf-8");
        config = yaml.load(content) as ProjectConfig;
        configStats = await fs.stat(configPath);
      } catch {
        // Use defaults if config doesn't exist
        configStats = await fs.stat(projectPath);
      }

      // Build file tree
      const tree = await buildFileTree(projectPath);

      const response: ArenaProjectWithTree = {
        id: projectId,
        name: config.name || projectId,
        description: config.description,
        tags: config.tags,
        createdAt: config.createdAt || configStats.birthtime.toISOString(),
        updatedAt: config.updatedAt || configStats.mtime.toISOString(),
        tree,
      };

      auditSuccess(auditCtx, "get", projectId);
      return NextResponse.json(response);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", projectId, error, 500);
      }
      return serverErrorResponse(error, "Failed to get project");
    }
  }
);

/**
 * DELETE /api/workspaces/:name/arena/projects/:id
 *
 * Delete a project and all its files.
 */
export const DELETE = withWorkspaceAccess<{ name: string; id: string }>(
  "editor",
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

      // Delete project directory recursively
      await deleteRecursive(projectPath);

      auditSuccess(auditCtx, "delete", projectId);
      return new NextResponse(null, { status: 204 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "delete", projectId, error, 500);
      }
      return serverErrorResponse(error, "Failed to delete project");
    }
  }
);
