/**
 * Browse content tree for a SkillSource. Mirrors arena sources/[name]/content.
 * GET /api/workspaces/:name/skills/:sourceName/content
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
import * as fs from "node:fs";
import * as path from "node:path";
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
const WORKSPACE_CONTENT_BASE = "/workspace-content";

function getSourceBasePath(
  workspaceName: string,
  namespace: string,
  sourceName: string,
  targetPath?: string
): string {
  const target = targetPath && targetPath.length > 0 ? targetPath : path.join("skills", sourceName);
  return path.join(WORKSPACE_CONTENT_BASE, workspaceName, namespace, target);
}

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

function resolveContentPath(basePath: string): string | null {
  const headVersion = readHeadVersion(basePath);
  if (headVersion) {
    const versionPath = path.join(basePath, ".arena", "versions", headVersion);
    if (fs.existsSync(versionPath)) {
      return versionPath;
    }
  }
  if (fs.existsSync(basePath)) {
    const entries = fs.readdirSync(basePath);
    if (entries.some((e) => !e.startsWith("."))) {
      return basePath;
    }
  }
  return null;
}

function buildContentTree(
  contentPath: string,
  relativePath = ""
): ArenaSourceContentNode[] {
  const nodes: ArenaSourceContentNode[] = [];
  const fullPath = relativePath ? path.join(contentPath, relativePath) : contentPath;

  try {
    const entries = fs.readdirSync(fullPath, { withFileTypes: true });
    for (const entry of entries) {
      if (entry.name.startsWith(".")) continue;
      const entryRel = relativePath ? `${relativePath}/${entry.name}` : entry.name;
      const entryFull = path.join(fullPath, entry.name);
      if (entry.isDirectory()) {
        nodes.push({
          name: entry.name,
          path: entryRel,
          isDirectory: true,
          children: buildContentTree(contentPath, entryRel),
        });
      } else if (entry.isFile()) {
        const stats = fs.statSync(entryFull);
        nodes.push({
          name: entry.name,
          path: entryRel,
          isDirectory: false,
          size: stats.size,
        });
      }
    }
  } catch (err) {
    console.error(`Error reading directory ${fullPath}:`, err);
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

      const basePath = getSourceBasePath(
        name,
        namespace,
        sourceName,
        result.resource.spec?.targetPath
      );

      if (!fs.existsSync(basePath)) {
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

      const contentPath = resolveContentPath(basePath);
      if (!contentPath) {
        return notFoundResponse("No content found. The source may need to be synced.");
      }

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
      if (auditCtx) auditError(auditCtx, "get", sourceName, error, 500);
      return handleK8sError(error, "get content for this skill source");
    }
  }
);
