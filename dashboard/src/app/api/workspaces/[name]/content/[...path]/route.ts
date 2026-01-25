/**
 * API route for workspace content filesystem operations.
 *
 * GET /api/workspaces/:name/content/:path - List directory or get file content
 *
 * Query Parameters:
 * - file=true: Return file content instead of directory listing
 * - version=<hash|tag>: Get content at specific version (for versioned arena content)
 *
 * The content is served from the workspace's shared PVC mounted at:
 * /workspace-content/{workspace-name}/{namespace}/
 *
 * Protected by workspace access checks. User must have at least viewer role.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess } from "@/lib/auth/workspace-guard";
import { getWorkspace } from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import * as fs from "node:fs/promises";
import * as path from "node:path";

// Base path for workspace content (configurable via environment)
const WORKSPACE_CONTENT_BASE =
  process.env.WORKSPACE_CONTENT_PATH || "/workspace-content";

// Error message constants
const ERROR_BAD_REQUEST = "Bad Request";

interface RouteParams {
  params: Promise<{ name: string; path: string[] }>;
}

interface FileInfo {
  name: string;
  type: "file" | "directory";
  size: number;
  modifiedAt: string;
}

interface DirectoryListing {
  path: string;
  files: FileInfo[];
  directories: FileInfo[];
}

interface FileContent {
  path: string;
  content: string;
  size: number;
  modifiedAt: string;
  encoding: "utf-8" | "base64";
}

/**
 * Check if a path is safe (no path traversal)
 */
function isPathSafe(requestedPath: string, basePath: string): boolean {
  const resolvedPath = path.resolve(basePath, requestedPath);
  return resolvedPath.startsWith(path.resolve(basePath));
}

/**
 * GET /api/workspaces/:name/content/:path
 *
 * Get directory listing or file content from workspace storage.
 * Requires at least viewer role in the workspace.
 *
 * Query Parameters:
 * - file=true: Return file content instead of directory listing
 * - version=<hash|tag>: Get content at specific version
 */
/* eslint-disable sonarjs/cognitive-complexity -- route handler with multiple paths */
export const GET = withWorkspaceAccess(
  "viewer",
  async (
    request: NextRequest,
    context: RouteParams,
    _access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    try {
      const { name: workspaceName, path: pathSegments } = await context.params;
      const searchParams = request.nextUrl.searchParams;
      const isFileRequest = searchParams.get("file") === "true";
      const version = searchParams.get("version");

      // Get workspace to find the namespace
      const workspace = await getWorkspace(workspaceName);
      if (!workspace) {
        return NextResponse.json(
          { error: "Not Found", message: `Workspace not found: ${workspaceName}` },
          { status: 404 }
        );
      }
      const namespaceName = workspace.spec.namespace.name;

      // Build the filesystem path
      // Base: /workspace-content/{workspace}/{namespace}/
      const workspaceBasePath = path.join(
        WORKSPACE_CONTENT_BASE,
        workspaceName,
        namespaceName
      );

      // Join path segments
      let requestedPath = pathSegments ? pathSegments.join("/") : "";

      // If version is specified and path starts with arena/, modify path to use version directory
      if (version && requestedPath.startsWith("arena/")) {
        const pathParts = requestedPath.split("/");
        if (pathParts.length >= 2) {
          // arena/{config-name}/... -> arena/{config-name}/.arena/versions/{version}/...
          const configName = pathParts[1];
          const remainingPath = pathParts.slice(2).join("/");

          // Resolve version (could be a tag or hash)
          const versionPath = await resolveVersion(
            workspaceBasePath,
            configName,
            version
          );
          if (versionPath) {
            requestedPath = path.join(
              "arena",
              configName,
              ".arena",
              "versions",
              versionPath,
              remainingPath
            );
          }
        }
      }

      const fullPath = path.join(workspaceBasePath, requestedPath);

      // Security check: prevent path traversal
      if (!isPathSafe(requestedPath, workspaceBasePath)) {
        return NextResponse.json(
          {
            error: ERROR_BAD_REQUEST,
            message: "Invalid path: path traversal not allowed",
          },
          { status: 400 }
        );
      }

      // Check if path exists
      try {
        await fs.access(fullPath);
      } catch {
        return NextResponse.json(
          {
            error: "Not Found",
            message: `Path not found: ${requestedPath}`,
          },
          { status: 404 }
        );
      }

      const stats = await fs.stat(fullPath);

      if (stats.isDirectory()) {
        if (isFileRequest) {
          return NextResponse.json(
            {
              error: ERROR_BAD_REQUEST,
              message: "Cannot return file content for a directory",
            },
            { status: 400 }
          );
        }

        // Return directory listing
        const entries = await fs.readdir(fullPath, { withFileTypes: true });
        const files: FileInfo[] = [];
        const directories: FileInfo[] = [];

        for (const entry of entries) {
          const entryPath = path.join(fullPath, entry.name);
          const entryStats = await fs.stat(entryPath);

          const info: FileInfo = {
            name: entry.name,
            type: entry.isDirectory() ? "directory" : "file",
            size: entryStats.size,
            modifiedAt: entryStats.mtime.toISOString(),
          };

          if (entry.isDirectory()) {
            directories.push(info);
          } else {
            files.push(info);
          }
        }

        // Sort alphabetically
        files.sort((a, b) => a.name.localeCompare(b.name));
        directories.sort((a, b) => a.name.localeCompare(b.name));

        const response: DirectoryListing = {
          path: requestedPath || "/",
          files,
          directories,
        };

        return NextResponse.json(response);
      } else {
        // It's a file
        if (!isFileRequest) {
          // Return file info only
          const info: FileInfo = {
            name: path.basename(fullPath),
            type: "file",
            size: stats.size,
            modifiedAt: stats.mtime.toISOString(),
          };
          return NextResponse.json(info);
        }

        // Return file content
        // Limit file size to prevent memory issues (10MB default)
        const maxFileSize = Number.parseInt(
          process.env.MAX_CONTENT_FILE_SIZE || "10485760",
          10
        );
        if (stats.size > maxFileSize) {
          return NextResponse.json(
            {
              error: ERROR_BAD_REQUEST,
              message: `File too large: ${stats.size} bytes (max: ${maxFileSize})`,
            },
            { status: 400 }
          );
        }

        const content = await fs.readFile(fullPath);

        // Determine if content is text or binary
        const isText = isTextFile(fullPath, content);

        const response: FileContent = {
          path: requestedPath,
          content: isText
            ? content.toString("utf-8")
            : content.toString("base64"),
          size: stats.size,
          modifiedAt: stats.mtime.toISOString(),
          encoding: isText ? "utf-8" : "base64",
        };

        return NextResponse.json(response);
      }
    } catch (error) {
      console.error("Failed to get content:", error);
      return NextResponse.json(
        {
          error: "Internal Server Error",
          message:
            error instanceof Error ? error.message : "Failed to get content",
        },
        { status: 500 }
      );
    }
  }
);
/* eslint-enable sonarjs/cognitive-complexity */

/**
 * Resolve a version reference (hash, tag, or "latest") to actual version hash
 */
async function resolveVersion(
  workspaceBasePath: string,
  configName: string,
  version: string
): Promise<string | null> {
  const arenaMetaPath = path.join(
    workspaceBasePath,
    "arena",
    configName,
    ".arena"
  );

  try {
    // Check if version is "latest" or a tag
    if (version === "latest") {
      // Read HEAD file
      const headPath = path.join(arenaMetaPath, "HEAD");
      try {
        const head = await fs.readFile(headPath, "utf-8");
        return head.trim();
      } catch {
        return null;
      }
    }

    // Check if it's a tag
    const tagPath = path.join(arenaMetaPath, "tags", version);
    try {
      const tagContent = await fs.readFile(tagPath, "utf-8");
      return tagContent.trim();
    } catch {
      // Not a tag, assume it's a version hash
    }

    // Check if version directory exists directly
    const versionPath = path.join(arenaMetaPath, "versions", version);
    try {
      await fs.access(versionPath);
      return version;
    } catch {
      return null;
    }
  } catch {
    return null;
  }
}

/**
 * Determine if a file is likely text based on extension and content
 */
function isTextFile(filePath: string, content: Buffer): boolean {
  const textExtensions = new Set([
    ".txt",
    ".md",
    ".yaml",
    ".yml",
    ".json",
    ".js",
    ".ts",
    ".jsx",
    ".tsx",
    ".html",
    ".css",
    ".scss",
    ".xml",
    ".csv",
    ".sh",
    ".bash",
    ".py",
    ".go",
    ".rs",
    ".java",
    ".c",
    ".cpp",
    ".h",
    ".hpp",
    ".sql",
    ".graphql",
    ".proto",
    ".toml",
    ".ini",
    ".cfg",
    ".conf",
    ".env",
    ".gitignore",
    ".dockerignore",
    ".editorconfig",
    "",
  ]);

  const ext = path.extname(filePath).toLowerCase();
  if (textExtensions.has(ext)) {
    return true;
  }

  // Check for null bytes which indicate binary content
  const sampleSize = Math.min(content.length, 8000);
  for (let i = 0; i < sampleSize; i++) {
    if (content[i] === 0) {
      return false;
    }
  }

  return true;
}
