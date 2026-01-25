/**
 * API route for listing arena config versions.
 *
 * GET /api/workspaces/:name/content/arena/:config/versions - List all versions
 *
 * Response:
 * {
 *   configName: string;
 *   head: string;           // Current HEAD version
 *   versions: {
 *     hash: string;
 *     createdAt: string;
 *     size: number;
 *   }[];
 * }
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
  params: Promise<{ name: string; config: string }>;
}

interface VersionInfo {
  hash: string;
  createdAt: string;
  size: number;
}

interface VersionsResponse {
  configName: string;
  head: string | null;
  versions: VersionInfo[];
}

/**
 * GET /api/workspaces/:name/content/arena/:config/versions
 *
 * List all versions for an arena config.
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
      const { name: workspaceName, config: configName } = await context.params;

      // Get workspace to find the namespace
      const workspace = await getWorkspace(workspaceName);
      if (!workspace) {
        return NextResponse.json(
          { error: "Not Found", message: `Workspace not found: ${workspaceName}` },
          { status: 404 }
        );
      }
      const namespaceName = workspace.spec.namespace.name;

      // Build paths: {base}/{workspace}/{namespace}/arena/{config}
      const arenaBasePath = path.join(
        WORKSPACE_CONTENT_BASE,
        workspaceName,
        namespaceName,
        "arena",
        configName
      );
      const arenaMetaPath = path.join(arenaBasePath, ".arena");
      const versionsPath = path.join(arenaMetaPath, "versions");
      const headPath = path.join(arenaMetaPath, "HEAD");

      // Check if the arena config exists
      try {
        await fs.access(arenaBasePath);
      } catch {
        return NextResponse.json(
          {
            error: "Not Found",
            message: `Arena config not found: ${configName}`,
          },
          { status: 404 }
        );
      }

      // Read HEAD if it exists
      let head: string | null = null;
      try {
        const headContent = await fs.readFile(headPath, "utf-8");
        head = headContent.trim();
      } catch {
        // HEAD doesn't exist yet
      }

      // List versions
      const versions: VersionInfo[] = [];
      try {
        const entries = await fs.readdir(versionsPath, { withFileTypes: true });

        for (const entry of entries) {
          if (!entry.isDirectory()) continue;

          const versionDir = path.join(versionsPath, entry.name);
          const stats = await fs.stat(versionDir);

          // Calculate total size of version directory
          let totalSize = 0;
          const calculateSize = async (dir: string): Promise<void> => {
            const items = await fs.readdir(dir, { withFileTypes: true });
            for (const item of items) {
              const itemPath = path.join(dir, item.name);
              if (item.isDirectory()) {
                await calculateSize(itemPath);
              } else {
                const itemStats = await fs.stat(itemPath);
                totalSize += itemStats.size;
              }
            }
          };
          await calculateSize(versionDir);

          versions.push({
            hash: entry.name,
            createdAt: stats.mtime.toISOString(),
            size: totalSize,
          });
        }

        // Sort by creation time (newest first)
        versions.sort(
          (a, b) =>
            new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
        );
      } catch {
        // Versions directory doesn't exist yet
      }

      const response: VersionsResponse = {
        configName,
        head,
        versions,
      };

      return NextResponse.json(response);
    } catch (error) {
      console.error("Failed to list versions:", error);
      return NextResponse.json(
        {
          error: "Internal Server Error",
          message:
            error instanceof Error ? error.message : "Failed to list versions",
        },
        { status: 500 }
      );
    }
  }
);
