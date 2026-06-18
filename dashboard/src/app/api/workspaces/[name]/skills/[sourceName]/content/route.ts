/**
 * Browse content tree for a SkillSource. Mirrors arena sources/[name]/content.
 * GET /api/workspaces/:name/skills/:sourceName/content
 *
 * Content is served by the operator's authenticated content API (the dashboard
 * no longer mounts the NFS workspace-content volume directly — see #1462). The
 * SkillSource CRD is still read to resolve the on-disk base (default
 * skills/<sourceName> or spec.targetPath) and the readiness/phase check.
 */

import { NextRequest, NextResponse } from "next/server";
import {
  withWorkspaceAccess,
  type WorkspaceRouteContext,
} from "@/lib/auth/workspace-guard";
import {
  getWorkspaceResource,
  handleK8sError,
  CRD_SKILL_SOURCES,
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
import type { SkillSource } from "@/types/skill-source";
import type {
  ArenaSourceContentNode,
  ArenaSourceContentResponse,
} from "@/types/arena";

type RouteParams = { name: string; sourceName: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

const CRD_KIND = "SkillSource";

/** Workspace-relative base path for a skill source's content. */
function getSourceBaseRelPath(sourceName: string, targetPath?: string): string {
  if (targetPath && targetPath.length > 0) {
    return targetPath;
  }
  return `skills/${sourceName}`;
}

/** Read a file's trimmed content, or null if it does not exist. */
async function readMaybeFile(
  workspace: string,
  user: User,
  relpath: string
): Promise<string | null> {
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

/** Report whether a directory listing exists and has at least one non-hidden entry. */
async function listingHasVisibleEntries(
  workspace: string,
  user: User,
  relpath: string
): Promise<boolean> {
  try {
    const node = await getContent(workspace, user, relpath);
    return (
      isContentListing(node) && node.entries.some((e) => !e.name.startsWith("."))
    );
  } catch (error) {
    if (error instanceof ContentApiError && error.status === 404) {
      return false;
    }
    throw error;
  }
}

/**
 * Resolve the content path for a source base: the HEAD version dir if present,
 * otherwise the base itself when it has visible content. Returns null when no
 * content is available.
 */
async function resolveContentRelPath(
  workspace: string,
  user: User,
  baseRelPath: string
): Promise<string | null> {
  const headVersion = await readMaybeFile(
    workspace,
    user,
    `${baseRelPath}/.arena/HEAD`
  );
  if (headVersion) {
    const versionPath = `${baseRelPath}/.arena/versions/${headVersion}`;
    if (await listingHasVisibleEntries(workspace, user, versionPath)) {
      return versionPath;
    }
  }
  if (await listingHasVisibleEntries(workspace, user, baseRelPath)) {
    return baseRelPath;
  }
  return null;
}

/**
 * Map a content tree node onto the arena source node shape. listContentTree
 * paths are workspace-relative; strip the content-root prefix so node.path is
 * relative to the source content root (matching the old fs-based behaviour).
 */
function toSourceNode(rootPrefix: string, node: ContentTreeNode): ArenaSourceContentNode {
  const relPath = node.path.startsWith(rootPrefix)
    ? node.path.slice(rootPrefix.length)
    : node.path;
  if (node.isDirectory) {
    return {
      name: node.name,
      path: relPath,
      isDirectory: true,
      children: (node.children ?? []).map((child) => toSourceNode(rootPrefix, child)),
    };
  }
  return {
    name: node.name,
    path: relPath,
    isDirectory: false,
    size: node.size,
  };
}

/** Sort directories first, then alphabetically, recursively. */
function sortNodes(nodes: ArenaSourceContentNode[]): ArenaSourceContentNode[] {
  for (const node of nodes) {
    if (node.children) {
      sortNodes(node.children);
    }
  }
  nodes.sort((a, b) => {
    if (a.isDirectory !== b.isDirectory) return a.isDirectory ? -1 : 1;
    return a.name.localeCompare(b.name);
  });
  return nodes;
}

function countFiles(nodes: ArenaSourceContentNode[]): number {
  let count = 0;
  for (const node of nodes) {
    if (node.isDirectory && node.children) count += countFiles(node.children);
    else if (!node.isDirectory) count++;
  }
  return count;
}

function countDirectories(nodes: ArenaSourceContentNode[]): number {
  let count = 0;
  for (const node of nodes) {
    if (node.isDirectory) {
      count++;
      if (node.children) count += countDirectories(node.children);
    }
  }
  return count;
}

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
      const result = await getWorkspaceResource<SkillSource>(
        name,
        access.role!,
        CRD_SKILL_SOURCES,
        sourceName,
        "Skill source"
      );
      if (!result.ok) return result.response;

      const namespace = result.workspace.spec.namespace.name;
      auditCtx = createAuditContext(name, namespace, user, access.role!, CRD_KIND);

      const baseRelPath = getSourceBaseRelPath(
        sourceName,
        result.resource.spec?.targetPath
      );

      const contentRelPath = await resolveContentRelPath(name, user, baseRelPath);
      if (!contentRelPath) {
        const phase = result.resource.status?.phase;
        if (phase !== "Ready") {
          return notFoundResponse(
            `Skill source is not ready (phase: ${phase || "Unknown"}). Content will be available once synced.`
          );
        }
        return notFoundResponse(
          "Skill source content directory not found. The source may need to be re-synced."
        );
      }

      const treeNodes = await listContentTree(name, user, contentRelPath, {
        skipHidden: true,
      });
      const rootPrefix = `${contentRelPath}/`;
      const tree = sortNodes(treeNodes.map((node) => toSourceNode(rootPrefix, node)));
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
      if (auditCtx) auditError(auditCtx, "get", sourceName, error, 500);
      return handleK8sError(error, "get content for this skill source");
    }
  }
);
