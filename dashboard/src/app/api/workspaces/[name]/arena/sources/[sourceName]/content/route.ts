/**
 * API route for browsing ArenaSource content.
 *
 * GET /api/workspaces/:name/arena/sources/:sourceName/content - Get source file tree
 *
 * Returns the folder/file structure of the source content for browsing.
 * This is used by the ConfigDialog to let users select a root folder.
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import {
  getWorkspaceResource,
  handleK8sError,
  CRD_ARENA_SOURCES,
  createAuditContext,
  auditSuccess,
  auditError,
  notFoundResponse,
} from "@/lib/k8s/workspace-route-helpers";
import * as fs from "node:fs";
import * as path from "node:path";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaSource, ArenaSourceContentResponse, ArenaSourceContentNode } from "@/types/arena";

type RouteParams = { name: string; sourceName: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

const CRD_KIND = "ArenaSource";

/** Base path for workspace content volume */
const WORKSPACE_CONTENT_BASE = "/workspace-content";

/**
 * Get the base path for an ArenaSource's content directory.
 * Pattern: /workspace-content/{workspace}/{namespace}/arena/{sourceName}
 */
function getSourceBasePath(workspaceName: string, namespace: string, sourceName: string): string {
  return path.join(WORKSPACE_CONTENT_BASE, workspaceName, namespace, "arena", sourceName);
}

/**
 * Read the HEAD file to get the current active version.
 */
function readHeadVersion(basePath: string): string | null {
  const headPath = path.join(basePath, ".arena", "HEAD");
  try {
    if (fs.existsSync(headPath)) {
      return fs.readFileSync(headPath, "utf-8").trim();
    }
  } catch (err) {
    console.error(`Error reading HEAD file at ${headPath}:`, err);
  }
  return null;
}

/**
 * Resolve the content path, preferring HEAD version over direct path.
 */
function resolveContentPath(basePath: string): string | null {
  // Try to read HEAD file to get the current version
  const headVersion = readHeadVersion(basePath);
  if (headVersion) {
    const versionPath = path.join(basePath, ".arena", "versions", headVersion);
    if (fs.existsSync(versionPath)) {
      return versionPath;
    }
    console.warn(`HEAD points to version ${headVersion} but directory doesn't exist`);
  }

  // Fall back to looking for content directly in basePath (legacy)
  if (fs.existsSync(basePath)) {
    // Check if there's actual content (not just .arena folder)
    const entries = fs.readdirSync(basePath);
    const hasContent = entries.some(e => !e.startsWith("."));
    if (hasContent) {
      return basePath;
    }
  }

  return null;
}

/**
 * Build the content tree from a directory.
 * Returns nodes for directories and files, with directories containing children.
 */
function buildContentTree(
  contentPath: string,
  relativePath: string = ""
): ArenaSourceContentNode[] {
  const nodes: ArenaSourceContentNode[] = [];
  const fullPath = relativePath ? path.join(contentPath, relativePath) : contentPath;

  try {
    const entries = fs.readdirSync(fullPath, { withFileTypes: true });

    for (const entry of entries) {
      // Skip hidden files and directories
      if (entry.name.startsWith(".")) continue;

      const entryRelativePath = relativePath ? `${relativePath}/${entry.name}` : entry.name;
      const entryFullPath = path.join(fullPath, entry.name);

      if (entry.isDirectory()) {
        // Recursively build children for directories
        const children = buildContentTree(contentPath, entryRelativePath);
        nodes.push({
          name: entry.name,
          path: entryRelativePath,
          isDirectory: true,
          children,
        });
      } else if (entry.isFile()) {
        // Get file size
        const stats = fs.statSync(entryFullPath);
        nodes.push({
          name: entry.name,
          path: entryRelativePath,
          isDirectory: false,
          size: stats.size,
        });
      }
    }
  } catch (err) {
    console.error(`Error reading directory ${fullPath}:`, err);
  }

  // Sort: directories first, then files, alphabetically
  nodes.sort((a, b) => {
    if (a.isDirectory !== b.isDirectory) {
      return a.isDirectory ? -1 : 1;
    }
    return a.name.localeCompare(b.name);
  });

  return nodes;
}

/**
 * Count total files in the tree.
 */
function countFiles(nodes: ArenaSourceContentNode[]): number {
  let count = 0;
  for (const node of nodes) {
    if (node.isDirectory && node.children) {
      count += countFiles(node.children);
    } else if (!node.isDirectory) {
      count++;
    }
  }
  return count;
}

/**
 * Count total directories in the tree.
 */
function countDirectories(nodes: ArenaSourceContentNode[]): number {
  let count = 0;
  for (const node of nodes) {
    if (node.isDirectory) {
      count++;
      if (node.children) {
        count += countDirectories(node.children);
      }
    }
  }
  return count;
}

// =============================================================================
// GET - List source content tree
// =============================================================================

export const GET = withWorkspaceAccess<RouteParams>(
  "viewer",
  async (
    _request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, sourceName } = await context.params;
    let auditCtx;

    try {
      // Get the ArenaSource to verify it exists and get namespace
      const result = await getWorkspaceResource<ArenaSource>(
        name,
        access.role!,
        CRD_ARENA_SOURCES,
        sourceName,
        "Arena source"
      );
      if (!result.ok) return result.response;

      const namespace = result.workspace.spec.namespace.name;
      auditCtx = createAuditContext(
        name,
        namespace,
        user,
        access.role!,
        CRD_KIND
      );

      const basePath = getSourceBasePath(name, namespace, sourceName);

      // Check if the source content directory exists
      if (!fs.existsSync(basePath)) {
        const sourcePhase = result.resource.status?.phase;
        if (sourcePhase !== "Ready") {
          return notFoundResponse(
            `Source is not ready (phase: ${sourcePhase || "Unknown"}). Content will be available once the source is synced.`
          );
        }
        return notFoundResponse(
          "Source content directory not found. The source may need to be re-synced."
        );
      }

      // Resolve the content path (HEAD version or fallback)
      const contentPath = resolveContentPath(basePath);
      if (!contentPath) {
        return notFoundResponse(
          "No content found. The source may need to be synced."
        );
      }

      // Build the content tree
      const tree = buildContentTree(contentPath);

      const response: ArenaSourceContentResponse = {
        sourceName,
        tree,
        fileCount: countFiles(tree),
        directoryCount: countDirectories(tree),
      };

      auditSuccess(auditCtx, "get", sourceName, {
        subresource: "content",
        fileCount: response.fileCount,
        directoryCount: response.directoryCount,
      });

      return NextResponse.json(response);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", sourceName, error, 500);
      }
      return handleK8sError(error, "get content for this arena source");
    }
  }
);
