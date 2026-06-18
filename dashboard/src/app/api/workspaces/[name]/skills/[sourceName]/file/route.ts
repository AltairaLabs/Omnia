/**
 * Read a single file from a SkillSource. Mirrors arena sources/[name]/file.
 * GET /api/workspaces/:name/skills/:sourceName/file?path=<relativePath>
 *
 * Content is served by the operator's authenticated content API (the dashboard
 * no longer mounts the NFS workspace-content volume directly — see #1462). The
 * SkillSource CRD is still read to resolve the on-disk base (default
 * skills/<sourceName> or spec.targetPath). Path-confinement and max-size are
 * enforced operator-side and surfaced as pass-through statuses.
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
import { contentErrorResponse } from "@/lib/data/content-api-response";
import {
  ContentApiError,
  getContent,
  isContentFile,
  isContentListing,
} from "@/lib/data/content-api-service";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { SkillSource } from "@/types/skill-source";

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
        return NextResponse.json(
          { error: "Skill source content not available" },
          { status: 404 }
        );
      }

      const node = await getContent(name, user, `${contentRelPath}/${filePath}`);
      if (isContentListing(node)) {
        return NextResponse.json({ error: "Path is a directory" }, { status: 400 });
      }
      if (!isContentFile(node)) {
        return NextResponse.json(
          { error: "Internal Server Error", message: "Unexpected content response" },
          { status: 500 }
        );
      }

      auditSuccess(auditCtx, "get", sourceName, {
        subresource: "file",
        filePath,
        size: node.size,
      });

      return NextResponse.json({ path: filePath, content: node.content, size: node.size });
    } catch (error) {
      if (auditCtx) auditError(auditCtx, "get", sourceName, error, 500);
      if (error instanceof ContentApiError) {
        return contentErrorResponse(error, "read file from skill source");
      }
      return handleK8sError(error, "read file from skill source");
    }
  }
);
