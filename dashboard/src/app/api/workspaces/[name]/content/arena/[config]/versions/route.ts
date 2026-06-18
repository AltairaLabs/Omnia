/**
 * API route for listing arena config versions.
 *
 * GET /api/workspaces/:name/content/arena/:config/versions - List all versions
 *
 * Response:
 * {
 *   configName: string;
 *   head: string | null;    // Current HEAD version
 *   versions: {
 *     hash: string;
 *     createdAt: string;
 *     size: number;
 *   }[];
 * }
 *
 * Content is served by the operator's authenticated content API (the dashboard
 * no longer mounts the NFS workspace-content volume directly — see #1462).
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
} from "@/lib/data/content-api-service";
import { listContentTree } from "@/lib/data/content-tree";
import { contentErrorResponse } from "@/lib/data/content-api-response";

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

/** Read the HEAD pointer for an arena config, returning null when absent. */
async function readHead(
  workspace: string,
  user: User,
  metaPath: string
): Promise<string | null> {
  try {
    const node = await getContent(workspace, user, `${metaPath}/HEAD`);
    return isContentFile(node) ? node.content.trim() : null;
  } catch (error) {
    if (error instanceof ContentApiError && error.status === 404) {
      return null;
    }
    throw error;
  }
}

/** Recursively sum the byte size of all files under a version directory. */
async function versionSize(
  workspace: string,
  user: User,
  versionPath: string
): Promise<number> {
  const tree = await listContentTree(workspace, user, versionPath);
  let total = 0;
  const walk = (nodes: { isDirectory: boolean; size?: number; children?: typeof nodes }[]): void => {
    for (const node of nodes) {
      if (node.isDirectory) {
        if (node.children) walk(node.children);
      } else {
        total += node.size ?? 0;
      }
    }
  };
  walk(tree);
  return total;
}

/** List the versions under an arena config's .arena/versions directory. */
async function listVersions(
  workspace: string,
  user: User,
  versionsPath: string
): Promise<VersionInfo[]> {
  let listing;
  try {
    listing = await getContent(workspace, user, versionsPath);
  } catch (error) {
    if (error instanceof ContentApiError && error.status === 404) {
      return [];
    }
    throw error;
  }
  if (!isContentListing(listing)) {
    return [];
  }

  const versions: VersionInfo[] = [];
  for (const entry of listing.entries) {
    if (entry.type !== "directory") continue;
    const size = await versionSize(workspace, user, `${versionsPath}/${entry.name}`);
    versions.push({
      hash: entry.name,
      createdAt: entry.modifiedAt,
      size,
    });
  }

  // Sort by creation time (newest first).
  versions.sort(
    (a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
  );
  return versions;
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
    user: User
  ): Promise<NextResponse> => {
    const { name: workspaceName, config: configName } = await context.params;

    try {
      // Verify the arena config exists (pass through 404 if not).
      await getContent(workspaceName, user, `arena/${configName}`);

      const metaPath = `arena/${configName}/.arena`;
      const head = await readHead(workspaceName, user, metaPath);
      const versions = await listVersions(workspaceName, user, `${metaPath}/versions`);

      const response: VersionsResponse = {
        configName,
        head,
        versions,
      };

      return NextResponse.json(response);
    } catch (error) {
      return contentErrorResponse(error, "Failed to list versions");
    }
  }
);
