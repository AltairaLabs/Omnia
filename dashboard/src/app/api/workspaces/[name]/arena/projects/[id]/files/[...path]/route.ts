/**
 * API routes for individual file operations in an Arena project.
 *
 * GET /api/workspaces/:name/arena/projects/:id/files/:path - Get file content
 * PUT /api/workspaces/:name/arena/projects/:id/files/:path - Update file content
 * POST /api/workspaces/:name/arena/projects/:id/files/:path - Create file/folder
 * DELETE /api/workspaces/:name/arena/projects/:id/files/:path - Delete file/folder
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
} from "@/types/arena-project";
import * as fs from "node:fs/promises";
import * as path from "node:path";

// Base path for workspace content (configurable via environment)
const WORKSPACE_CONTENT_BASE =
  process.env.WORKSPACE_CONTENT_PATH || "/workspace-content";

// Max file size for reading (10MB)
const MAX_FILE_SIZE = Number.parseInt(
  process.env.MAX_CONTENT_FILE_SIZE || "10485760",
  10
);

const RESOURCE_TYPE = "ArenaProjectFile";
const PATH_TRAVERSAL_ERROR = "Invalid path: path traversal not allowed";
const BAD_REQUEST = "Bad Request";

interface RouteParams {
  params: Promise<{ name: string; id: string; path: string[] }>;
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
 * Check if a path is safe (no path traversal)
 */
function isPathSafe(requestedPath: string, basePath: string): boolean {
  const resolvedPath = path.resolve(basePath, requestedPath);
  return resolvedPath.startsWith(path.resolve(basePath));
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

/**
 * Check if file is text based on extension
 */
function isTextFile(filePath: string): boolean {
  const textExtensions = new Set([
    ".txt", ".md", ".yaml", ".yml", ".json", ".js", ".ts", ".jsx", ".tsx",
    ".html", ".css", ".scss", ".xml", ".csv", ".sh", ".bash", ".py", ".go",
    ".rs", ".java", ".c", ".cpp", ".h", ".hpp", ".sql", ".graphql", ".proto",
    ".toml", ".ini", ".cfg", ".conf", ".env", ".gitignore", ".dockerignore",
    ".editorconfig", "",
  ]);
  const ext = path.extname(filePath).toLowerCase();
  return textExtensions.has(ext);
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

      const projectPath = getProjectPath(name, result.workspace.spec.namespace.name, projectId);
      const fullPath = path.join(projectPath, filePath);

      // Security check: prevent path traversal
      if (!isPathSafe(filePath, projectPath)) {
        return NextResponse.json(
          { error: BAD_REQUEST, message: PATH_TRAVERSAL_ERROR },
          { status: 400 }
        );
      }

      // Check if file exists
      try {
        await fs.access(fullPath);
      } catch {
        return notFoundResponse(`File not found: ${filePath}`);
      }

      const stats = await fs.stat(fullPath);

      if (stats.isDirectory()) {
        return NextResponse.json(
          { error: BAD_REQUEST, message: "Cannot get content of a directory" },
          { status: 400 }
        );
      }

      // Check file size
      if (stats.size > MAX_FILE_SIZE) {
        return NextResponse.json(
          { error: BAD_REQUEST, message: `File too large: ${stats.size} bytes (max: ${MAX_FILE_SIZE})` },
          { status: 400 }
        );
      }

      const content = await fs.readFile(fullPath);
      const isText = isTextFile(fullPath);

      const response: FileContentResponse = {
        path: filePath,
        content: isText ? content.toString("utf-8") : content.toString("base64"),
        size: stats.size,
        modifiedAt: stats.mtime.toISOString(),
        encoding: isText ? "utf-8" : "base64",
      };

      auditSuccess(auditCtx, "get", `${projectId}/${filePath}`);
      return NextResponse.json(response);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", `${projectId}/${filePath}`, error, 500);
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

      const projectPath = getProjectPath(name, result.workspace.spec.namespace.name, projectId);
      const fullPath = path.join(projectPath, filePath);

      // Security check: prevent path traversal
      if (!isPathSafe(filePath, projectPath)) {
        return NextResponse.json(
          { error: BAD_REQUEST, message: PATH_TRAVERSAL_ERROR },
          { status: 400 }
        );
      }

      // Check if file exists (must exist for PUT)
      try {
        const stats = await fs.stat(fullPath);
        if (stats.isDirectory()) {
          return NextResponse.json(
            { error: BAD_REQUEST, message: "Cannot update content of a directory" },
            { status: 400 }
          );
        }
      } catch {
        return notFoundResponse(`File not found: ${filePath}`);
      }

      // Write file
      await fs.writeFile(fullPath, body.content, "utf-8");

      const stats = await fs.stat(fullPath);

      const response: FileUpdateResponse = {
        path: filePath,
        size: stats.size,
        modifiedAt: stats.mtime.toISOString(),
      };

      auditSuccess(auditCtx, "update", `${projectId}/${filePath}`);
      return NextResponse.json(response);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "update", `${projectId}/${filePath}`, error, 500);
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

      const projectPath = getProjectPath(name, result.workspace.spec.namespace.name, projectId);
      const parentFullPath = path.join(projectPath, parentPath);
      const targetPath = path.join(parentFullPath, body.name);
      const targetRelativePath = `${parentPath}/${body.name}`;

      // Security check: prevent path traversal
      if (!isPathSafe(parentPath, projectPath) || !isPathSafe(targetRelativePath, projectPath)) {
        return NextResponse.json(
          { error: BAD_REQUEST, message: PATH_TRAVERSAL_ERROR },
          { status: 400 }
        );
      }

      // Check if parent exists and is a directory
      try {
        const parentStats = await fs.stat(parentFullPath);
        if (!parentStats.isDirectory()) {
          return NextResponse.json(
            { error: BAD_REQUEST, message: "Parent path is not a directory" },
            { status: 400 }
          );
        }
      } catch {
        return notFoundResponse(`Parent directory not found: ${parentPath}`);
      }

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

      if (body.isDirectory) {
        await fs.mkdir(targetPath, { recursive: true });
      } else {
        const content = body.content ?? "";
        await fs.writeFile(targetPath, content, "utf-8");
      }

      const stats = await fs.stat(targetPath);

      const response: FileCreateResponse = {
        path: targetRelativePath,
        name: body.name,
        isDirectory: body.isDirectory,
        size: body.isDirectory ? undefined : stats.size,
        modifiedAt: stats.mtime.toISOString(),
      };

      auditSuccess(auditCtx, "create", `${projectId}/${targetRelativePath}`);
      return NextResponse.json(response, { status: 201 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "create", `${projectId}/${parentPath}`, error, 500);
      }
      return handleK8sError(error, "create file");
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

      const projectPath = getProjectPath(name, result.workspace.spec.namespace.name, projectId);
      const fullPath = path.join(projectPath, filePath);

      // Security check: prevent path traversal
      if (!isPathSafe(filePath, projectPath)) {
        return NextResponse.json(
          { error: BAD_REQUEST, message: PATH_TRAVERSAL_ERROR },
          { status: 400 }
        );
      }

      // Don't allow deleting config.arena.yaml
      if (filePath === "config.arena.yaml") {
        return NextResponse.json(
          { error: BAD_REQUEST, message: "Cannot delete project config file" },
          { status: 400 }
        );
      }

      // Check if file exists
      try {
        await fs.access(fullPath);
      } catch {
        return notFoundResponse(`File not found: ${filePath}`);
      }

      const stats = await fs.stat(fullPath);

      if (stats.isDirectory()) {
        await deleteRecursive(fullPath);
      } else {
        await fs.unlink(fullPath);
      }

      auditSuccess(auditCtx, "delete", `${projectId}/${filePath}`);
      return new NextResponse(null, { status: 204 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "delete", `${projectId}/${filePath}`, error, 500);
      }
      return handleK8sError(error, "delete file");
    }
  }
);
