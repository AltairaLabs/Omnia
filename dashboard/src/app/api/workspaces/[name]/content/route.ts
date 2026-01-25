/**
 * API route for workspace content root listing.
 *
 * GET /api/workspaces/:name/content - List root of workspace content
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

interface RouteParams {
  params: Promise<{ name: string }>;
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
 * GET /api/workspaces/:name/content
 *
 * Get root directory listing from workspace storage.
 * Requires at least viewer role in the workspace.
 */
export const GET = withWorkspaceAccess(
  "viewer",
  async (
    _request: NextRequest,
    context: RouteParams,
    _access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    try {
      const { name: workspaceName } = await context.params;

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

      // Check if path exists
      try {
        await fs.access(workspaceBasePath);
      } catch {
        // Directory doesn't exist yet (PVC might be empty or not mounted)
        // Return empty listing instead of 404
        const response: DirectoryListing = {
          path: "/",
          files: [],
          directories: [],
        };
        return NextResponse.json(response);
      }

      const stats = await fs.stat(workspaceBasePath);

      if (!stats.isDirectory()) {
        return NextResponse.json(
          {
            error: "Internal Server Error",
            message: "Workspace content path is not a directory",
          },
          { status: 500 }
        );
      }

      // Return directory listing
      const entries = await fs.readdir(workspaceBasePath, {
        withFileTypes: true,
      });
      const files: FileInfo[] = [];
      const directories: FileInfo[] = [];

      for (const entry of entries) {
        const entryPath = path.join(workspaceBasePath, entry.name);
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
        path: "/",
        files,
        directories,
      };

      return NextResponse.json(response);
    } catch (error) {
      console.error("Failed to list content:", error);
      return NextResponse.json(
        {
          error: "Internal Server Error",
          message:
            error instanceof Error ? error.message : "Failed to list content",
        },
        { status: 500 }
      );
    }
  }
);
