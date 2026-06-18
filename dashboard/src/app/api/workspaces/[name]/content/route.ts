/**
 * API route for workspace content root listing.
 *
 * GET /api/workspaces/:name/content - List root of workspace content
 *
 * Content is served by the operator's authenticated content API (the dashboard
 * no longer mounts the NFS workspace-content volume directly — see #1462). The
 * operator resolves the workspace's namespace and confines access to
 * <contentRoot>/<workspace>/<namespace>/.
 *
 * Protected by workspace access checks. User must have at least viewer role.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess } from "@/lib/auth/workspace-guard";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import { getContent, isContentListing } from "@/lib/data/content-api-service";
import { contentErrorResponse } from "@/lib/data/content-api-response";

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
    user: User,
  ): Promise<NextResponse> => {
    const { name: workspaceName } = await context.params;
    try {
      const node = await getContent(workspaceName, user, "");
      const files: FileInfo[] = [];
      const directories: FileInfo[] = [];
      if (isContentListing(node)) {
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
      }

      files.sort((a, b) => a.name.localeCompare(b.name));
      directories.sort((a, b) => a.name.localeCompare(b.name));

      const response: DirectoryListing = { path: "/", files, directories };
      return NextResponse.json(response);
    } catch (error) {
      return contentErrorResponse(error, "Failed to list content");
    }
  },
);
