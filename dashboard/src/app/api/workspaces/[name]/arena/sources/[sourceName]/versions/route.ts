/**
 * API routes for ArenaSource version management.
 *
 * GET /api/workspaces/:name/arena/sources/:sourceName/versions - List all versions with metadata
 * POST /api/workspaces/:name/arena/sources/:sourceName/versions - Switch active version (update HEAD)
 *
 * Versions are stored in `.arena/versions/{hash}/` directories in the workspace filesystem.
 * The HEAD file points to the current active version.
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
import * as fs from "node:fs";
import * as path from "node:path";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaSource, ArenaVersion, ArenaVersionsResponse } from "@/types/arena";

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
 * Write the HEAD file to switch to a new version.
 */
function writeHeadVersion(basePath: string, versionHash: string): void {
  const arenaDir = path.join(basePath, ".arena");
  const headPath = path.join(arenaDir, "HEAD");

  // Ensure .arena directory exists
  if (!fs.existsSync(arenaDir)) {
    fs.mkdirSync(arenaDir, { recursive: true });
  }

  fs.writeFileSync(headPath, versionHash + "\n", "utf-8");
}

/**
 * Get metadata for a single version directory.
 */
function getVersionMetadata(versionsPath: string, hash: string, latestHash: string | null): ArenaVersion | null {
  const versionPath = path.join(versionsPath, hash);

  try {
    const stats = fs.statSync(versionPath);
    if (!stats.isDirectory()) {
      return null;
    }

    // Count files and calculate total size
    let fileCount = 0;
    let totalSize = 0;

    const countFiles = (dir: string) => {
      const entries = fs.readdirSync(dir, { withFileTypes: true });
      for (const entry of entries) {
        const fullPath = path.join(dir, entry.name);
        if (entry.isDirectory()) {
          countFiles(fullPath);
        } else if (entry.isFile()) {
          fileCount++;
          totalSize += fs.statSync(fullPath).size;
        }
      }
    };

    countFiles(versionPath);

    return {
      hash,
      createdAt: stats.birthtime.toISOString(),
      size: totalSize,
      fileCount,
      isLatest: hash === latestHash,
    };
  } catch (err) {
    console.error(`Error getting metadata for version ${hash}:`, err);
    return null;
  }
}

/**
 * Check if an entry is a valid version directory.
 */
function isValidVersionEntry(entry: fs.Dirent): boolean {
  return entry.isDirectory() && !entry.name.startsWith(".");
}

/**
 * Find the latest version hash from a list of version directories.
 */
function findLatestVersionHash(versionsPath: string, versionNames: string[]): string | null {
  let latestHash: string | null = null;
  let latestTime = 0;

  for (const name of versionNames) {
    const versionPath = path.join(versionsPath, name);
    const stats = fs.statSync(versionPath);
    const createdTime = stats.birthtime.getTime();

    if (createdTime > latestTime) {
      latestTime = createdTime;
      latestHash = name;
    }
  }

  return latestHash;
}

/**
 * List all available versions from the versions directory.
 */
function listVersions(basePath: string): ArenaVersion[] {
  const versionsPath = path.join(basePath, ".arena", "versions");

  try {
    if (!fs.existsSync(versionsPath)) {
      return [];
    }

    // Get all valid version directory names
    const entries = fs.readdirSync(versionsPath, { withFileTypes: true });
    const versionNames = entries.filter(isValidVersionEntry).map(e => e.name);

    // Find the latest version
    const latestHash = findLatestVersionHash(versionsPath, versionNames);

    // Collect metadata for all versions
    const versions = versionNames
      .map(name => getVersionMetadata(versionsPath, name, latestHash))
      .filter((v): v is ArenaVersion => v !== null);

    // Sort by createdAt descending (newest first)
    versions.sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime());

    return versions;
  } catch (err) {
    console.error(`Error listing versions at ${versionsPath}:`, err);
    return [];
  }
}

/**
 * Check if a version exists in the versions directory.
 */
function versionExists(basePath: string, hash: string): boolean {
  const versionPath = path.join(basePath, ".arena", "versions", hash);
  try {
    return fs.existsSync(versionPath) && fs.statSync(versionPath).isDirectory();
  } catch {
    return false;
  }
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
            `Source is not ready (phase: ${sourcePhase || "Unknown"}). Versions will be available once the source is synced.`
          );
        }
        return notFoundResponse(
          "Source content directory not found. The source may need to be re-synced."
        );
      }

      // Read HEAD and list versions
      const head = readHeadVersion(basePath);
      const versions = listVersions(basePath);

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

      // Parse request body
      const body = await request.json();
      const targetVersion = body.version;

      if (!targetVersion || typeof targetVersion !== "string") {
        return NextResponse.json(
          { error: "Missing or invalid 'version' field in request body" },
          { status: 400 }
        );
      }

      const basePath = getSourceBasePath(name, namespace, sourceName);

      // Check if the source content directory exists
      if (!fs.existsSync(basePath)) {
        return notFoundResponse(
          "Source content directory not found. The source may need to be synced first."
        );
      }

      // Check if the target version exists
      if (!versionExists(basePath, targetVersion)) {
        return NextResponse.json(
          { error: "Version not found. It may have been garbage collected." },
          { status: 404 }
        );
      }

      // Read current HEAD for comparison
      const previousHead = readHeadVersion(basePath);

      // Write new HEAD
      writeHeadVersion(basePath, targetVersion);

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
