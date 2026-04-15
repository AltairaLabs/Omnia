/**
 * Read a single file from a SkillSource. Mirrors arena sources/[name]/file.
 * GET /api/workspaces/:name/skills/:sourceName/file?path=<relativePath>
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
} from "@/lib/k8s/workspace-route-helpers";
import * as fs from "node:fs";
import * as path from "node:path";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { SkillSource } from "@/types/skill-source";

type RouteParams = { name: string; sourceName: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

const CRD_KIND = "SkillSource";
const WORKSPACE_CONTENT_BASE = "/workspace-content";
const MAX_FILE_SIZE = 1024 * 1024;

function getSourceBasePath(
  workspaceName: string,
  namespace: string,
  sourceName: string,
  targetPath?: string
): string {
  const target = targetPath && targetPath.length > 0 ? targetPath : path.join("skills", sourceName);
  return path.join(WORKSPACE_CONTENT_BASE, workspaceName, namespace, target);
}

function resolveContentPath(basePath: string): string | null {
  const headPath = path.join(basePath, ".arena", "HEAD");
  try {
    if (fs.existsSync(headPath)) {
      const headVersion = fs.readFileSync(headPath, "utf-8").trim();
      if (headVersion) {
        const versionPath = path.join(basePath, ".arena", "versions", headVersion);
        if (fs.existsSync(versionPath)) return versionPath;
      }
    }
  } catch {
    // Fall through
  }
  if (fs.existsSync(basePath)) {
    const entries = fs.readdirSync(basePath);
    if (entries.some((e) => !e.startsWith("."))) return basePath;
  }
  return null;
}

export const GET = withWorkspaceAccess<RouteParams>(
  "viewer",
  async (
    request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, sourceName } = await context.params;
    let auditCtx;

    try {
      const filePath = request.nextUrl.searchParams.get("path");
      if (!filePath) {
        return NextResponse.json(
          { error: "Missing required query parameter: path" },
          { status: 400 }
        );
      }

      const normalized = path.normalize(filePath);
      if (normalized.startsWith("..") || path.isAbsolute(normalized)) {
        return NextResponse.json({ error: "Invalid file path" }, { status: 400 });
      }

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
      const contentPath = resolveContentPath(basePath);
      if (!contentPath) {
        return NextResponse.json(
          { error: "Skill source content not available" },
          { status: 404 }
        );
      }

      const fullPath = path.join(contentPath, normalized);
      const resolvedFull = path.resolve(fullPath);
      const resolvedContent = path.resolve(contentPath);
      if (!resolvedFull.startsWith(resolvedContent + path.sep)) {
        return NextResponse.json({ error: "Invalid file path" }, { status: 400 });
      }

      if (!fs.existsSync(fullPath)) {
        return NextResponse.json({ error: "File not found" }, { status: 404 });
      }

      const stats = fs.statSync(fullPath);
      if (stats.isDirectory()) {
        return NextResponse.json({ error: "Path is a directory" }, { status: 400 });
      }
      if (stats.size > MAX_FILE_SIZE) {
        return NextResponse.json(
          { error: `File too large (${stats.size} bytes, max ${MAX_FILE_SIZE})` },
          { status: 413 }
        );
      }

      const content = fs.readFileSync(fullPath, "utf-8");

      auditSuccess(auditCtx, "get", sourceName, {
        subresource: "file",
        filePath: normalized,
        size: stats.size,
      });

      return NextResponse.json({ path: normalized, content, size: stats.size });
    } catch (error) {
      if (auditCtx) auditError(auditCtx, "get", sourceName, error, 500);
      return handleK8sError(error, "read file from skill source");
    }
  }
);
