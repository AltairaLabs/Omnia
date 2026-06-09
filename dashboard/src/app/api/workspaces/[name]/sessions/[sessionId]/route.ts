/**
 * Session detail proxy route.
 *
 * GET /api/workspaces/{name}/sessions/{sessionId}
 *   -> SESSION_API_URL/api/v1/sessions/{sessionId}
 *
 * DELETE /api/workspaces/{name}/sessions/{sessionId}
 *   -> SESSION_API_URL/api/v1/sessions/{sessionId}?namespace={namespace}
 *
 * Both verify the session belongs to the workspace's namespace before
 * acting (prevents cross-workspace IDOR). Delete requires editor role.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { verifySessionNamespace } from "../session-namespace-guard";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";

type Params = { name: string; sessionId: string };

export const GET = withWorkspaceAccess<Params>(
  "viewer",
  async (
    _request: NextRequest,
    context: WorkspaceRouteContext<Params>,
    _access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    const { name, sessionId } = await context.params;

    const guard = await verifySessionNamespace(name, sessionId);
    if (!guard.ok) return guard.response;

    const targetUrl = `${guard.baseUrl}/api/v1/sessions/${encodeURIComponent(sessionId)}`;

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
        },
        { status: 502 }
      );
    }
  }
);

export const DELETE = withWorkspaceAccess<Params>(
  "editor",
  async (
    _request: NextRequest,
    context: WorkspaceRouteContext<Params>,
    _access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    const { name, sessionId } = await context.params;

    // Resolves the workspace namespace AND confirms the session belongs to it,
    // so the backend's namespace guard can never delete a foreign session.
    const guard = await verifySessionNamespace(name, sessionId);
    if (!guard.ok) return guard.response;

    const targetUrl =
      `${guard.baseUrl}/api/v1/sessions/${encodeURIComponent(sessionId)}` +
      `?namespace=${encodeURIComponent(guard.namespace)}`;

    try {
      const response = await fetch(targetUrl, {
        method: "DELETE",
        headers: { Accept: "application/json" },
      });

      // 204 No Content has no body to parse.
      if (response.status === 204) {
        return new NextResponse(null, { status: 204 });
      }

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
