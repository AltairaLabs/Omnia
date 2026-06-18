/**
 * API routes for a single Arena project.
 *
 * GET /api/workspaces/:name/arena/projects/:id - Get project metadata and file tree
 * DELETE /api/workspaces/:name/arena/projects/:id - Delete a project
 *
 * Content is served by the operator's authenticated content API (the dashboard
 * no longer mounts the NFS workspace-content volume directly — see #1462).
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess } from "@/lib/auth/workspace-guard";
import {
  validateWorkspace,
  handleK8sError,
  notFoundResponse,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import { getFileType, type ArenaProjectWithTree, type FileTreeNode, type ProviderBinding } from "@/types/arena-project";
import { extractBindingAnnotations } from "@/lib/arena/provider-binding";
import {
  ContentApiError,
  getContent,
  isContentFile,
  deleteContent,
} from "@/lib/data/content-api-service";
import { listContentTree, type ContentTreeNode } from "@/lib/data/content-tree";
import { contentErrorResponse } from "@/lib/data/content-api-response";
import * as yaml from "js-yaml";

const RESOURCE_TYPE = "ArenaProject";
const PROJECT_CONFIG_FILE = "config.arena.yaml";

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

/** The content relpath for a project's directory. */
function projectRelPath(projectId: string): string {
  return `arena/projects/${projectId}`;
}

/**
 * Read provider binding annotations from a file, if it is a provider YAML.
 */
async function readProviderBinding(
  workspace: string,
  user: User,
  relpath: string,
  fileType: string
): Promise<ProviderBinding | undefined> {
  if (fileType !== "provider") return undefined;
  try {
    const node = await getContent(workspace, user, relpath);
    if (!isContentFile(node)) return undefined;
    return extractBindingAnnotations(node.content) ?? undefined;
  } catch {
    return undefined;
  }
}

/**
 * Map content-tree nodes (operator API) onto the arena FileTreeNode shape,
 * enriching provider files with their binding annotations and sorting
 * directories first, then alphabetically.
 */
async function toFileTree(
  workspace: string,
  user: User,
  baseRelPath: string,
  nodes: ContentTreeNode[]
): Promise<FileTreeNode[]> {
  const result: FileTreeNode[] = [];

  for (const node of nodes) {
    const relpath = `${baseRelPath}/${node.path}`;
    if (node.isDirectory) {
      result.push({
        name: node.name,
        path: node.path,
        isDirectory: true,
        modifiedAt: node.modifiedAt,
        children: await toFileTree(workspace, user, baseRelPath, node.children ?? []),
      });
    } else {
      const fileType = getFileType(node.name);
      result.push({
        name: node.name,
        path: node.path,
        isDirectory: false,
        size: node.size,
        modifiedAt: node.modifiedAt,
        type: fileType,
        providerBinding: await readProviderBinding(workspace, user, relpath, fileType),
      });
    }
  }

  // Sort: directories first, then alphabetically.
  result.sort((a, b) => {
    if (a.isDirectory && !b.isDirectory) return -1;
    if (!a.isDirectory && b.isDirectory) return 1;
    return a.name.localeCompare(b.name);
  });

  return result;
}

/**
 * Read project config metadata via the content API. Returns defaults (project
 * id as name) when the config file is absent or invalid.
 */
async function readProjectMeta(
  workspace: string,
  user: User,
  projectId: string
): Promise<ProjectConfig & { modifiedAt: string }> {
  const configPath = `${projectRelPath(projectId)}/${PROJECT_CONFIG_FILE}`;
  try {
    const node = await getContent(workspace, user, configPath);
    if (isContentFile(node)) {
      const config = yaml.load(node.content) as ProjectConfig;
      return { ...config, modifiedAt: node.modifiedAt };
    }
  } catch (error) {
    if (!(error instanceof ContentApiError && error.status === 404)) {
      throw error;
    }
  }
  return { name: projectId, modifiedAt: "" };
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

      const basePath = projectRelPath(projectId);

      // Check if project exists.
      try {
        await getContent(name, user, basePath);
      } catch (error) {
        if (error instanceof ContentApiError && error.status === 404) {
          return notFoundResponse(`Project not found: ${projectId}`);
        }
        throw error;
      }

      const config = await readProjectMeta(name, user, projectId);
      const treeNodes = await listContentTree(name, user, basePath, { skipHidden: true });
      const tree = await toFileTree(name, user, basePath, treeNodes);

      const response: ArenaProjectWithTree = {
        id: projectId,
        name: config.name || projectId,
        description: config.description,
        tags: config.tags,
        createdAt: config.createdAt || config.modifiedAt,
        updatedAt: config.updatedAt || config.modifiedAt,
        tree,
      };

      auditSuccess(auditCtx, "get", projectId);
      return NextResponse.json(response);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", projectId, error, 500);
      }
      if (error instanceof ContentApiError) {
        return contentErrorResponse(error, "Failed to get project");
      }
      return handleK8sError(error, "get project");
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

      const basePath = projectRelPath(projectId);

      // Check if project exists.
      try {
        await getContent(name, user, basePath);
      } catch (error) {
        if (error instanceof ContentApiError && error.status === 404) {
          return notFoundResponse(`Project not found: ${projectId}`);
        }
        throw error;
      }

      // Delete project directory (operator deletes recursively).
      await deleteContent(name, user, basePath);

      auditSuccess(auditCtx, "delete", projectId);
      return new NextResponse(null, { status: 204 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "delete", projectId, error, 500);
      }
      if (error instanceof ContentApiError) {
        return contentErrorResponse(error, "Failed to delete project");
      }
      return handleK8sError(error, "delete project");
    }
  }
);
