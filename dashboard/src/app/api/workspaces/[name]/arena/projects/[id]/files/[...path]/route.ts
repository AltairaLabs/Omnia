/**
 * API routes for individual file operations in an Arena project.
 *
 * GET /api/workspaces/:name/arena/projects/:id/files/:path - Get file content
 * PUT /api/workspaces/:name/arena/projects/:id/files/:path - Update file content
 * POST /api/workspaces/:name/arena/projects/:id/files/:path - Create file/folder
 * DELETE /api/workspaces/:name/arena/projects/:id/files/:path - Delete file/folder
 *
 * Content is served by the operator's authenticated content API (the dashboard
 * no longer mounts the NFS workspace-content volume directly — see #1462).
 * Path-confinement, max-size and text/binary encoding are operator-side and
 * surface here as pass-through statuses (400/413).
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
import type {
  FileContentResponse,
  FileUpdateRequest,
  FileUpdateResponse,
  FileCreateRequest,
  FileCreateResponse,
  FileRenameRequest,
} from "@/types/arena-project";
import {
  ContentApiError,
  getContent,
  isContentListing,
  makeContentDir,
  moveContent,
  writeContentFile,
  deleteContent,
} from "@/lib/data/content-api-service";
import { contentErrorResponse } from "@/lib/data/content-api-response";

const RESOURCE_TYPE = "ArenaProjectFile";
const BAD_REQUEST = "Bad Request";
const PROJECT_CONFIG_FILE = "config.arena.yaml";

interface RouteParams {
  params: Promise<{ name: string; id: string; path: string[] }>;
}

/** The content relpath for a project's directory. */
function projectRelPath(projectId: string): string {
  return `arena/projects/${projectId}`;
}

/**
 * Validate filename (no path separators, no .. )
 */
function isValidFilename(name: string): boolean {
  if (!name || name.trim() === "") return false;
  if (name.includes("/") || name.includes("\\")) return false;
  if (name === "." || name === "..") return false;
  if (name.startsWith(".")) return false;
  return true;
}

/** True when the thrown error is a content-API 404. */
function isNotFound(error: unknown): boolean {
  return error instanceof ContentApiError && error.status === 404;
}

/**
 * Verify the parent path exists and is a directory. Returns an error response
 * to send back, or null when the parent is a usable directory.
 */
async function checkParentDirectory(
  workspace: string,
  user: User,
  parentRelPath: string,
  parentPath: string
): Promise<NextResponse | null> {
  try {
    const parent = await getContent(workspace, user, parentRelPath);
    if (!isContentListing(parent)) {
      return NextResponse.json(
        { error: BAD_REQUEST, message: "Parent path is not a directory" },
        { status: 400 }
      );
    }
    return null;
  } catch (error) {
    if (isNotFound(error)) {
      return notFoundResponse(`Parent directory not found: ${parentPath}`);
    }
    throw error;
  }
}

/**
 * Return a 409 response when the target already exists, or null when it does
 * not (a 404 from the content API). Re-throws other errors.
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
 * GET /api/workspaces/:name/arena/projects/:id/files/:path
 *
 * Get file content. Returns the file content with encoding info.
 */
export const GET = withWorkspaceAccess<{ name: string; id: string; path: string[] }>(
  "viewer",
  async (
    _request: NextRequest,
    context: RouteParams,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, id: projectId, path: pathSegments } = await context.params;
    const filePath = pathSegments.join("/");
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

      const relpath = `${projectRelPath(projectId)}/${filePath}`;

      let node;
      try {
        node = await getContent(name, user, relpath);
      } catch (error) {
        if (isNotFound(error)) {
          return notFoundResponse(`File not found: ${filePath}`);
        }
        throw error;
      }

      if (isContentListing(node)) {
        return NextResponse.json(
          { error: BAD_REQUEST, message: "Cannot get content of a directory" },
          { status: 400 }
        );
      }

      const response: FileContentResponse = {
        path: filePath,
        content: node.content,
        size: node.size,
        modifiedAt: node.modifiedAt,
        encoding: node.encoding,
      };

      auditSuccess(auditCtx, "get", `${projectId}/${filePath}`);
      return NextResponse.json(response);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", `${projectId}/${filePath}`, error, 500);
      }
      if (error instanceof ContentApiError) {
        return contentErrorResponse(error, "Failed to get file content");
      }
      return handleK8sError(error, "get file content");
    }
  }
);

/**
 * PUT /api/workspaces/:name/arena/projects/:id/files/:path
 *
 * Update file content.
 */
export const PUT = withWorkspaceAccess<{ name: string; id: string; path: string[] }>(
  "editor",
  async (
    request: NextRequest,
    context: RouteParams,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, id: projectId, path: pathSegments } = await context.params;
    const filePath = pathSegments.join("/");
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

      const body = (await request.json()) as FileUpdateRequest;

      if (typeof body.content !== "string") {
        return NextResponse.json(
          { error: BAD_REQUEST, message: "Content is required" },
          { status: 400 }
        );
      }

      const relpath = `${projectRelPath(projectId)}/${filePath}`;

      // The file must already exist for a PUT, and must not be a directory.
      let existing;
      try {
        existing = await getContent(name, user, relpath);
      } catch (error) {
        if (isNotFound(error)) {
          return notFoundResponse(`File not found: ${filePath}`);
        }
        throw error;
      }
      if (isContentListing(existing)) {
        return NextResponse.json(
          { error: BAD_REQUEST, message: "Cannot update content of a directory" },
          { status: 400 }
        );
      }

      const writeResult = await writeContentFile(name, user, relpath, body.content);

      const response: FileUpdateResponse = {
        path: filePath,
        size: writeResult.size,
        modifiedAt: writeResult.modifiedAt,
      };

      auditSuccess(auditCtx, "update", `${projectId}/${filePath}`);
      return NextResponse.json(response);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "update", `${projectId}/${filePath}`, error, 500);
      }
      if (error instanceof ContentApiError) {
        return contentErrorResponse(error, "Failed to update file");
      }
      return handleK8sError(error, "update file");
    }
  }
);

/**
 * POST /api/workspaces/:name/arena/projects/:id/files/:path
 *
 * Create a new file or directory at the specified path.
 */
export const POST = withWorkspaceAccess<{ name: string; id: string; path: string[] }>(
  "editor",
  async (
    request: NextRequest,
    context: RouteParams,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, id: projectId, path: pathSegments } = await context.params;
    const parentPath = pathSegments.join("/");
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
          { error: BAD_REQUEST, message: "Invalid filename" },
          { status: 400 }
        );
      }

      const basePath = projectRelPath(projectId);
      const parentRelPath = `${basePath}/${parentPath}`;
      const targetRelPath = `${parentRelPath}/${body.name}`;
      const targetRelativePath = `${parentPath}/${body.name}`;

      const parentError = await checkParentDirectory(name, user, parentRelPath, parentPath);
      if (parentError) return parentError;

      const conflict = await conflictIfExists(name, user, targetRelPath, body.name);
      if (conflict) return conflict;

      const now = new Date().toISOString();
      const size = await createNode(name, user, targetRelPath, body);

      const response: FileCreateResponse = {
        path: targetRelativePath,
        name: body.name,
        isDirectory: body.isDirectory,
        size,
        modifiedAt: now,
      };

      auditSuccess(auditCtx, "create", `${projectId}/${targetRelativePath}`);
      return NextResponse.json(response, { status: 201 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "create", `${projectId}/${parentPath}`, error, 500);
      }
      if (error instanceof ContentApiError) {
        return contentErrorResponse(error, "Failed to create file");
      }
      return handleK8sError(error, "create file");
    }
  }
);

/** Compute the project-relative destination for a rename within the same directory. */
function siblingPath(filePath: string, newName: string): string {
  const segs = filePath.split("/");
  segs.pop();
  segs.push(newName);
  return segs.join("/");
}

function badRequest(message: string): NextResponse {
  return NextResponse.json({ error: BAD_REQUEST, message }, { status: 400 });
}

/**
 * Resolve the project-relative destination for a rename (`newName`) or move
 * (`destDir`, keeping the basename). Returns the destination path or an error
 * response to send back.
 */
function resolveMoveTarget(
  filePath: string,
  body: FileRenameRequest
): { destRelativePath: string } | { error: NextResponse } {
  if (typeof body.newName === "string") {
    if (!isValidFilename(body.newName)) {
      return { error: badRequest("Invalid filename") };
    }
    return { destRelativePath: siblingPath(filePath, body.newName) };
  }

  if (typeof body.destDir === "string") {
    // Refuse to move a directory into itself or one of its own descendants.
    if (body.destDir === filePath || body.destDir.startsWith(`${filePath}/`)) {
      return { error: badRequest("Cannot move an item into itself") };
    }
    const basename = filePath.split("/").pop() ?? filePath;
    const destRelativePath = body.destDir ? `${body.destDir}/${basename}` : basename;
    return { destRelativePath };
  }

  return { error: badRequest("newName or destDir is required") };
}

/**
 * PATCH /api/workspaces/:name/arena/projects/:id/files/:path
 *
 * Rename a file or directory in place (`newName`), or move it into another
 * directory keeping its name (`destDir`).
 */
export const PATCH = withWorkspaceAccess<{ name: string; id: string; path: string[] }>(
  "editor",
  async (
    request: NextRequest,
    context: RouteParams,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, id: projectId, path: pathSegments } = await context.params;
    const filePath = pathSegments.join("/");
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

      const body = (await request.json()) as FileRenameRequest;

      if (filePath === PROJECT_CONFIG_FILE) {
        return badRequest("Cannot move project config file");
      }

      const resolved = resolveMoveTarget(filePath, body);
      if ("error" in resolved) return resolved.error;
      const { destRelativePath } = resolved;

      const basePath = projectRelPath(projectId);
      const srcRelPath = `${basePath}/${filePath}`;
      const destRelPath = `${basePath}/${destRelativePath}`;

      // The source must exist; capture its type for the response.
      let existing;
      try {
        existing = await getContent(name, user, srcRelPath);
      } catch (error) {
        if (isNotFound(error)) {
          return notFoundResponse(`File not found: ${filePath}`);
        }
        throw error;
      }
      const isDirectory = isContentListing(existing);
      const destName = destRelativePath.split("/").pop() ?? destRelativePath;

      const conflict = await conflictIfExists(name, user, destRelPath, destName);
      if (conflict) return conflict;

      const moveResult = await moveContent(name, user, srcRelPath, destRelPath);

      const response: FileCreateResponse = {
        path: destRelativePath,
        name: destName,
        isDirectory,
        size: isDirectory ? undefined : moveResult.size,
        modifiedAt: moveResult.modifiedAt,
      };

      auditSuccess(auditCtx, "patch", `${projectId}/${filePath} -> ${destRelativePath}`);
      return NextResponse.json(response);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "patch", `${projectId}/${filePath}`, error, 500);
      }
      if (error instanceof ContentApiError) {
        return contentErrorResponse(error, "Failed to rename file");
      }
      return handleK8sError(error, "rename file");
    }
  }
);

/**
 * DELETE /api/workspaces/:name/arena/projects/:id/files/:path
 *
 * Delete a file or directory.
 */
export const DELETE = withWorkspaceAccess<{ name: string; id: string; path: string[] }>(
  "editor",
  async (
    _request: NextRequest,
    context: RouteParams,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, id: projectId, path: pathSegments } = await context.params;
    const filePath = pathSegments.join("/");
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

      // Don't allow deleting config.arena.yaml.
      if (filePath === PROJECT_CONFIG_FILE) {
        return NextResponse.json(
          { error: BAD_REQUEST, message: "Cannot delete project config file" },
          { status: 400 }
        );
      }

      const relpath = `${projectRelPath(projectId)}/${filePath}`;

      // Check if file exists.
      try {
        await getContent(name, user, relpath);
      } catch (error) {
        if (isNotFound(error)) {
          return notFoundResponse(`File not found: ${filePath}`);
        }
        throw error;
      }

      // Delete (operator deletes directories recursively).
      await deleteContent(name, user, relpath);

      auditSuccess(auditCtx, "delete", `${projectId}/${filePath}`);
      return new NextResponse(null, { status: 204 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "delete", `${projectId}/${filePath}`, error, 500);
      }
      if (error instanceof ContentApiError) {
        return contentErrorResponse(error, "Failed to delete file");
      }
      return handleK8sError(error, "delete file");
    }
  }
);
