/**
 * API route for browsing ArenaSource content.
 *
 * GET /api/workspaces/:name/arena/sources/:sourceName/content - Get source file tree
 *
 * Returns the folder/file structure of the source content for browsing.
 * This is used by the ConfigDialog to let users select a root folder.
 *
 * Content is served by the operator's authenticated content API (the dashboard
 * no longer mounts the NFS workspace-content volume directly — see #1462). The
 * operator prepends <workspace>/<namespace>, so paths here are relative to the
 * workspace root (e.g. `arena/<sourceName>`).
 *
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
import {
  ContentApiError,
  getContent,
  isContentFile,
  isContentListing,
} from "@/lib/data/content-api-service";
import { listContentTree, type ContentTreeNode } from "@/lib/data/content-tree";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaSource, ArenaSourceContentResponse, ArenaSourceContentNode } from "@/types/arena";

type RouteParams = { name: string; sourceName: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

const CRD_KIND = "ArenaSource";

/**
 * Workspace-relative base path for an ArenaSource's content directory.
 * The operator prepends <workspace>/<namespace>.
 */
function sourceBasePath(sourceName: string): string {
  return `arena/${sourceName}`;
}

/**
 * Read a file's trimmed content, returning null when it does not exist.
 */
async function readMaybeFile(workspace: string, user: User, relpath: string): Promise<string | null> {
  try {
    const node = await getContent(workspace, user, relpath);
    return isContentFile(node) ? node.content.trim() : null;
  } catch (error) {
    if (error instanceof ContentApiError && error.status === 404) {
      return null;
    }
    throw error;
  }
}

/**
 * Report whether a path exists.
 */
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

/**
 * Report whether a directory listing has any non-hidden entries.
 */
async function hasVisibleContent(workspace: string, user: User, relpath: string): Promise<boolean> {
  try {
    const node = await getContent(workspace, user, relpath);
    if (!isContentListing(node)) {
      return false;
    }
    return node.entries.some(e => !e.name.startsWith("."));
  } catch (error) {
    if (error instanceof ContentApiError && error.status === 404) {
      return false;
    }
    throw error;
  }
}

/**
 * Resolve the content path, preferring the HEAD version over the legacy
 * direct-in-base layout. Returns null when no content is available.
 */
async function resolveContentPath(workspace: string, user: User, basePath: string): Promise<string | null> {
  const headVersion = await readMaybeFile(workspace, user, `${basePath}/.arena/HEAD`);
  if (headVersion) {
    const versionPath = `${basePath}/.arena/versions/${headVersion}`;
    if (await pathExists(workspace, user, versionPath)) {
      return versionPath;
    }
  }

  if (await hasVisibleContent(workspace, user, basePath)) {
    return basePath;
  }

  return null;
}

/**
 * Map a content tree (paths relative to the workspace root) to the
 * ArenaSource node shape (paths relative to the content root).
 */
function toSourceTree(nodes: ContentTreeNode[], rootPrefix: string): ArenaSourceContentNode[] {
  const mapped: ArenaSourceContentNode[] = nodes.map(node => {
    const relativePath = node.path.startsWith(rootPrefix) ? node.path.slice(rootPrefix.length) : node.path;
    if (node.isDirectory) {
      return {
        name: node.name,
        path: relativePath,
        isDirectory: true,
        children: toSourceTree(node.children ?? [], rootPrefix),
      };
    }
    return {
      name: node.name,
      path: relativePath,
      isDirectory: false,
      size: node.size,
    };
  });

  // Sort: directories first, then files, alphabetically.
  mapped.sort((a, b) => {
    if (a.isDirectory !== b.isDirectory) {
      return a.isDirectory ? -1 : 1;
    }
    return a.name.localeCompare(b.name);
  });

  return mapped;
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
      // Get the ArenaSource to verify it exists and read its phase.
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

      const basePath = sourceBasePath(sourceName);

      // Check if the source content directory exists.
      if (!(await pathExists(name, user, basePath))) {
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

      // Resolve the content path (HEAD version or fallback).
      const contentPath = await resolveContentPath(name, user, basePath);
      if (!contentPath) {
        return notFoundResponse(
          "No content found. The source may need to be synced."
        );
      }

      // Build the content tree (skip dotfiles, e.g. the .arena dir).
      const treeNodes = await listContentTree(name, user, contentPath, { skipHidden: true });
      const tree = toSourceTree(treeNodes, `${contentPath}/`);

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
