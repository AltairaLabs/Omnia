/**
 * Session list/search proxy route.
 *
 * GET /api/workspaces/{name}/sessions
 *   → SESSION_API_URL/api/v1/sessions?workspace={namespace}&...
 *
 * When `q` param is present, routes to the /search backend endpoint.
 *
 * DELETE /api/workspaces/{name}/sessions[?agent=&before=]
 *   → SESSION_API_URL/api/v1/sessions?namespace={namespace}[&agent=&before=]
 *
 * Bulk purge of all sessions in the workspace's namespace (optionally
 * narrowed by agent / before-cutoff). Owner-only and user-agnostic —
 * removes automated sessions (ArenaJob, function) too.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { getWorkspace } from "@/lib/k8s/workspace-route-helpers";
import { resolveServiceURLs } from "@/lib/k8s/service-url-resolver";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";

export const GET = withWorkspaceAccess(
  "viewer",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext,
    _access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;

    const urls = await resolveServiceURLs(name);
    if (!urls) {
      return NextResponse.json(
        { error: "Session API not configured", sessions: [], total: 0, hasMore: false },
        { status: 503 }
      );
    }

    // Resolve the k8s namespace from the workspace CRD — sessions are stored
    // by namespace, not by workspace CRD name.
    const workspace = await getWorkspace(name);
    if (!workspace) {
      return NextResponse.json(
        { error: "Workspace not found", sessions: [], total: 0, hasMore: false },
        { status: 404 }
      );
    }
    const namespace = workspace.spec.namespace.name;

    // Forward query params to the backend — filter by k8s namespace and workspace
    const params = new URLSearchParams();
    params.set("namespace", namespace);
    params.set("workspace", name);

    const forwardParams = ["agent", "status", "from", "to", "limit", "offset", "q", "count"];
    for (const key of forwardParams) {
      const value = request.nextUrl.searchParams.get(key);
      if (value) params.set(key, value);
    }

    // Route to /search endpoint when q param is present
    const hasQuery = request.nextUrl.searchParams.has("q");
    const endpoint = hasQuery ? "sessions/search" : "sessions";

    const baseUrl = urls.sessionURL.endsWith("/") ? urls.sessionURL.slice(0, -1) : urls.sessionURL;
    const targetUrl = `${baseUrl}/api/v1/${endpoint}?${params.toString()}`;

    try {
      const response = await fetch(targetUrl, {
        headers: { Accept: "application/json" },
      });

      const data = await response.json();
      return NextResponse.json(data, { status: response.status });
    } catch (error) {
      console.error("Session API proxy error:", error);
      return NextResponse.json(
        {
          error: "Failed to connect to Session API",
          details: error instanceof Error ? error.message : String(error),
          sessions: [],
          total: 0,
          hasMore: false,
        },
        { status: 502 }
      );
    }
  }
);

export const DELETE = withWorkspaceAccess(
  "owner",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext,
    _access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;

    const urls = await resolveServiceURLs(name);
    if (!urls) {
      return NextResponse.json(
        { error: "Session API not configured" },
        { status: 503 }
      );
    }

    // Sessions are stored by k8s namespace, not workspace CRD name. Resolving it
    // here also bounds the purge to this workspace — the namespace is never
    // taken from the client.
    const workspace = await getWorkspace(name);
    if (!workspace) {
      return NextResponse.json({ error: "Workspace not found" }, { status: 404 });
    }
    const namespace = workspace.spec.namespace.name;

    const params = new URLSearchParams();
    params.set("namespace", namespace);
    for (const key of ["agent", "before"]) {
      const value = request.nextUrl.searchParams.get(key);
      if (value) params.set(key, value);
    }

    const baseUrl = urls.sessionURL.endsWith("/") ? urls.sessionURL.slice(0, -1) : urls.sessionURL;
    const targetUrl = `${baseUrl}/api/v1/sessions?${params.toString()}`;

    try {
      const response = await fetch(targetUrl, {
        method: "DELETE",
        headers: { Accept: "application/json" },
      });

      const data = await response.json();
      return NextResponse.json(data, { status: response.status });
    } catch (error) {
      console.error("Session API proxy error:", error);
      return NextResponse.json(
        {
          error: "Failed to connect to Session API",
          details: error instanceof Error ? error.message : String(error),
        },
        { status: 502 }
      );
    }
  }
);
