/**
 * API routes for ArenaSource version management.
 *
 * GET /api/workspaces/:name/arena/sources/:sourceName/versions - List all versions with metadata
 * POST /api/workspaces/:name/arena/sources/:sourceName/versions - Switch active version (update HEAD)
 *
 * Versions are stored in `.arena/versions/{hash}/` directories under the
 * source's content root. The `.arena/HEAD` file points to the active version.
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
  makeContentDir,
  writeContentFile,
  type ContentEntry,
} from "@/lib/data/content-api-service";
import { listContentTree, type ContentTreeNode } from "@/lib/data/content-tree";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaSource, ArenaVersion, ArenaVersionsResponse } from "@/types/arena";

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
 * Read the HEAD file to get the current active version, returning null when
 * it does not exist.
 */
async function readHeadVersion(workspace: string, user: User, basePath: string): Promise<string | null> {
  try {
    const node = await getContent(workspace, user, `${basePath}/.arena/HEAD`);
    return isContentFile(node) ? node.content.trim() : null;
  } catch (error) {
    if (error instanceof ContentApiError && error.status === 404) {
      return null;
    }
    throw error;
  }
}

/**
 * Write the HEAD file to switch to a new version, ensuring `.arena` exists.
 */
async function writeHeadVersion(
  workspace: string,
  user: User,
  basePath: string,
  versionHash: string
): Promise<void> {
  await makeContentDir(workspace, user, `${basePath}/.arena`);
  await writeContentFile(workspace, user, `${basePath}/.arena/HEAD`, versionHash + "\n");
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
 * List the version directory entries under `.arena/versions`, or [] when the
 * directory is missing.
 */
async function listVersionEntries(workspace: string, user: User, basePath: string): Promise<ContentEntry[]> {
  try {
    const node = await getContent(workspace, user, `${basePath}/.arena/versions`);
    if (!isContentListing(node)) {
      return [];
    }
    return node.entries.filter(e => e.type === "directory" && !e.name.startsWith("."));
  } catch (error) {
    if (error instanceof ContentApiError && error.status === 404) {
      return [];
    }
    throw error;
  }
}

/** Sum file count and total size from a content tree. */
function tallyTree(nodes: ContentTreeNode[]): { fileCount: number; totalSize: number } {
  let fileCount = 0;
  let totalSize = 0;
  for (const node of nodes) {
    if (node.isDirectory) {
      const child = tallyTree(node.children ?? []);
      fileCount += child.fileCount;
      totalSize += child.totalSize;
    } else {
      fileCount++;
      totalSize += node.size ?? 0;
    }
  }
  return { fileCount, totalSize };
}

/**
 * Build metadata for a single version directory.
 */
async function buildVersionMetadata(
  workspace: string,
  user: User,
  basePath: string,
  entry: ContentEntry,
  latestHash: string | null
): Promise<ArenaVersion> {
  const tree = await listContentTree(workspace, user, `${basePath}/.arena/versions/${entry.name}`);
  const { fileCount, totalSize } = tallyTree(tree);
  return {
    hash: entry.name,
    createdAt: entry.modifiedAt,
    size: totalSize,
    fileCount,
    isLatest: entry.name === latestHash,
  };
}

/**
 * Find the latest version hash (newest modifiedAt) from version entries.
 */
function findLatestVersionHash(entries: ContentEntry[]): string | null {
  let latestHash: string | null = null;
  let latestTime = -Infinity;
  for (const entry of entries) {
    const time = new Date(entry.modifiedAt).getTime();
    if (time > latestTime) {
      latestTime = time;
      latestHash = entry.name;
    }
  }
  return latestHash;
}

/**
 * List all available versions with metadata, newest first.
 */
async function listVersions(workspace: string, user: User, basePath: string): Promise<ArenaVersion[]> {
  const entries = await listVersionEntries(workspace, user, basePath);
  if (entries.length === 0) {
    return [];
  }

  const latestHash = findLatestVersionHash(entries);
  const versions = await Promise.all(
    entries.map(entry => buildVersionMetadata(workspace, user, basePath, entry, latestHash))
  );

  versions.sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime());
  return versions;
}

// =============================================================================
// GET - List all versions with metadata
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
            `Source is not ready (phase: ${sourcePhase || "Unknown"}). Versions will be available once the source is synced.`
          );
        }
        return notFoundResponse(
          "Source content directory not found. The source may need to be re-synced."
        );
      }

      // Read HEAD and list versions.
      const head = await readHeadVersion(name, user, basePath);
      const versions = await listVersions(name, user, basePath);

      const response: ArenaVersionsResponse = {
        sourceName,
        head,
        versions,
      };

      auditSuccess(auditCtx, "get", sourceName, {
        subresource: "versions",
        versionCount: versions.length,
        head,
      });

      return NextResponse.json(response);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", sourceName, error, 500);
      }
      return handleK8sError(error, "list versions for this arena source");
    }
  }
);

// =============================================================================
// POST - Switch active version (update HEAD)
// =============================================================================

export const POST = withWorkspaceAccess<RouteParams>(
  "editor",
  async (
    request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, sourceName } = await context.params;
    let auditCtx;

    try {
      // Get the ArenaSource to verify it exists.
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

      // Parse request body.
      const body = await request.json();
      const targetVersion = body.version;

      if (!targetVersion || typeof targetVersion !== "string") {
        return NextResponse.json(
          { error: "Missing or invalid 'version' field in request body" },
          { status: 400 }
        );
      }

      const basePath = sourceBasePath(sourceName);

      // Check if the source content directory exists.
      if (!(await pathExists(name, user, basePath))) {
        return notFoundResponse(
          "Source content directory not found. The source may need to be synced first."
        );
      }

      // Check if the target version exists.
      if (!(await pathExists(name, user, `${basePath}/.arena/versions/${targetVersion}`))) {
        return NextResponse.json(
          { error: "Version not found. It may have been garbage collected." },
          { status: 404 }
        );
      }

      // Read current HEAD for comparison.
      const previousHead = await readHeadVersion(name, user, basePath);

      // Write new HEAD.
      await writeHeadVersion(name, user, basePath, targetVersion);

      auditSuccess(auditCtx, "update", sourceName, {
        subresource: "versions",
        action: "switch",
        previousVersion: previousHead,
        newVersion: targetVersion,
      });

      return NextResponse.json({
        success: true,
        sourceName,
        previousHead,
        newHead: targetVersion,
      });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "update", sourceName, error, 500);
      }
      return handleK8sError(error, "switch version for this arena source");
    }
  }
);
