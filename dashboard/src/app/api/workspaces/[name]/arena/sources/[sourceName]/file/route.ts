/**
 * API route for reading individual ArenaSource files.
 *
 * GET /api/workspaces/:name/arena/sources/:sourceName/file?path=<relativePath>
 *
 * Returns the content of a single file within the source.
 *
 * Content is served by the operator's authenticated content API (the dashboard
 * no longer mounts the NFS workspace-content volume directly — see #1462). The
 * operator prepends <workspace>/<namespace> and enforces path confinement,
 * max-size and text/binary encoding; this route passes those statuses through.
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
} from "@/lib/k8s/workspace-route-helpers";
import {
  ContentApiError,
  getContent,
  isContentFile,
  isContentListing,
} from "@/lib/data/content-api-service";
import { contentErrorResponse } from "@/lib/data/content-api-response";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaSource } from "@/types/arena";

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
    return `${basePath}/.arena/versions/${headVersion}`;
  }

  if (await hasVisibleContent(workspace, user, basePath)) {
    return basePath;
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

      const result = await getWorkspaceResource<ArenaSource>(
        name,
        access.role!,
        CRD_ARENA_SOURCES,
        sourceName,
        "Arena source"
      );
      if (!result.ok) return result.response;

      const namespace = result.workspace.spec.namespace.name;
      auditCtx = createAuditContext(name, namespace, user, access.role!, CRD_KIND);

      const basePath = sourceBasePath(sourceName);
      const contentPath = await resolveContentPath(name, user, basePath);
      if (!contentPath) {
        return NextResponse.json(
          { error: "Source content not available" },
          { status: 404 }
        );
      }

      // Path confinement, max-size and a directory check are enforced
      // operator-side; surfaced here as pass-through 400/404/413 statuses.
      const relpath = `${contentPath}/${filePath}`;
      const node = await getContent(name, user, relpath);

      if (isContentListing(node)) {
        return NextResponse.json(
          { error: "Path is a directory" },
          { status: 400 }
        );
      }

      if (!isContentFile(node)) {
        return NextResponse.json(
          { error: "Unexpected content response" },
          { status: 500 }
        );
      }

      auditSuccess(auditCtx, "get", sourceName, {
        subresource: "file",
        filePath,
        size: node.size,
      });

      return NextResponse.json({
        path: filePath,
        content: node.content,
        size: node.size,
      });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", sourceName, error, 500);
      }
      if (error instanceof ContentApiError) {
        return contentErrorResponse(error, "read file from arena source");
      }
      return handleK8sError(error, "read file from arena source");
    }
  }
);
