/**
 * API route for workspace content filesystem operations.
 *
 * GET /api/workspaces/:name/content/:path - List directory or get file content
 *
 * Query Parameters:
 * - file=true: Return file content instead of directory listing
 * - version=<hash|tag>: Get content at specific version (for versioned arena content)
 *
 * Content is served by the operator's authenticated content API (the dashboard
 * no longer mounts the NFS workspace-content volume directly — see #1462).
 * Path-confinement and text/binary encoding are handled operator-side.
 *
 * Protected by workspace access checks. User must have at least viewer role.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess } from "@/lib/auth/workspace-guard";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import {
  ContentApiError,
  getContent,
  isContentFile,
  isContentListing,
  type ContentListing,
} from "@/lib/data/content-api-service";
import { contentErrorResponse } from "@/lib/data/content-api-response";

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

/**
 * GET /api/workspaces/:name/content/:path
 *
 * Get directory listing or file content from workspace storage.
 * Requires at least viewer role in the workspace.
 */
export const GET = withWorkspaceAccess(
  "viewer",
  async (
    request: NextRequest,
    context: RouteParams,
    _access: WorkspaceAccess,
    user: User,
  ): Promise<NextResponse> => {
    const { name: workspaceName, path: pathSegments } = await context.params;
    const searchParams = request.nextUrl.searchParams;
    const isFileRequest = searchParams.get("file") === "true";
    const version = searchParams.get("version");

    try {
      const requestedPath = await resolveRequestedPath(
        workspaceName,
        user,
        pathSegments ? pathSegments.join("/") : "",
        version,
      );

      const node = await getContent(workspaceName, user, requestedPath);

      if (isContentListing(node)) {
        if (isFileRequest) {
          return NextResponse.json(
            { error: ERROR_BAD_REQUEST, message: "Cannot return file content for a directory" },
            { status: 400 },
          );
        }
        return NextResponse.json(toDirectoryListing(node, requestedPath));
      }

      if (!isContentFile(node)) {
        return NextResponse.json(
          { error: "Internal Server Error", message: "Unexpected content response" },
          { status: 500 },
        );
      }

      if (!isFileRequest) {
        const info: FileInfo = {
          name: requestedPath.split("/").pop() || requestedPath,
          type: "file",
          size: node.size,
          modifiedAt: node.modifiedAt,
        };
        return NextResponse.json(info);
      }

      return NextResponse.json({
        path: requestedPath,
        content: node.content,
        size: node.size,
        modifiedAt: node.modifiedAt,
        encoding: node.encoding,
      });
    } catch (error) {
      return contentErrorResponse(error, "Failed to get content");
    }
  },
);

/** Map an operator listing to the dashboard's files/directories shape. */
function toDirectoryListing(node: ContentListing, requestedPath: string): DirectoryListing {
  const files: FileInfo[] = [];
  const directories: FileInfo[] = [];
  for (const entry of node.entries) {
    const info: FileInfo = {
      name: entry.name,
      type: entry.type,
      size: entry.size,
      modifiedAt: entry.modifiedAt,
    };
    if (entry.type === "directory") {
      directories.push(info);
    } else {
      files.push(info);
    }
  }
  files.sort((a, b) => a.name.localeCompare(b.name));
  directories.sort((a, b) => a.name.localeCompare(b.name));
  return { path: requestedPath || "/", files, directories };
}

/**
 * Rewrite an arena content path to a specific version directory when a
 * ?version is requested: arena/<config>/<rest> ->
 * arena/<config>/.arena/versions/<resolved>/<rest>.
 */
async function resolveRequestedPath(
  workspace: string,
  user: User,
  requestedPath: string,
  version: string | null,
): Promise<string> {
  if (!version || !requestedPath.startsWith("arena/")) {
    return requestedPath;
  }
  const parts = requestedPath.split("/");
  if (parts.length < 2) {
    return requestedPath;
  }
  const configName = parts[1];
  const remainingPath = parts.slice(2).join("/");
  const resolved = await resolveVersion(workspace, user, configName, version);
  if (!resolved) {
    return requestedPath;
  }
  return ["arena", configName, ".arena", "versions", resolved, remainingPath]
    .filter(Boolean)
    .join("/");
}

/** Resolve a version reference (hash, tag, or "latest") to an actual version hash. */
async function resolveVersion(
  workspace: string,
  user: User,
  configName: string,
  version: string,
): Promise<string | null> {
  const meta = `arena/${configName}/.arena`;

  if (version === "latest") {
    const head = await readMaybeFile(workspace, user, `${meta}/HEAD`);
    return head ? head.trim() : null;
  }

  const tag = await readMaybeFile(workspace, user, `${meta}/tags/${version}`);
  if (tag) {
    return tag.trim();
  }

  if (await pathExists(workspace, user, `${meta}/versions/${version}`)) {
    return version;
  }
  return null;
}

/** Read a file's content, returning null if it does not exist. */
async function readMaybeFile(workspace: string, user: User, relpath: string): Promise<string | null> {
  try {
    const node = await getContent(workspace, user, relpath);
    return isContentFile(node) ? node.content : null;
  } catch (error) {
    if (error instanceof ContentApiError && error.status === 404) {
      return null;
    }
    throw error;
  }
}

/** Report whether a path exists. */
async function pathExists(workspace: string, user: User, relpath: string): Promise<boolean> {
  try {
    await getContent(workspace, user, relpath);
    return true;
  } catch (error) {
    if (error instanceof ContentApiError && error.status === 404) {
      return false;
    }
    throw error;
  }
}
