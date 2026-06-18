/**
 * API routes for listing files in an Arena project.
 *
 * GET /api/workspaces/:name/arena/projects/:id/files - List all files in project
 * POST /api/workspaces/:name/arena/projects/:id/files - Create file/folder at root
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
import { getFileType, type FileTreeNode, type FileCreateRequest, type FileCreateResponse, type ProviderBinding } from "@/types/arena-project";
import { extractBindingAnnotations } from "@/lib/arena/provider-binding";
import {
  ContentApiError,
  getContent,
  isContentFile,
  makeContentDir,
  writeContentFile,
} from "@/lib/data/content-api-service";
import { listContentTree, type ContentTreeNode } from "@/lib/data/content-tree";
import { contentErrorResponse } from "@/lib/data/content-api-response";

const RESOURCE_TYPE = "ArenaProjectFile";

interface RouteParams {
  params: Promise<{ name: string; id: string }>;
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
 * Validate filename (no path separators, no .. )
 */
function isValidFilename(name: string): boolean {
  if (!name || name.trim() === "") return false;
  if (name.includes("/") || name.includes("\\")) return false;
  if (name === "." || name === "..") return false;
  if (name.startsWith(".")) return false; // No hidden files
  return true;
}

/** True when the thrown error is a content-API 404. */
function isNotFound(error: unknown): boolean {
  return error instanceof ContentApiError && error.status === 404;
}

/**
 * Return a notFound response when the project does not exist, or null when it
 * does. Re-throws other content-API errors.
 */
async function checkProjectExists(
  workspace: string,
  user: User,
  basePath: string,
  projectId: string
): Promise<NextResponse | null> {
  try {
    await getContent(workspace, user, basePath);
    return null;
  } catch (error) {
    if (isNotFound(error)) {
      return notFoundResponse(`Project not found: ${projectId}`);
    }
    throw error;
  }
}

/**
 * Return a 409 response when the target already exists, or null when it does
 * not. Re-throws other content-API errors.
 */
async function conflictIfExists(
  workspace: string,
  user: User,
  targetRelPath: string,
  fileName: string
): Promise<NextResponse | null> {
  try {
    await getContent(workspace, user, targetRelPath);
    return NextResponse.json(
      { error: "Conflict", message: `File or directory already exists: ${fileName}` },
      { status: 409 }
    );
  } catch (error) {
    if (isNotFound(error)) {
      return null;
    }
    throw error;
  }
}

/** Create a file or directory, returning the resulting size (undefined for dirs). */
async function createNode(
  workspace: string,
  user: User,
  targetRelPath: string,
  body: FileCreateRequest
): Promise<number | undefined> {
  if (body.isDirectory) {
    await makeContentDir(workspace, user, targetRelPath);
    return undefined;
  }
  const writeResult = await writeContentFile(workspace, user, targetRelPath, body.content ?? "");
  return writeResult.size;
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

      const basePath = projectRelPath(projectId);

      const missing = await checkProjectExists(name, user, basePath, projectId);
      if (missing) return missing;

      const treeNodes = await listContentTree(name, user, basePath, { skipHidden: true });
      const tree = await toFileTree(name, user, basePath, treeNodes);

      auditSuccess(auditCtx, "list", projectId);
      return NextResponse.json({ tree });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "list", projectId, error, 500);
      }
      if (error instanceof ContentApiError) {
        return contentErrorResponse(error, "Failed to list project files");
      }
      return handleK8sError(error, "list project files");
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

      const basePath = projectRelPath(projectId);

      const missing = await checkProjectExists(name, user, basePath, projectId);
      if (missing) return missing;

      const targetRelPath = `${basePath}/${body.name}`;

      const conflict = await conflictIfExists(name, user, targetRelPath, body.name);
      if (conflict) return conflict;

      const now = new Date().toISOString();
      const size = await createNode(name, user, targetRelPath, body);

      const response: FileCreateResponse = {
        path: body.name,
        name: body.name,
        isDirectory: body.isDirectory,
        size,
        modifiedAt: now,
      };

      auditSuccess(auditCtx, "create", `${projectId}/${body.name}`);
      return NextResponse.json(response, { status: 201 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "create", projectId, error, 500);
      }
      if (error instanceof ContentApiError) {
        return contentErrorResponse(error, "Failed to create file");
      }
      return handleK8sError(error, "create file");
    }
  }
);
